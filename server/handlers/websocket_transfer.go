package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
)

const wsChunkSize = 64 * 1024

const wsMaxUploadBytes = 100 << 20

var wsChunkPool = sync.Pool{
	New: func() any {
		buf := make([]byte, wsChunkSize)
		return &buf
	},
}

var errWSUploadTooLarge = errors.New("websocket upload too large")

var errWSUploadSizeMismatch = errors.New("websocket upload size does not match declared size")

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  wsChunkSize,
	WriteBufferSize: wsChunkSize,
	WriteBufferPool: &sync.Pool{},
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Non-browser clients
		}
		// Parse origin as URL and compare host exactly to prevent substring bypass
		// (e.g. evil.callfs.internal:8443.attacker.com matching callfs.internal:8443)
		originHost := origin
		if idx := strings.Index(origin, "://"); idx >= 0 {
			originHost = origin[idx+3:]
		}
		// Strip any path component
		if idx := strings.Index(originHost, "/"); idx >= 0 {
			originHost = originHost[:idx]
		}
		return originHost == r.Host
	},
}

const (
	wsMaxConcurrentUploads = 64
	wsMaxUploadsPerUser    = 4
	wsIOTimeout            = 30 * time.Second
	wsMaxIgnoredFrames     = 64
	wsCloseConflict        = 4409
)

var errWSTooManyIgnoredFrames = errors.New("websocket connection sent too many non-binary frames")

const (
	wsProgressInterval int64 = 1 << 20
	wsAckInterval      int64 = 8 << 20
)

func wsWriteText(conn *websocket.Conn, message string) error {
	if err := conn.SetWriteDeadline(time.Now().Add(wsIOTimeout)); nil != err {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func wsSendIfDue(conn *websocket.Conn, prefix string, count, interval int64, next *int64) error {
	if *next > count {
		return nil
	}
	if err := wsWriteText(conn, prefix+strconv.FormatInt(count, 10)); nil != err {
		return err
	}
	*next = (count/interval + 1) * interval
	return nil
}

var (
	wsUploadSlotsMu     sync.Mutex
	wsUploadSlotsActive int
	wsUploadSlotsByUser = make(map[string]int)

	wsSlotCountersMu sync.RWMutex
	wsSlotCounters   SlotCounters
)

const (
	wsSlotScopeTotal = "ws_total"
	wsSlotScopeUser  = "ws_user"
	wsSlotKeyAll     = "all"
)

type SlotCounters interface {
	Acquire(ctx context.Context, scope, key string, limit int) (bool, error)
	Release(ctx context.Context, scope, key string) error
	ReportFailure(op string, err error)
}

func SetWebSocketSlotCounters(counters SlotCounters) {
	wsSlotCountersMu.Lock()
	defer wsSlotCountersMu.Unlock()
	wsSlotCounters = counters
}

func currentWebSocketSlotCounters() SlotCounters {
	wsSlotCountersMu.RLock()
	defer wsSlotCountersMu.RUnlock()
	return wsSlotCounters
}

func wsAcquireClusterSlot(ctx context.Context, counters SlotCounters, userID string) (release func(), acquired, decided bool) {
	totalOK, err := counters.Acquire(ctx, wsSlotScopeTotal, wsSlotKeyAll, wsMaxConcurrentUploads)
	if nil != err {
		counters.ReportFailure("websocket_slots", err)
		return nil, false, false
	}
	if !totalOK {
		return nil, false, true
	}
	releaseTotal := func() {
		if relErr := counters.Release(context.Background(), wsSlotScopeTotal, wsSlotKeyAll); nil != relErr {
			counters.ReportFailure("websocket_slots", relErr)
		}
	}
	userOK, err := counters.Acquire(ctx, wsSlotScopeUser, userID, wsMaxUploadsPerUser)
	if nil != err {
		counters.ReportFailure("websocket_slots", err)
		releaseTotal()
		return nil, false, false
	}
	if !userOK {
		releaseTotal()
		return nil, false, true
	}
	return func() {
		if relErr := counters.Release(context.Background(), wsSlotScopeUser, userID); nil != relErr {
			counters.ReportFailure("websocket_slots", relErr)
		}
		releaseTotal()
	}, true, true
}

func wsAcquireUploadSlot(ctx context.Context, userID string) (func(), bool) {
	if counters := currentWebSocketSlotCounters(); nil != counters {
		if release, acquired, decided := wsAcquireClusterSlot(ctx, counters, userID); decided {
			return release, acquired
		}
	}
	wsUploadSlotsMu.Lock()
	defer wsUploadSlotsMu.Unlock()
	if wsMaxConcurrentUploads <= wsUploadSlotsActive || wsMaxUploadsPerUser <= wsUploadSlotsByUser[userID] {
		return nil, false
	}
	wsUploadSlotsActive++
	wsUploadSlotsByUser[userID]++
	return func() { wsReleaseUploadSlot(userID) }, true
}

func wsReleaseUploadSlot(userID string) {
	wsUploadSlotsMu.Lock()
	defer wsUploadSlotsMu.Unlock()
	wsUploadSlotsActive--
	wsUploadSlotsByUser[userID]--
	if 0 == wsUploadSlotsByUser[userID] {
		delete(wsUploadSlotsByUser, userID)
	}
}

func transferClient(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if nil != err || "" == host {
		return r.RemoteAddr
	}
	return host
}

func wsCloseWithError(conn *websocket.Conn, code int, msg string) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, msg),
		time.Now().Add(5*time.Second))
}

