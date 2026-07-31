package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const wsChunkSize = 64 * 1024

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  wsChunkSize,
	WriteBufferSize: wsChunkSize,
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

func wsCloseWithError(conn *websocket.Conn, code int, msg string) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, msg),
		time.Now().Add(5*time.Second))
}

func wsDownload(conn *websocket.Conn, r *http.Request, engine *core.Engine, authorizer auth.Authorizer, userID, enginePath string, logger *zap.Logger) {
	if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.ReadPerm); err != nil {
		wsCloseWithError(conn, websocket.ClosePolicyViolation, "read access denied")
		return
	}

	reader, err := engine.GetFile(r.Context(), enginePath)
	if err != nil {
		wsCloseWithError(conn, websocket.CloseInternalServerErr, "failed to open file")
		return
	}
	defer reader.Close()

	buf := make([]byte, wsChunkSize)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				logger.Warn("Failed writing websocket download chunk", zap.Error(err))
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
	buf  bytes.Buffer
	file *os.File
	size int64
}

// wsSpillThreshold is the point at which an in-progress upload moves to disk.
const wsSpillThreshold = 8 << 20 // 8 MiB

func (s *uploadSpool) Write(p []byte) error {
	if s.file == nil && s.size+int64(len(p)) > wsSpillThreshold {
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
		if _, err := f.Write(s.buf.Bytes()); err != nil {
			f.Close()
			return fmt.Errorf("failed to spill upload to disk: %w", err)
		}
		s.buf.Reset()
		s.file = f
	}

	var err error
	if s.file != nil {
		_, err = s.file.Write(p)
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
	}
}

func wsReadUploadPayload(conn *websocket.Conn, logger *zap.Logger) (*uploadSpool, bool) {
	spool := &uploadSpool{}
	const maxWSUpload = 100 << 20 // 100 MB max
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return spool, true
			}
			logger.Warn("Failed reading websocket upload message", zap.Error(readErr))
			spool.Close()
			return nil, false
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		if spool.size+int64(len(data)) > maxWSUpload {
			wsCloseWithError(conn, websocket.CloseMessageTooBig, "upload too large")
			spool.Close()
			return nil, false
		}
		if err := spool.Write(data); err != nil {
			logger.Warn("Failed buffering websocket upload payload", zap.Error(err))
			spool.Close()
			return nil, false
		}
	}
}

func wsUpload(conn *websocket.Conn, r *http.Request, engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, userID string, pathInfo PathInfo, enginePath string, logger *zap.Logger) {
	if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.WritePerm); err != nil {
		wsCloseWithError(conn, websocket.ClosePolicyViolation, "write access denied")
		return
	}

	payload, ok := wsReadUploadPayload(conn, logger)
	if !ok {
		return
	}
	defer payload.Close()

	size := payload.size
	existingMd, err := engine.GetMetadata(r.Context(), enginePath)
	if err != nil {
		if !errors.Is(err, metadata.ErrNotFound) {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "metadata lookup failed")
			return
		}
		reader, readerErr := payload.Reader()
		if readerErr != nil {
			logger.Error("Failed to read buffered websocket upload", zap.Error(readerErr))
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file create failed")
			return
		}
		createMd := &metadata.Metadata{
			Name:        pathInfo.Name,
			Type:        "file",
			Mode:        "0644",
			Owner:       userID,
			BackendType: backendConfig.DefaultBackend,
			ATime:       time.Now(),
			MTime:       time.Now(),
			CTime:       time.Now(),
		}
		if err := engine.CreateFile(r.Context(), enginePath, reader, size, createMd); err != nil {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file create failed")
			return
		}
	} else {
		if existingMd.Type != "file" {
			wsCloseWithError(conn, websocket.CloseUnsupportedData, "target path is not a file")
			return
		}
		reader, readerErr := payload.Reader()
		if readerErr != nil {
			logger.Error("Failed to read buffered websocket upload", zap.Error(readerErr))
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file update failed")
			return
		}
		if err := engine.UpdateFile(r.Context(), enginePath, reader, size, existingMd); err != nil {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file update failed")
			return
		}
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("ok:%d", size))); err != nil {
		logger.Warn("Failed writing websocket upload ack", zap.Error(err))
	}
	wsCloseWithError(conn, websocket.CloseNormalClosure, "upload complete")
}

// V1WebSocketTransfer handles websocket file transfers on /v1/files/ws/{path}.
// Query param mode=download|upload controls transfer direction.
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
			wsUpload(conn, r, engine, authorizer, backendConfig, userID, pathInfo, enginePath, logger)
		}
	}
}
