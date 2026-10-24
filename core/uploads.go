package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	UploadPartBytes     int64 = 8 << 20
	MaxUploadBytes      int64 = 10 << 30
	MaxSplitUploadBytes int64 = 1 << 30
	UploadSessionTTL          = 24 * time.Hour
	uploadSpoolDirName        = ".callfs-uploads"
	uploadRecordName          = "session.json"
	uploadDataName            = "data"
	uploadHashName            = "hash.state"
	copyBufferBytes           = 256 << 10
	uploadMaxShards           = 255
	uploadMaxInstances        = 64
	uploadDefaultMode         = "0644"
)

var (
	uploadIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	uploadModePattern = regexp.MustCompile(`^0?[0-7]{3}$`)
)

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, copyBufferBytes)
		return &buf
	},
}

type plainWriter struct {
	w io.Writer
}

func (p plainWriter) Write(b []byte) (int, error) {
	return p.w.Write(b)
}

func copyPooled(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufPtr)
	return io.CopyBuffer(plainWriter{w: dst}, src, *bufPtr)
}

type UploadState string

const (
	UploadOpen       UploadState = "open"
	UploadCompleting UploadState = "completing"
	UploadDone       UploadState = "done"
	UploadFailed     UploadState = "failed"
	UploadExpired    UploadState = "expired"
)

var (
	ErrUploadNotFound        = errors.New("upload not found")
	ErrUploadExpired         = errors.New("upload expired")
	ErrUploadNotOpen         = errors.New("upload is not open")
	ErrUploadIncomplete      = errors.New("upload is incomplete")
	ErrUploadInvalid         = errors.New("invalid upload request")
	ErrUploadRange           = errors.New("invalid upload part range")
	ErrUploadTooLarge        = errors.New("upload exceeds size limit")
	ErrUploadExists          = errors.New("file already exists")
	ErrUploadPathIsDirectory = errors.New("path is a directory")
)

type UploadOffsetError struct {
	Offset int64
}

func (e *UploadOffsetError) Error() string {
	return fmt.Sprintf("upload offset mismatch, expected offset %d", e.Offset)
}

type UploadRequest struct {
	Path      string
	Size      int64
	Overwrite bool
	IfMatch   string
	Mode      string
	Storage   string
	Erasure   *erasure.StoreOptions
	NoErasure bool
}

type UploadPart struct {
	Start int64
	End   int64
	Total int64
}

type UploadSession struct {
	ID        string                `json:"id"`
	UserID    string                `json:"user_id"`
	Path      string                `json:"path"`
	Size      int64                 `json:"size"`
	Offset    int64                 `json:"offset"`
	Overwrite bool                  `json:"overwrite"`
	IfMatch   string                `json:"if_match"`
	Mode      string                `json:"mode"`
	Storage   string                `json:"storage"`
	Erasure   *erasure.StoreOptions `json:"erasure"`
	State     UploadState           `json:"state"`
	ExpiresAt time.Time             `json:"expires_at"`
}

type UploadResult struct {
	Path    string
	ETag    string
	Created bool
}

type UploadEngine interface {
	GetMetadata(ctx context.Context, path string) (*metadata.Metadata, error)
	PutFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata, ifMatch string, ifNoneMatch bool) (*metadata.Metadata, bool, error)
	CreateErasureFile(ctx context.Context, path string, r io.Reader, opts *erasure.StoreOptions, build func(*erasure.ErasureFileInfo) *metadata.Metadata) (*metadata.Metadata, error)
}

type Uploads struct {
	engine         UploadEngine
	authorizer     auth.Authorizer
	spoolRoot      string
	defaultBackend string
	logger         *zap.Logger
	mu             sync.Mutex
	locks          map[string]*sync.Mutex
}

func NewUploads(engine UploadEngine, authorizer auth.Authorizer, localRoot, defaultBackend string, logger *zap.Logger) (*Uploads, error) {
	spoolRoot := filepath.Join(localRoot, uploadSpoolDirName)
	if err := os.MkdirAll(spoolRoot, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create upload spool directory: %w", err)
	}
	return &Uploads{
		engine:         engine,
		authorizer:     authorizer,
		spoolRoot:      spoolRoot,
		defaultBackend: defaultBackend,
		logger:         logger,
		locks:          make(map[string]*sync.Mutex),
	}, nil
}