func wsCloseUploadError(conn *websocket.Conn, err error) {
	if errors.Is(err, metadata.ErrAlreadyExists) {
		wsCloseWithError(conn, wsCloseConflict, "file already exists")
		return
	}
	wsCloseWithError(conn, websocket.CloseInternalServerErr, "file write failed")
}

func wsDownload(conn *websocket.Conn, r *http.Request, engine *core.Engine, authorizer auth.Authorizer, userID, enginePath string, logger *zap.Logger) {
	if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.ReadPerm); err != nil {
		wsCloseWithError(conn, websocket.ClosePolicyViolation, "read access denied")
		return
	}

	md, err := engine.GetMetadata(r.Context(), enginePath)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			wsCloseWithError(conn, websocket.ClosePolicyViolation, "file not found")
			return
		}
		wsCloseWithError(conn, websocket.CloseInternalServerErr, "failed to open file")
		return
	}
	if "file" != md.Type {
		wsCloseWithError(conn, websocket.CloseUnsupportedData, "path is not a file")
		return
	}

	reader, err := engine.GetFile(r.Context(), enginePath)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			wsCloseWithError(conn, websocket.ClosePolicyViolation, "file not found")
			return
		}
		if errors.Is(err, core.ErrNotAFile) {
			wsCloseWithError(conn, websocket.CloseUnsupportedData, "path is not a file")
			return
		}
		wsCloseWithError(conn, websocket.CloseInternalServerErr, "failed to open file")
		return
	}
	defer reader.Close()

	if err := wsWriteText(conn, "size:"+strconv.FormatInt(md.Size, 10)); nil != err {
		logger.Warn("Failed writing websocket download size", zap.Error(err))
		return
	}

	transfer := telemetry.Transfers.Begin("websocket_download", userID, enginePath, transferClient(r), md.Size)
	defer transfer.Finish()

	bufPtr := wsChunkPool.Get().(*[]byte)
	defer wsChunkPool.Put(bufPtr)
	buf := *bufPtr
	var sent int64
	nextProgress := wsProgressInterval
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(wsIOTimeout)); nil != err {
				logger.Warn("Failed arming websocket write deadline", zap.Error(err))
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				logger.Warn("Failed writing websocket download chunk", zap.Error(err))
				return
			}
			sent += int64(n)
			transfer.Add(int64(n))
			if err := wsSendIfDue(conn, "progress:", sent, wsProgressInterval, &nextProgress); nil != err {
				logger.Warn("Failed writing websocket download progress", zap.Error(err))
				return
			}
		}

		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			wsCloseWithError(conn, websocket.CloseNormalClosure, "download complete")
		} else {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file read failed")
		}
		return
	}
}

