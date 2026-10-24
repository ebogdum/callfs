package links

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/internal/pathutil"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/server/handlers"
)

// DownloadLinkHandler creates an HTTP handler for downloading files via single-use links.
// @Summary Download file via single-use link
// @Description Downloads a file using a single-use token. The token becomes invalid after one use.
// @Tags links
// @Param token path string true "Single-use download token"
// @Produce application/octet-stream
// @Success 200 {string} binary "File content"
// @Failure 400 {object} handlers.ErrorResponse "Bad Request"
// @Failure 404 {object} handlers.ErrorResponse "Token not found"
// @Failure 410 {object} handlers.ErrorResponse "Token expired or already used"
// @Failure 500 {object} handlers.ErrorResponse "Internal Server Error"
// @Router /download/{token} [post]
func V1DownloadLinkHandler(engine *core.Engine, manager *links.LinkManager, logger *zap.Logger) http.HandlerFunc {
	return V1DownloadLinkHandlerWithTrustedProxies(engine, manager, nil, logger)
}

func V1DownloadLinkHandlerWithTrustedProxies(engine *core.Engine, manager *links.LinkManager, trustedProxies []netip.Prefix, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract token from URL path
		token := chi.URLParam(r, "token")
		if token == "" {
			handlers.SendErrorResponse(w, logger, errors.New("missing token"), http.StatusBadRequest)
			return
		}

		// Get user IP address
		userIP := getUserIP(r, trustedProxies)

		filePath, err := manager.InspectLink(ctx, token)
		if err != nil {
			logInvalidLinkAccess(logger, token, userIP, err)
			writeLinkError(w, logger, err)
			return
		}

		if err := pathutil.ValidatePath(filePath); err != nil {
			logger.Error("Stored link path failed validation",
				zap.String("file_path", filePath),
				zap.Error(err))
			handlers.SendErrorResponse(w, logger, errors.New("link validation failed"), http.StatusInternalServerError)
			return
		}

		reader, err := engine.GetFile(ctx, filePath)
		if err != nil {
			logger.Error("Failed to get file for single-use link",
				zap.String("token", links.TruncateToken(token)),
				zap.String("file_path", filePath),
				zap.String("user_ip", userIP),
				zap.Error(err))
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		if _, err := manager.ValidateAndInvalidateLink(ctx, token, userIP); err != nil {
			logInvalidLinkAccess(logger, token, userIP, err)
			writeLinkError(w, logger, err)
			return
		}

		// Set appropriate headers for file download (RFC 5987 encoding for safety)
		w.Header().Set("Content-Type", "application/octet-stream")
		filename := filepath.Base(filePath)
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))

		// Stream the file content
		_, err = io.Copy(w, reader)
		if err != nil {
			logger.Error("Failed to stream file content for single-use link",
				zap.String("token", links.TruncateToken(token)),
				zap.String("file_path", filePath),
				zap.String("user_ip", userIP),
				zap.Error(err))
			return
		}

		logger.Info("Successfully served file via single-use link",
			zap.String("token", links.TruncateToken(token)),
			zap.String("file_path", filePath),
			zap.String("user_ip", userIP))
	}
}

func logInvalidLinkAccess(logger *zap.Logger, token, userIP string, err error) {
	logger.Warn("Invalid single-use link access attempt",
		zap.String("token", links.TruncateToken(token)),
		zap.String("user_ip", userIP),
		zap.Error(err))
}

func writeLinkError(w http.ResponseWriter, logger *zap.Logger, err error) {
	switch {
	case errors.Is(err, links.ErrLinkNotFound):
		handlers.SendErrorResponse(w, logger, err, http.StatusNotFound)
	case errors.Is(err, links.ErrLinkExpired):
		handlers.SendErrorResponse(w, logger, err, http.StatusGone)
	case errors.Is(err, links.ErrLinkInvalid):
		handlers.SendErrorResponse(w, logger, err, http.StatusGone)
	default:
		handlers.SendErrorResponse(w, logger, errors.New("link validation failed"), http.StatusInternalServerError)
	}
}

func getUserIP(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if nil != err {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if nil != err || !isTrustedProxy(remote.Unmap(), trustedProxies) {
		return host
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; 0 <= i; i-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if nil != err {
			break
		}
		if !isTrustedProxy(candidate.Unmap(), trustedProxies) {
			return candidate.Unmap().String()
		}
	}
	return host
}

func isTrustedProxy(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

const downloadConfirmPage = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Confirm download</title></head>
<body>
<form method="post">
<p>This link can be used only once. Continue to download the file.</p>
<button type="submit">Download file</button>
</form>
</body>
</html>
`

func V1DownloadLinkConfirmHandler(manager *links.LinkManager, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if "" == token {
			handlers.SendErrorResponse(w, logger, errors.New("missing token"), http.StatusBadRequest)
			return
		}

		if _, err := manager.InspectLink(r.Context(), token); nil != err {
			logInvalidLinkAccess(logger, token, getUserIP(r, nil), err)
			writeLinkError(w, logger, err)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := io.WriteString(w, downloadConfirmPage); nil != err {
			logger.Error("Failed to write confirmation page for single-use link",
				zap.String("token", links.TruncateToken(token)),
				zap.Error(err))
		}
	}
}