func (u *Uploads) Logger() *zap.Logger {
	return u.logger
}

func (u *Uploads) Create(ctx context.Context, userID string, req UploadRequest) (UploadSession, error) {
	if "" == req.Mode {
		req.Mode = uploadDefaultMode
	}
	if err := validateUploadRequest(req); err != nil {
		return UploadSession{}, err
	}

	existing, err := u.engine.GetMetadata(ctx, req.Path)
	if nil != err {
		if !errors.Is(err, metadata.ErrNotFound) {
			return UploadSession{}, fmt.Errorf("failed to get existing metadata: %w", err)
		}
		existing = nil
	}
	if err := u.authorizer.AuthorizeWithMetadata(ctx, userID, req.Path, existing, auth.WritePerm); nil != err {
		return UploadSession{}, err
	}
	if nil != existing {
		if "file" != existing.Type {
			return UploadSession{}, ErrUploadPathIsDirectory
		}
		if !req.Overwrite && "" == req.IfMatch {
			return UploadSession{}, ErrUploadExists
		}
	}

	if nil == req.Erasure && !req.NoErasure && nil == existing && u.autoSplit(req.Size) {
		req.Erasure = &erasure.StoreOptions{}
	}

	id, err := newUploadID()
	if nil != err {
		return UploadSession{}, err
	}
	session := UploadSession{
		ID:        id,
		UserID:    userID,
		Path:      req.Path,
		Size:      req.Size,
		Overwrite: req.Overwrite,
		IfMatch:   req.IfMatch,
		Mode:      req.Mode,
		Storage:   req.Storage,
		Erasure:   req.Erasure,
		State:     UploadOpen,
		ExpiresAt: time.Now().UTC().Add(UploadSessionTTL),
	}

	dir := u.sessionDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return UploadSession{}, fmt.Errorf("failed to create upload directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, uploadDataName), nil, 0o600); err != nil {
		return UploadSession{}, errors.Join(fmt.Errorf("failed to create upload data file: %w", err), os.RemoveAll(dir))
	}
	if err := u.save(session); nil != err {
		return UploadSession{}, errors.Join(err, os.RemoveAll(dir))
	}
	return session, nil
}

func (u *Uploads) Status(userID, id string) (UploadSession, error) {
	session, err := u.loadOwned(userID, id)
	if nil != err {
		return UploadSession{}, err
	}
	if UploadOpen == session.State && time.Now().After(session.ExpiresAt) {
		session.State = UploadExpired
	}
	return session, nil
}

func (u *Uploads) PutPart(userID, id string, part UploadPart, body io.Reader) (UploadSession, error) {
	release := u.lock(id)
	defer release()

	session, err := u.loadOwned(userID, id)
	if nil != err {
		return UploadSession{}, err
	}
	if time.Now().After(session.ExpiresAt) {
		return UploadSession{}, ErrUploadExpired
	}
	if UploadOpen != session.State {
		return UploadSession{}, ErrUploadNotOpen
	}
	if part.Total != session.Size {
		return UploadSession{}, fmt.Errorf("%w: total must equal upload size %d", ErrUploadRange, session.Size)
	}
	if part.Start != session.Offset {
		return UploadSession{}, &UploadOffsetError{Offset: session.Offset}
	}
	if part.End < part.Start || part.End >= part.Total {
		return UploadSession{}, fmt.Errorf("%w: end must be between start and total-1", ErrUploadRange)
	}
	length := part.End - part.Start + 1
	if UploadPartBytes < length {
		return UploadSession{}, ErrUploadTooLarge
	}

	hasher, hashErr := u.loadHash(id, part.Start)
	if nil != hashErr {
		return UploadSession{}, hashErr
	}
	file, err := os.OpenFile(filepath.Join(u.sessionDir(id), uploadDataName), os.O_WRONLY, 0)
	if nil != err {
		return UploadSession{}, fmt.Errorf("failed to open upload data file: %w", err)
	}
	if nil != hasher {
		body = io.TeeReader(body, hasher)
	}
	written, writeErr := writeUploadPart(file, part.Start, length, body)
	closeErr := file.Close()
	if nil != writeErr {
		if errors.Is(writeErr, io.EOF) {
			return UploadSession{}, fmt.Errorf("%w: body is shorter than the Content-Range", ErrUploadRange)
		}
		return UploadSession{}, fmt.Errorf("failed to write upload part after %d bytes: %w", written, writeErr)
	}
	if nil != closeErr {
		return UploadSession{}, fmt.Errorf("failed to close upload data file: %w", closeErr)
	}

	if nil != hasher {
		if err := u.saveHash(id, part.End+1, hasher); nil != err {
			return UploadSession{}, err
		}
	}

	session.Offset = part.End + 1
	if err := u.save(session); nil != err {
		return UploadSession{}, err
	}
	return session, nil
}