// uploadSpool holds a websocket upload while it is being received. Small uploads
// stay in memory; anything larger spills to a temp file.
//
// Buffering the whole payload in memory made peak usage the product of upload
// size and concurrency: any authenticated client could open many upload sockets
// and push the server toward OOM, since nothing caps concurrent connections.
// Spooling to disk past a threshold bounds memory per connection regardless of
// how many are open, while still giving CreateFile/UpdateFile the byte count they
// require up front.
type uploadSpool struct {
	buf    bytes.Buffer
	file   *os.File
	writer *bufio.Writer
	size   int64
}

// wsSpillThreshold is the point at which an in-progress upload moves to disk.
const wsSpillThreshold = 8 << 20 // 8 MiB

func (s *uploadSpool) spill() error {
	f, err := os.CreateTemp("", "callfs-ws-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create upload spool file: %w", err)
	}
	// Unlink immediately: the descriptor keeps the data reachable, and the
	// space is reclaimed by the OS even if this process dies mid-upload.
	if err := os.Remove(f.Name()); err != nil {
		f.Close()
		return fmt.Errorf("failed to unlink upload spool file: %w", err)
	}
	writer := bufio.NewWriterSize(f, wsChunkSize)
	if _, err := writer.Write(s.buf.Bytes()); err != nil {
		f.Close()
		return fmt.Errorf("failed to spill upload to disk: %w", err)
	}
	s.buf = bytes.Buffer{}
	s.file = f
	s.writer = writer
	return nil
}

func (s *uploadSpool) Write(p []byte) error {
	if s.file == nil && s.size+int64(len(p)) > wsSpillThreshold {
		if err := s.spill(); err != nil {
			return err
		}
	}

	var err error
	if s.file != nil {
		_, err = s.writer.Write(p)
	} else {
		_, err = s.buf.Write(p)
	}
	if err != nil {
		return err
	}
	s.size += int64(len(p))
	return nil
}

// Reader rewinds the spool and returns a reader over the full payload.
func (s *uploadSpool) Reader() (io.Reader, error) {
	if s.file == nil {
		return bytes.NewReader(s.buf.Bytes()), nil
	}
	if err := s.writer.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush upload spool: %w", err)
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to rewind upload spool: %w", err)
	}
	return s.file, nil
}

// Close releases the spool's temp file, if one was created.
func (s *uploadSpool) Close() {
	if s.file != nil {
		s.file.Close()
		s.file = nil
		s.writer = nil
	}
}

// wsCopyMessage copies one websocket message into write in buf-sized pieces.
// received is the byte count already transferred for the upload and limit is
// the most bytes the upload may carry in total; the returned count includes
// this message.
func wsCopyMessage(src io.Reader, buf []byte, received, limit int64, write func([]byte) error) (int64, error) {
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if received+int64(n) > limit {
				return received, errWSUploadTooLarge
			}
			if err := write(buf[:n]); err != nil {
				return received, err
			}
			received += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			return received, nil
		}
		if readErr != nil {
			return received, readErr
		}
	}
}

func wsReadUploadPayload(conn *websocket.Conn, logger *zap.Logger) (*uploadSpool, bool) {
	spool := &uploadSpool{}
	bufPtr := wsChunkPool.Get().(*[]byte)
	defer wsChunkPool.Put(bufPtr)
	ignoredFrames := 0
	nextAck := wsAckInterval
	for {
		if err := conn.SetReadDeadline(time.Now().Add(wsIOTimeout)); nil != err {
			logger.Warn("Failed arming websocket read deadline", zap.Error(err))
			spool.Close()
			return nil, false
		}
		messageType, reader, readErr := conn.NextReader()
		if readErr != nil {
			if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return spool, true
			}
			logger.Warn("Failed reading websocket upload message", zap.Error(readErr))
			spool.Close()
			return nil, false
		}
		if messageType != websocket.BinaryMessage {
			ignoredFrames++
			if wsMaxIgnoredFrames < ignoredFrames {
				wsCloseWithError(conn, websocket.ClosePolicyViolation, "too many non-binary frames")
				spool.Close()
				return nil, false
			}
			continue
		}
		if _, err := wsCopyMessage(reader, *bufPtr, spool.size, wsMaxUploadBytes, spool.Write); err != nil {
			if errors.Is(err, errWSUploadTooLarge) {
				wsCloseWithError(conn, websocket.CloseMessageTooBig, "upload too large")
			} else {
				logger.Warn("Failed buffering websocket upload payload", zap.Error(err))
			}
			spool.Close()
			return nil, false
		}
		if err := wsSendIfDue(conn, "ack:", spool.size, wsAckInterval, &nextAck); nil != err {
			logger.Warn("Failed writing websocket upload ack", zap.Error(err))
			spool.Close()
			return nil, false
		}
	}
}

