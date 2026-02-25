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
		return true
	},
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

		enginePath := pathInfo.FullPath

		switch mode {
		case "download":
			if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.ReadPerm); err != nil {
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "read access denied"),
					time.Now().Add(5*time.Second))
				return
			}

			reader, err := engine.GetFile(r.Context(), enginePath)
			if err != nil {
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to open file"),
					time.Now().Add(5*time.Second))
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

				if readErr != nil {
					if errors.Is(readErr, io.EOF) {
						_ = conn.WriteControl(websocket.CloseMessage,
							websocket.FormatCloseMessage(websocket.CloseNormalClosure, "download complete"),
							time.Now().Add(5*time.Second))
						return
					}

					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "file read failed"),
						time.Now().Add(5*time.Second))
					return
				}
			}
		case "upload":
			if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.WritePerm); err != nil {
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "write access denied"),
					time.Now().Add(5*time.Second))
				return
			}

			var payload bytes.Buffer
			for {
				messageType, data, readErr := conn.ReadMessage()
				if readErr != nil {
					if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						break
					}

					logger.Warn("Failed reading websocket upload message", zap.Error(readErr))
					return
				}

				if messageType != websocket.BinaryMessage {
					continue
				}

				if _, err := payload.Write(data); err != nil {
					logger.Warn("Failed buffering websocket upload payload", zap.Error(err))
					return
				}
			}

			size := int64(payload.Len())
			existingMd, err := engine.GetMetadata(r.Context(), enginePath)
			if err != nil {
				if !errors.Is(err, metadata.ErrNotFound) {
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "metadata lookup failed"),
						time.Now().Add(5*time.Second))
					return
				}

				createMd := &metadata.Metadata{
					Name:        pathInfo.Name,
					Type:        "file",
					Mode:        "0644",
					UID:         1000,
					GID:         1000,
					BackendType: backendConfig.DefaultBackend,
					ATime:       time.Now(),
					MTime:       time.Now(),
					CTime:       time.Now(),
				}
				if err := engine.CreateFile(r.Context(), enginePath, bytes.NewReader(payload.Bytes()), size, createMd); err != nil {
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "file create failed"),
						time.Now().Add(5*time.Second))
					return
				}
			} else {
				if existingMd.Type != "file" {
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "target path is not a file"),
						time.Now().Add(5*time.Second))
					return
				}
				if err := engine.UpdateFile(r.Context(), enginePath, bytes.NewReader(payload.Bytes()), size, existingMd); err != nil {
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "file update failed"),
						time.Now().Add(5*time.Second))
					return
				}
			}

			if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("ok:%d", size))); err != nil {
				logger.Warn("Failed writing websocket upload ack", zap.Error(err))
			}
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "upload complete"),
				time.Now().Add(5*time.Second))
		}
	}
}