func writeUploadPart(file *os.File, start, length int64, body io.Reader) (int64, error) {
	if _, err := file.Seek(start, io.SeekStart); nil != err {
		return 0, fmt.Errorf("failed to seek upload data file: %w", err)
	}
	written, err := copyPooled(file, io.LimitReader(body, length))
	if nil == err && written < length {
		err = io.EOF
	}
	return written, err
}

func (u *Uploads) loadHash(id string, offset int64) (hash.Hash, error) {
	hasher := sha256.New()
	if 0 == offset {
		return hasher, nil
	}
	raw, err := os.ReadFile(filepath.Join(u.sessionDir(id), uploadHashName))
	if nil != err {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read upload hash state: %w", err)
	}
	unmarshaler, ok := hasher.(encoding.BinaryUnmarshaler)
	if !ok {
		return nil, nil
	}
	if 8 > len(raw) || uint64(offset) != binary.BigEndian.Uint64(raw[:8]) {
		return nil, nil
	}
	if err := unmarshaler.UnmarshalBinary(raw[8:]); nil != err {
		return nil, nil
	}
	return hasher, nil
}

func (u *Uploads) saveHash(id string, offset int64, hasher hash.Hash) error {
	marshaler, ok := hasher.(encoding.BinaryMarshaler)
	if !ok {
		return nil
	}
	raw, err := marshaler.MarshalBinary()
	if nil != err {
		return fmt.Errorf("failed to encode upload hash state: %w", err)
	}
	framed := make([]byte, 8, 8+len(raw))
	binary.BigEndian.PutUint64(framed, uint64(offset))
	framed = append(framed, raw...)
	dir := u.sessionDir(id)
	tmp := filepath.Join(dir, uploadHashName+".tmp")
	if err := os.WriteFile(tmp, framed, 0o600); nil != err {
		return fmt.Errorf("failed to write upload hash state: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, uploadHashName)); nil != err {
		return fmt.Errorf("failed to replace upload hash state: %w", err)
	}
	return nil
}

func (u *Uploads) Complete(ctx context.Context, userID, id string) (UploadResult, error) {
	release := u.lock(id)
	defer release()

	session, err := u.loadOwned(userID, id)
	if nil != err {
		return UploadResult{}, err
	}
	if time.Now().After(session.ExpiresAt) {
		return UploadResult{}, ErrUploadExpired
	}
	if UploadOpen != session.State {
		return UploadResult{}, ErrUploadNotOpen
	}
	if session.Offset != session.Size {
		return UploadResult{}, fmt.Errorf("%w: %d of %d bytes received", ErrUploadIncomplete, session.Offset, session.Size)
	}

	session.State = UploadCompleting
	if err := u.save(session); nil != err {
		return UploadResult{}, err
	}

	committed, created, commitErr := u.commit(ctx, session)
	if nil != commitErr && !errors.Is(commitErr, ErrReplicaCommitPending) {
		session.State = UploadFailed
		if errors.Is(commitErr, metadata.ErrPrecondition) {
			session.State = UploadOpen
		}
		return UploadResult{}, errors.Join(commitErr, u.save(session))
	}

	session.State = UploadDone
	if err := u.save(session); nil != err {
		return UploadResult{}, errors.Join(commitErr, err)
	}
	if err := os.Remove(filepath.Join(u.sessionDir(id), uploadDataName)); nil != err && !errors.Is(err, os.ErrNotExist) {
		u.logger.Warn("Failed to remove completed upload data", zap.String("upload_id", id), zap.Error(err))
	}
	u.forget(id)

	result := UploadResult{Path: session.Path, Created: created}
	if nil != committed {
		result.ETag = committed.ETag
	}
	return result, commitErr
}

func (u *Uploads) commit(ctx context.Context, session UploadSession) (*metadata.Metadata, bool, error) {
	existing, err := u.engine.GetMetadata(ctx, session.Path)
	if nil != err {
		if !errors.Is(err, metadata.ErrNotFound) {
			return nil, false, fmt.Errorf("failed to get existing metadata: %w", err)
		}
		existing = nil
	}
	if err := u.authorizer.AuthorizeWithMetadata(ctx, session.UserID, session.Path, existing, auth.WritePerm); nil != err {
		return nil, false, err
	}

	file, err := os.Open(filepath.Join(u.sessionDir(session.ID), uploadDataName))
	if nil != err {
		return nil, false, fmt.Errorf("failed to open upload data file: %w", err)
	}
	defer file.Close()

	now := time.Now()
	if nil != session.Erasure {
		var source io.Reader = file
		stored, hashErr := u.loadHash(session.ID, session.Offset)
		if nil != hashErr {
			return nil, false, hashErr
		}
		if nil == stored {
			stored = sha256.New()
			source = io.TeeReader(file, stored)
		}
		committed, err := u.engine.CreateErasureFile(ctx, session.Path, source, session.Erasure, func(info *erasure.ErasureFileInfo) *metadata.Metadata {
			return &metadata.Metadata{
				Name:         path.Base(session.Path),
				Type:         "file",
				Size:         info.OriginalSize,
				Mode:         session.Mode,
				Owner:        session.UserID,
				BackendType:  "erasure",
				ErasureCoded: true,
				ATime:        now,
				MTime:        now,
				CTime:        now,
				ETag:         hex.EncodeToString(stored.Sum(nil)),
			}
		})
		return committed, nil == err, err
	}

	backend := session.Storage
	if "" == backend {
		backend = u.defaultBackend
	}
	md := &metadata.Metadata{
		Name:        path.Base(session.Path),
		Type:        "file",
		Size:        session.Size,
		Mode:        session.Mode,
		Owner:       session.UserID,
		BackendType: backend,
		ATime:       now,
		MTime:       now,
		CTime:       now,
	}
	ifNoneMatch := !session.Overwrite && "" == session.IfMatch
	return u.engine.PutFile(ctx, session.Path, file, session.Size, md, session.IfMatch, ifNoneMatch)
}

func (u *Uploads) Abort(userID, id string) error {
	release := u.lock(id)
	defer release()

	session, err := u.loadOwned(userID, id)
	if nil != err {
		return err
	}
	if UploadCompleting == session.State {
		return ErrUploadNotOpen
	}
	if err := os.RemoveAll(u.sessionDir(id)); nil != err {
		return fmt.Errorf("failed to remove upload spool: %w", err)
	}
	u.forget(id)
	return nil
}

func (u *Uploads) Cleanup(now time.Time) (int, error) {
	entries, err := os.ReadDir(u.spoolRoot)
	if nil != err {
		return 0, fmt.Errorf("failed to read upload spool: %w", err)
	}
	removed := 0
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() || !uploadIDPattern.MatchString(entry.Name()) {
			continue
		}
		gone, cleanupErr := u.cleanupEntry(entry.Name(), now)
		if nil != cleanupErr {
			errs = append(errs, cleanupErr)
			continue
		}
		if gone {
			removed++
		}
	}
	return removed, errors.Join(errs...)
}