// wsStreamUpload forwards binary frames to w until the client closes the socket.
// The declared size bounds the payload; a short or oversized upload is an error.
func wsStreamUpload(conn *websocket.Conn, w io.Writer, declared int64) error {
	bufPtr := wsChunkPool.Get().(*[]byte)
	defer wsChunkPool.Put(bufPtr)

	var received int64
	nextAck := wsAckInterval
	ignoredFrames := 0
	write := func(p []byte) error {
		_, err := w.Write(p)
		return err
	}
	for {
		if err := conn.SetReadDeadline(time.Now().Add(wsIOTimeout)); nil != err {
			return err
		}
		messageType, reader, err := conn.NextReader()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				if received != declared {
					return errWSUploadSizeMismatch
				}
				return nil
			}
			return err
		}
		if messageType != websocket.BinaryMessage {
			ignoredFrames++
			if wsMaxIgnoredFrames < ignoredFrames {
				return errWSTooManyIgnoredFrames
			}
			continue
		}
		if received, err = wsCopyMessage(reader, *bufPtr, received, declared, write); err != nil {
			return err
		}
		if err := wsSendIfDue(conn, "ack:", received, wsAckInterval, &nextAck); nil != err {
			return err
		}
	}
}

func wsResolveUploadTarget(conn *websocket.Conn, ctx context.Context, engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, userID string, pathInfo PathInfo, enginePath string) (*metadata.Metadata, bool) {
	md, err := engine.GetMetadata(ctx, enginePath)
	if nil != err && !errors.Is(err, metadata.ErrNotFound) {
		wsCloseWithError(conn, websocket.CloseInternalServerErr, "metadata lookup failed")
		return nil, false
	}
	if err := authorizer.AuthorizeWithMetadata(ctx, userID, enginePath, md, auth.WritePerm); err != nil {
		wsCloseWithError(conn, websocket.ClosePolicyViolation, "write access denied")
		return nil, false
	}
	if nil == md {
		now := time.Now()
		return &metadata.Metadata{
			Name:        pathInfo.Name,
			Type:        "file",
			Mode:        "0644",
			Owner:       userID,
			BackendType: backendConfig.DefaultBackend,
			ATime:       now,
			MTime:       now,
			CTime:       now,
		}, true
	}
	if md.Type != "file" {
		wsCloseWithError(conn, websocket.CloseUnsupportedData, "target path is not a file")
		return nil, false
	}
	return md, true
}

func wsStreamToEngine(conn *websocket.Conn, r *http.Request, engine *core.Engine, md *metadata.Metadata, enginePath string, declared int64, transfer *telemetry.TransferHandle, logger *zap.Logger) (string, error) {
	pr, pw := io.Pipe()
	streamDone := make(chan error, 1)
	go func() {
		err := wsStreamUpload(conn, transfer.Writer(pw), declared)
		pw.CloseWithError(err)
		streamDone <- err
	}()

	committedMd, _, putErr := engine.PutFile(r.Context(), enginePath, pr, declared, md, "", false)
	pr.Close()
	streamErr := <-streamDone
	if nil != putErr && !errors.Is(putErr, core.ErrReplicaCommitPending) {
		return "", putErr
	}
	if nil != putErr {
		logger.Warn("Websocket upload committed with replica commit pending", zap.String("path", enginePath), zap.Error(putErr))
	}
	if streamErr != nil {
		return "", streamErr
	}
	return committedMd.ETag, nil
}

func wsAckUpload(conn *websocket.Conn, size int64, etag string, logger *zap.Logger) {
	err := conn.SetWriteDeadline(time.Now().Add(wsIOTimeout))
	if nil == err {
		err = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("ok:%d:%s", size, etag)))
	}
	if nil != err {
		logger.Warn("Failed writing websocket upload ack", zap.Error(err))
	}
	wsCloseWithError(conn, websocket.CloseNormalClosure, "upload complete")
}

