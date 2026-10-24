package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	trashDir           = "/.callfs/trash"
	trashScanBatch     = 500
	trashRecordLimit   = 64 << 10
	trashOwner         = "root"
	trashIDBytes       = 16
	trashConflictTries = 1000
	trashJobKind       = "trash_empty"
	trashFailedCode    = "DELETE_FAILED"
	trashPurgeGroup    = 64
)

var (
	ErrTrashInvalid  = errors.New("invalid trash request")
	ErrTrashConflict = errors.New("destination already exists")
	trashIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type Trash struct {
	engine     *Engine
	authorizer auth.Authorizer
	admins     *auth.AdminSet
	runner     *jobs.Runner
	retention  time.Duration
	logger     *zap.Logger
}

type TrashItem struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"original_path"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Size         int64     `json:"size"`
	DeletedBy    string    `json:"deleted_by"`
	DeletedAt    time.Time `json:"deleted_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TrashPage struct {
	Items      []TrashItem
	NextCursor string
	TotalBytes int64
}

func NewTrash(engine *Engine, authorizer auth.Authorizer, admins *auth.AdminSet, runner *jobs.Runner, retention time.Duration, logger *zap.Logger) *Trash {
	if nil == logger {
		logger = zap.NewNop()
	}
	return &Trash{
		engine:     engine,
		authorizer: authorizer,
		admins:     admins,
		runner:     runner,
		retention:  retention,
		logger:     logger,
	}
}

func (t *Trash) Logger() *zap.Logger {
	return t.logger
}

func (t *Trash) MoveToTrash(ctx context.Context, userID, filePath string) (TrashItem, error) {
	filePath = normalizeMovePath(filePath)
	if "/" == filePath || isTrashPath(filePath) {
		return TrashItem{}, fmt.Errorf("%w: %s cannot be moved to trash", ErrTrashInvalid, filePath)
	}
	md, err := t.engine.GetMetadata(ctx, filePath)
	if nil != err {
		return TrashItem{}, fmt.Errorf("failed to get metadata for %s: %w", filePath, err)
	}
	size, err := t.subtreeBytes(ctx, filePath, md)
	if nil != err {
		return TrashItem{}, err
	}
	id, err := newTrashID()
	if nil != err {
		return TrashItem{}, err
	}
	now := time.Now().UTC()
	name := path.Base(filePath)
	rec := TrashItem{
		ID:           id,
		OriginalPath: filePath,
		Name:         name,
		Type:         md.Type,
		Size:         size,
		DeletedBy:    userID,
		DeletedAt:    now,
		ExpiresAt:    now.Add(t.retention),
	}
	stored := childPath(trashItemDir(id), name)
	record, err := t.prepareRecord(ctx, rec)
	if nil != err {
		return TrashItem{}, err
	}
	err = t.engine.Move(ctx, filePath, stored, MoveOptions{
		CreateParents: true,
		extraOps:      []metadata.Op{record.createOp()},
		extraUndo:     []metadata.Op{{Kind: metadata.OpDelete, Path: record.path}},
		extraLocks:    []string{record.path},
	})
	if nil != err {
		t.engine.abortFile(ctx, record)
		var extrasErr *moveExtrasError
		if errors.As(err, &extrasErr) {
			return TrashItem{}, t.rollbackTrash(ctx, rec, stored, fmt.Errorf("failed to write trash record: %w", extrasErr.err))
		}
		return TrashItem{}, fmt.Errorf("failed to move %s to trash: %w", filePath, err)
	}
	if err := t.engine.finishFile(ctx, record); nil != err {
		return TrashItem{}, t.rollbackTrash(ctx, rec, stored, fmt.Errorf("failed to write trash record: %w", err))
	}
	t.logger.Info("item moved to trash", zap.String("id", id), zap.String("path", filePath), zap.String("deleted_by", userID))
	return rec, nil
}

func (t *Trash) List(ctx context.Context, userID, deletedBy, cursor string, limit int) (TrashPage, error) {
	filter, err := t.ownerFilter(userID, deletedBy)
	if nil != err {
		return TrashPage{}, err
	}
	after, err := decodeTrashCursor(cursor)
	if nil != err {
		return TrashPage{}, err
	}
	if 1 > limit {
		return TrashPage{}, fmt.Errorf("%w: limit must be positive", ErrTrashInvalid)
	}
	page := TrashPage{Items: make([]TrashItem, 0, limit)}
	err = t.walkRecords(ctx, after, func(rec TrashItem) (bool, error) {
		if "" != filter && filter != rec.DeletedBy {
			return false, nil
		}
		if limit == len(page.Items) {
			page.NextCursor = encodeTrashCursor(recordPath(page.Items[limit-1].ID))
			return true, nil
		}
		page.Items = append(page.Items, rec)
		return false, nil
	})
	if nil != err {
		return TrashPage{}, err
	}
	total, err := t.visibleBytes(ctx, filter)
	if nil != err {
		return TrashPage{}, err
	}
	page.TotalBytes = total
	return page, nil
}

func (t *Trash) Restore(ctx context.Context, userID, id, to, onConflict string) (string, error) {
	rec, err := t.load(ctx, id)
	if nil != err {
		return "", err
	}
	if !t.canManage(userID, rec.DeletedBy) {
		return "", fmt.Errorf("%w: trash item belongs to another user", auth.ErrPermissionDenied)
	}
	mode := onConflict
	if "" == mode {
		mode = "fail"
	}
	if "fail" != mode && "keep_both" != mode && "replace" != mode {
		return "", fmt.Errorf("%w: on_conflict must be fail, keep_both or replace", ErrTrashInvalid)
	}
	destination := rec.OriginalPath
	if "" != to {
		destination = normalizeMovePath(to)
		if "/" == destination || isTrashPath(destination) {
			return "", fmt.Errorf("%w: %s is not a valid restore path", ErrTrashInvalid, destination)
		}
	}
	existing, err := t.engine.GetMetadata(ctx, destination)
	if nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return "", fmt.Errorf("failed to get metadata for %s: %w", destination, err)
	}
	if nil != err {
		existing = nil
	}
	if err := t.authorizer.AuthorizeWithMetadata(ctx, userID, destination, existing, auth.WritePerm); nil != err {
		return "", err
	}
	if nil != existing {
		switch mode {
		case "fail":
			return "", fmt.Errorf("%w: %s", ErrTrashConflict, destination)
		case "keep_both":
			destination, err = t.restoredName(ctx, destination)
			if nil != err {
				return "", err
			}
		}
	}
	stored := childPath(trashItemDir(id), rec.Name)
	opts := MoveOptions{Overwrite: "replace" == mode, CreateParents: true}
	dirMd, recordMd, detach := t.detachableItem(ctx, id, stored)
	if detach {
		opts.extraOps = []metadata.Op{
			{Kind: metadata.OpDelete, Path: dirMd.Path},
			{Kind: metadata.OpDelete, Path: recordMd.Path},
		}
		opts.extraUndo = []metadata.Op{
			{Kind: metadata.OpCreate, Metadata: dirMd},
			{Kind: metadata.OpCreate, Metadata: recordMd},
		}
		opts.extraLocks = []string{dirMd.Path, recordMd.Path}
	}
	var extrasErr *moveExtrasError
	if err := t.engine.Move(ctx, stored, destination, opts); errors.As(err, &extrasErr) {
		detach = false
	} else if nil != err {
		return "", fmt.Errorf("failed to restore %s: %w", rec.OriginalPath, err)
	}
	if detach {
		if err := t.engine.removeDetachedBytes(ctx, dirMd.Path, dirMd); nil != err {
			return destination, fmt.Errorf("failed to remove trash folder %s: %w", id, err)
		}
		if err := t.engine.removeDetachedBytes(ctx, recordMd.Path, recordMd); nil != err {
			return destination, fmt.Errorf("failed to remove trash record %s: %w", id, err)
		}
	} else if err := t.removeEmptyItem(ctx, id); nil != err {
		return destination, err
	}
	t.logger.Info("trash item restored", zap.String("id", id), zap.String("path", destination))
	return destination, nil
}

func (t *Trash) Delete(ctx context.Context, userID, id string) error {
	rec, err := t.load(ctx, id)
	if nil != err {
		return err
	}
	if !t.canManage(userID, rec.DeletedBy) {
		return fmt.Errorf("%w: trash item belongs to another user", auth.ErrPermissionDenied)
	}
	if err := t.purge(ctx, rec.ID); nil != err {
		return err
	}
	t.logger.Info("trash item deleted for good", zap.String("id", id), zap.String("deleted_by", rec.DeletedBy))
	return nil
}

func (t *Trash) Empty(ctx context.Context, userID, deletedBy string) (jobs.Job, error) {
	filter, err := t.ownerFilter(userID, deletedBy)
	if nil != err {
		return jobs.Job{}, err
	}
	return t.runner.Submit(trashJobKind, userID, func(jobCtx context.Context, p *jobs.Progress) (any, error) {
		return t.emptyMatching(jobCtx, filter, p)
	}), nil
}

func (t *Trash) Cleanup(ctx context.Context, retention time.Duration) (int, error) {
	now := time.Now()
	removed := 0
	var purgeErrs []error
	err := t.walkRecords(ctx, "", func(rec TrashItem) (bool, error) {
		if now.Before(rec.DeletedAt.Add(retention)) {
			return false, nil
		}
		if err := t.purge(ctx, rec.ID); nil != err {
			purgeErrs = append(purgeErrs, err)
			return false, nil
		}
		removed++
		return false, nil
	})
	if 0 < removed {
		t.logger.Info("trash cleanup removed expired items", zap.Int("removed", removed))
	}
	return removed, errors.Join(err, errors.Join(purgeErrs...))
}

func (t *Trash) emptyMatching(ctx context.Context, filter string, p *jobs.Progress) (any, error) {
	t.logger.Info("trash empty started", zap.String("deleted_by", filter))
	deleted := 0
	group := make([]TrashItem, 0, trashPurgeGroup)
	flush := func() {
		if t.purgeGroup(ctx, group) {
			deleted += len(group)
			p.Advance(len(group))
			group = group[:0]
			return
		}
		for _, rec := range group {
			if err := t.purge(ctx, rec.ID); nil != err {
				p.Fail(jobs.FailedItem{Path: rec.OriginalPath, Code: trashFailedCode, Reason: err.Error()})
				continue
			}
			deleted++
			p.Advance(1)
		}
		group = group[:0]
	}
	err := t.walkRecords(ctx, "", func(rec TrashItem) (bool, error) {
		if "" != filter && filter != rec.DeletedBy {
			return false, nil
		}
		group = append(group, rec)
		if trashPurgeGroup == len(group) {
			flush()
		}
		return false, nil
	})
	if nil == err && 0 < len(group) {
		flush()
	}
	if nil != err {
		t.logger.Error("trash empty failed", zap.Int("deleted", deleted), zap.Error(err))
		return nil, err
	}
	t.logger.Info("trash empty finished", zap.Int("deleted", deleted))
	return map[string]int{"deleted": deleted}, nil
}

func (t *Trash) ownerFilter(userID, deletedBy string) (string, error) {
	if "" == userID {
		return "", auth.ErrAuthenticationFailed
	}
	if t.admins.IsAdmin(userID) {
		if "*" == deletedBy {
			return "", nil
		}
		return deletedBy, nil
	}
	if "" != deletedBy && "*" != deletedBy && userID != deletedBy {
		return "", fmt.Errorf("%w: trash of another user", auth.ErrPermissionDenied)
	}
	return userID, nil
}

func (t *Trash) canManage(userID, deletedBy string) bool {
	return "" != userID && (t.admins.IsAdmin(userID) || userID == deletedBy)
}

func (t *Trash) subtreeBytes(ctx context.Context, dirPath string, md *metadata.Metadata) (int64, error) {
	if "directory" != md.Type {
		return md.Size, nil
	}
	descendants, err := t.engine.metadataStore.ListDescendants(ctx, dirPath, 0, 0)
	if nil != err {
		return 0, fmt.Errorf("failed to list %s: %w", dirPath, err)
	}
	var total int64
	for _, item := range descendants {
		if "file" == item.Type {
			total += item.Size
		}
	}
	return total, nil
}

func (t *Trash) visibleBytes(ctx context.Context, filter string) (int64, error) {
	var total int64
	err := t.walkRecords(ctx, "", func(rec TrashItem) (bool, error) {
		if "" == filter || filter == rec.DeletedBy {
			total += rec.Size
		}
		return false, nil
	})
	return total, err
}

func (t *Trash) walkRecords(ctx context.Context, after string, visit func(TrashItem) (bool, error)) error {
	for {
		rows, err := t.engine.metadataStore.ListByPrefix(ctx, trashDir+"/", after, trashScanBatch)
		if nil != err {
			return fmt.Errorf("failed to list trash: %w", err)
		}
		for _, row := range rows {
			after = row.Path
			id, ok := recordID(row.Path)
			if !ok {
				continue
			}
			rec, err := t.readRecord(ctx, id)
			if errors.Is(err, metadata.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if nil != err {
				return err
			}
			stop, err := visit(rec)
			if nil != err || stop {
				return err
			}
		}
		if len(rows) < trashScanBatch {
			return nil
		}
	}
}

func (t *Trash) load(ctx context.Context, id string) (TrashItem, error) {
	if !trashIDPattern.MatchString(id) {
		return TrashItem{}, fmt.Errorf("%w: malformed trash id", ErrTrashInvalid)
	}
	return t.readRecord(ctx, id)
}

func (t *Trash) readRecord(ctx context.Context, id string) (TrashItem, error) {
	rc, err := t.engine.GetFile(ctx, recordPath(id))
	if nil != err {
		return TrashItem{}, fmt.Errorf("failed to open trash record %s: %w", id, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, trashRecordLimit))
	if nil != err {
		return TrashItem{}, fmt.Errorf("failed to read trash record %s: %w", id, err)
	}
	var rec TrashItem
	if err := json.Unmarshal(data, &rec); nil != err {
		return TrashItem{}, fmt.Errorf("failed to decode trash record %s: %w", id, err)
	}
	if id != rec.ID || "" == rec.Name || "." == rec.Name || ".." == rec.Name || strings.ContainsRune(rec.Name, '/') {
		return TrashItem{}, fmt.Errorf("trash record %s is malformed", id)
	}
	return rec, nil
}

func (t *Trash) prepareRecord(ctx context.Context, rec TrashItem) (*pendingFile, error) {
	data, err := json.Marshal(rec)
	if nil != err {
		return nil, fmt.Errorf("failed to write trash record: failed to encode trash record: %w", err)
	}
	md := &metadata.Metadata{
		Name:        rec.ID + ".json",
		Type:        "file",
		Mode:        "0600",
		Owner:       trashOwner,
		BackendType: "localfs",
	}
	record, err := t.engine.prepareFile(ctx, recordPath(rec.ID), bytes.NewReader(data), int64(len(data)), md)
	if nil != err {
		return nil, fmt.Errorf("failed to write trash record: %w", err)
	}
	return record, nil
}

func (t *Trash) detachableItem(ctx context.Context, id, stored string) (*metadata.Metadata, *metadata.Metadata, bool) {
	dirMd, err := t.engine.metadataStore.Get(ctx, trashItemDir(id))
	if nil != err || "directory" != dirMd.Type || isErasureMetadata(dirMd) {
		return nil, nil, false
	}
	recordMd, err := t.engine.metadataStore.Get(ctx, recordPath(id))
	if nil != err || "file" != recordMd.Type || isErasureMetadata(recordMd) {
		return nil, nil, false
	}
	children, err := t.engine.metadataStore.ListChildren(ctx, dirMd.Path)
	if nil != err || 1 != len(children) || stored != children[0].Path {
		return nil, nil, false
	}
	return dirMd, recordMd, true
}

func (t *Trash) rollbackTrash(ctx context.Context, rec TrashItem, stored string, cause error) error {
	if err := t.engine.Move(ctx, stored, rec.OriginalPath, MoveOptions{}); nil != err {
		return errors.Join(cause, fmt.Errorf("failed to move %s back from trash: %w", rec.OriginalPath, err))
	}
	if err := t.engine.DeleteFile(ctx, trashItemDir(rec.ID)); nil != err {
		return errors.Join(cause, err)
	}
	return cause
}

func (t *Trash) removeEmptyItem(ctx context.Context, id string) error {
	if err := t.engine.DeleteFile(ctx, trashItemDir(id)); nil != err {
		return fmt.Errorf("failed to remove trash folder %s: %w", id, err)
	}
	if err := t.engine.DeleteFile(ctx, recordPath(id)); nil != err {
		return fmt.Errorf("failed to remove trash record %s: %w", id, err)
	}
	return nil
}

func (t *Trash) purgeGroup(ctx context.Context, recs []TrashItem) bool {
	targets := make([]deleteTarget, 0, len(recs)*2)
	for _, rec := range recs {
		recTargets, ok := t.purgeTargets(ctx, rec.ID)
		if !ok {
			return false
		}
		targets = append(targets, recTargets...)
	}
	return nil == t.engine.deleteTargets(ctx, targets).err
}

func (t *Trash) purgeTargets(ctx context.Context, id string) ([]deleteTarget, bool) {
	targets := make([]deleteTarget, 0, 2)
	_, err := t.engine.metadataStore.Get(ctx, trashItemDir(id))
	switch {
	case nil == err:
		tree, err := t.engine.treeDeleteTargets(ctx, trashItemDir(id))
		if nil != err {
			return nil, false
		}
		targets = append(targets, tree...)
	case !errors.Is(err, metadata.ErrNotFound):
		return nil, false
	}
	_, err = t.engine.metadataStore.Get(ctx, recordPath(id))
	switch {
	case nil == err:
		targets = append(targets, deleteTarget{path: recordPath(id)})
	case !errors.Is(err, metadata.ErrNotFound):
		return nil, false
	}
	return targets, true
}

func (t *Trash) purge(ctx context.Context, id string) error {
	if targets, ok := t.purgeTargets(ctx, id); ok && nil == t.engine.deleteTargets(ctx, targets).err {
		return nil
	}
	return t.purgeSequential(ctx, id)
}

func (t *Trash) purgeSequential(ctx context.Context, id string) error {
	if _, _, err := t.engine.DeleteDirectoryRecursive(ctx, trashItemDir(id)); nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return fmt.Errorf("failed to delete trash contents %s: %w", id, err)
	}
	if err := t.engine.DeleteFile(ctx, recordPath(id)); nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return fmt.Errorf("failed to delete trash record %s: %w", id, err)
	}
	return nil
}

func (t *Trash) restoredName(ctx context.Context, destination string) (string, error) {
	base := path.Base(destination)
	dir := dirOf(destination)
	for attempt := 1; attempt <= trashConflictTries; attempt++ {
		name := base + " (restored)"
		if 1 < attempt {
			name = fmt.Sprintf("%s (restored %d)", base, attempt)
		}
		candidate := childPath(dir, name)
		_, err := t.engine.GetMetadata(ctx, candidate)
		if errors.Is(err, metadata.ErrNotFound) {
			return candidate, nil
		}
		if nil != err {
			return "", fmt.Errorf("failed to get metadata for %s: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("%w: no free name for %s", ErrTrashConflict, destination)
}

func newTrashID() (string, error) {
	buf := make([]byte, trashIDBytes)
	if _, err := rand.Read(buf); nil != err {
		return "", fmt.Errorf("failed to generate trash id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func recordID(recPath string) (string, bool) {
	rest, ok := strings.CutPrefix(recPath, trashDir+"/")
	if !ok {
		return "", false
	}
	id, ok := strings.CutSuffix(rest, ".json")
	if !ok || !trashIDPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

func recordPath(id string) string {
	return trashDir + "/" + id + ".json"
}

func trashItemDir(id string) string {
	return trashDir + "/" + id
}

func isTrashPath(p string) bool {
	return p == trashDir || strings.HasPrefix(p, trashDir+"/")
}

func childPath(dir, name string) string {
	if "/" == dir {
		return "/" + name
	}
	return dir + "/" + name
}

func encodeTrashCursor(p string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(p))
}

func decodeTrashCursor(cursor string) (string, error) {
	if "" == cursor {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err {
		return "", fmt.Errorf("%w: malformed cursor", ErrTrashInvalid)
	}
	decoded := string(raw)
	if !strings.HasPrefix(decoded, trashDir+"/") {
		return "", fmt.Errorf("%w: malformed cursor", ErrTrashInvalid)
	}
	return decoded, nil
}