func (u *Uploads) cleanupEntry(id string, now time.Time) (bool, error) {
	release := u.lock(id)
	defer release()

	session, err := u.load(id)
	switch {
	case errors.Is(err, ErrUploadNotFound):
		return false, nil
	case nil != err:
		return false, err
	}
	if !now.After(session.ExpiresAt) {
		return false, nil
	}
	if err := os.RemoveAll(u.sessionDir(id)); nil != err {
		return false, fmt.Errorf("failed to remove expired upload: %w", err)
	}
	u.forget(id)
	return true, nil
}

func (u *Uploads) loadOwned(userID, id string) (UploadSession, error) {
	session, err := u.load(id)
	if nil != err {
		return UploadSession{}, err
	}
	if session.UserID != userID {
		return UploadSession{}, ErrUploadNotFound
	}
	return session, nil
}

func (u *Uploads) load(id string) (UploadSession, error) {
	if !uploadIDPattern.MatchString(id) {
		return UploadSession{}, ErrUploadNotFound
	}
	raw, err := os.ReadFile(filepath.Join(u.sessionDir(id), uploadRecordName))
	if nil != err {
		if errors.Is(err, os.ErrNotExist) {
			return UploadSession{}, ErrUploadNotFound
		}
		return UploadSession{}, fmt.Errorf("failed to read upload record: %w", err)
	}
	var session UploadSession
	if err := json.Unmarshal(raw, &session); nil != err {
		return UploadSession{}, fmt.Errorf("failed to decode upload record: %w", err)
	}
	return session, nil
}

