package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func wsReadUploadPayload(conn *websocket.Conn, logger *zap.Logger) (*bytes.Buffer, bool) {
	var payload bytes.Buffer
	const maxWSUpload = 100 << 20 // 100 MB max
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return &payload, true
			}
			logger.Warn("Failed reading websocket upload message", zap.Error(readErr))
			return nil, false
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		if int64(payload.Len())+int64(len(data)) > maxWSUpload {
			wsCloseWithError(conn, websocket.CloseMessageTooBig, "upload too large")
			return nil, false
		}
		if _, err := payload.Write(data); err != nil {
			logger.Warn("Failed buffering websocket upload payload", zap.Error(err))
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

	size := int64(payload.Len())
	existingMd, err := engine.GetMetadata(r.Context(), enginePath)
	if err != nil {
		if !errors.Is(err, metadata.ErrNotFound) {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "metadata lookup failed")
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
		if err := engine.CreateFile(r.Context(), enginePath, bytes.NewReader(payload.Bytes()), size, createMd); err != nil {
			wsCloseWithError(conn, websocket.CloseInternalServerErr, "file create failed")
			return
		}
	} else {
		if existingMd.Type != "file" {
			wsCloseWithError(conn, websocket.CloseUnsupportedData, "target path is not a file")
			return
		}
		if err := engine.UpdateFile(r.Context(), enginePath, bytes.NewReader(payload.Bytes()), size, existingMd); err != nil {
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