func wsUpload(conn *websocket.Conn, r *http.Request, engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, userID string, pathInfo PathInfo, enginePath string, releaseSlot func(), logger *zap.Logger) {
	conn.SetCloseHandler(func(int, string) error { return nil })
	md, ok := wsResolveUploadTarget(conn, r.Context(), engine, authorizer, backendConfig, userID, pathInfo, enginePath)
	if !ok {
		return
	}

	transfer := telemetry.Transfers.Begin("websocket_upload", userID, enginePath, transferClient(r), -1)
	defer transfer.Finish()

	if rawSize := r.URL.Query().Get("size"); rawSize != "" {
		declared, err := strconv.ParseInt(rawSize, 10, 64)
		if err != nil || declared < 0 || declared > wsMaxUploadBytes {
			wsCloseWithError(conn, websocket.CloseUnsupportedData, "invalid upload size")
			return
		}
		transfer.SetTotal(declared)
		etag, err := wsStreamToEngine(conn, r, engine, md, enginePath, declared, transfer, logger)
		if err != nil {
			if errors.Is(err, errWSUploadTooLarge) {
				wsCloseWithError(conn, websocket.CloseMessageTooBig, "upload too large")
				return
			}
			logger.Warn("Failed streaming websocket upload", zap.Error(err))
			wsCloseUploadError(conn, err)
			return
		}
		releaseSlot()
		wsAckUpload(conn, declared, etag, logger)
		return
	}

	payload, ok := wsReadUploadPayload(conn, logger)
	if !ok {
		return
	}
	defer payload.Close()

	size := payload.size
	transfer.Add(size)
	reader, readerErr := payload.Reader()
	if readerErr != nil {
		logger.Error("Failed to read buffered websocket upload", zap.Error(readerErr))
		wsCloseWithError(conn, websocket.CloseInternalServerErr, "file write failed")
		return
	}
	committedMd, _, err := engine.PutFile(r.Context(), enginePath, reader, size, md, "", false)
	if nil != err {
		if !errors.Is(err, core.ErrReplicaCommitPending) {
			wsCloseUploadError(conn, err)
			return
		}
		logger.Warn("Websocket upload committed with replica commit pending", zap.String("path", enginePath), zap.Error(err))
	}

	releaseSlot()
	wsAckUpload(conn, size, committedMd.ETag, logger)
}

// V1WebSocketTransfer handles websocket file transfers on /v1/files/ws/{path}.
// Query param mode=download|upload controls transfer direction. Uploads may pass
// size=<bytes> to stream frames straight into the backend without spooling.
func V1WebSocketTransfer(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}
		if pathInfo.IsDirectory {
			SendErrorResponse(w, logger, &customError{message: "websocket transfer requires a file path"}, http.StatusBadRequest)
			return
		}

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
		if mode == "" {
			mode = "download"
		}
		if mode != "download" && mode != "upload" {
			SendErrorResponse(w, logger, &customError{message: "mode must be one of: download, upload"}, http.StatusBadRequest)
			return
		}

		releaseSlot := func() {}
		if "upload" == mode {
			releaseUploadSlot, acquired := wsAcquireUploadSlot(r.Context(), userID)
			if !acquired {
				SendErrorResponse(w, logger, &customError{message: "too many concurrent websocket uploads"}, http.StatusServiceUnavailable)
				return
			}
			var releaseOnce sync.Once
			releaseSlot = func() { releaseOnce.Do(releaseUploadSlot) }
			defer releaseSlot()
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("Failed to upgrade websocket", zap.Error(err))
			return
		}
		defer conn.Close()

		const maxWSMessageBytes = 1 << 20 // 1 MiB per message
		conn.SetReadLimit(maxWSMessageBytes)

		enginePath := pathInfo.FullPath

		switch mode {
		case "download":
			wsDownload(conn, r, engine, authorizer, userID, enginePath, logger)
		case "upload":
			wsUpload(conn, r, engine, authorizer, backendConfig, userID, pathInfo, enginePath, releaseSlot, logger)
		}
	}
}