func (u *Uploads) save(session UploadSession) error {
	raw, err := json.Marshal(session)
	if nil != err {
		return fmt.Errorf("failed to encode upload record: %w", err)
	}
	dir := u.sessionDir(session.ID)
	tmp := filepath.Join(dir, uploadRecordName+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); nil != err {
		return fmt.Errorf("failed to write upload record: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, uploadRecordName)); nil != err {
		return fmt.Errorf("failed to replace upload record: %w", err)
	}
	return nil
}

func (u *Uploads) autoSplit(size int64) bool {
	source, ok := u.engine.(interface{ GetErasureManager() *erasure.Manager })
	if !ok || 0 >= size || MaxSplitUploadBytes < size {
		return false
	}
	em := source.GetErasureManager()
	return nil != em && em.AutoSplit(size)
}

func (u *Uploads) sessionDir(id string) string {
	return filepath.Join(u.spoolRoot, id)
}

func (u *Uploads) lock(id string) func() {
	u.mu.Lock()
	mu, ok := u.locks[id]
	if !ok {
		mu = &sync.Mutex{}
		u.locks[id] = mu
	}
	u.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (u *Uploads) forget(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.locks, id)
}

func validateUploadRequest(req UploadRequest) error {
	if 0 > req.Size {
		return fmt.Errorf("%w: size must not be negative", ErrUploadInvalid)
	}
	if MaxUploadBytes < req.Size {
		return ErrUploadTooLarge
	}
	if !uploadModePattern.MatchString(req.Mode) {
		return fmt.Errorf("%w: mode must be an octal permission like 0644", ErrUploadInvalid)
	}
	switch req.Storage {
	case "", "localfs", "s3":
	default:
		return fmt.Errorf("%w: storage must be localfs or s3", ErrUploadInvalid)
	}
	if nil == req.Erasure {
		return nil
	}
	if MaxSplitUploadBytes < req.Size {
		return ErrUploadTooLarge
	}
	return validateUploadErasure(req.Erasure)
}

func validateUploadErasure(opts *erasure.StoreOptions) error {
	if 0 > opts.DataShards || 0 > opts.ParityShards {
		return fmt.Errorf("%w: erasure shard counts must not be negative", ErrUploadInvalid)
	}
	if uploadMaxShards < opts.DataShards || uploadMaxShards < opts.ParityShards || uploadMaxShards < opts.DataShards+opts.ParityShards {
		return fmt.Errorf("%w: erasure shard counts must not exceed 255", ErrUploadInvalid)
	}
	if uploadMaxInstances < len(opts.Instances) {
		return fmt.Errorf("%w: erasure instance list must not exceed 64 entries", ErrUploadInvalid)
	}
	return nil
}

func newUploadID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); nil != err {
		return "", fmt.Errorf("failed to generate upload id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
