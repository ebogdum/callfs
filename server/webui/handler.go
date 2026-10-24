package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ebogdum/callfs/server/middleware"
	"go.uber.org/zap"
)

const (
	uiPrefix              = "/ui/"
	uiRoot                = "/ui"
	rootPath              = "/"
	buildDirectory        = "build"
	indexDocument         = "index.html"
	immutableDirectory    = "_app/immutable/"
	immutableCacheControl = "public, max-age=31536000, immutable"
	revalidateCache       = "no-cache"
	unavailableDocument   = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Console not available</title>
</head>
<body>
<h1>Console not available</h1>
<p>The console is not part of this build. Build with <code>make build-ui</code>.</p>
</body>
</html>
`
)

var (
	inlineScriptPattern  = regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)
	srcAttributePattern  = regexp.MustCompile(`(?i)(^|\s)src\s*=`)
	contentTypeOverrides = map[string]string{
		".woff":  "font/woff",
		".woff2": "font/woff2",
	}
	encodedVariants = []encodedVariant{{coding: "br", suffix: ".br"}, {coding: "gzip", suffix: ".gz"}}
)

type encodedVariant struct {
	coding string
	suffix string
}

type handler struct {
	next     http.Handler
	secured  http.Handler
	assets   fs.FS
	csp      string
	embedded bool
	logger   *zap.Logger
}

func Mount(next http.Handler, assets fs.FS, embedded bool, enabled bool, logger *zap.Logger) (http.Handler, error) {
	if !enabled {
		return next, nil
	}
	if nil == logger {
		logger = zap.NewNop()
	}
	h := &handler{next: next, embedded: embedded, logger: logger}
	var scriptHashes []string
	if embedded {
		if nil == assets {
			return nil, errors.New("console assets are missing")
		}
		build, err := fs.Sub(assets, buildDirectory)
		if nil != err {
			return nil, fmt.Errorf("open console build directory: %w", err)
		}
		index, err := fs.ReadFile(build, indexDocument)
		if nil != err {
			return nil, fmt.Errorf("read console index document: %w", err)
		}
		h.assets = build
		scriptHashes = inlineScriptHashes(index)
	}
	h.csp = contentSecurityPolicy(scriptHashes)
	h.secured = middleware.V1SecurityHeaders()(http.HandlerFunc(h.serveConsole))
	logger.Info("console handler mounted", zap.Bool("embedded", embedded), zap.Int("inline_script_hashes", len(scriptHashes)))
	return h, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case rootPath == r.URL.Path && isReadMethod(r.Method):
		http.Redirect(w, r, uiPrefix, http.StatusFound)
	case uiRoot == r.URL.Path:
		http.Redirect(w, r, consoleRedirectTarget(r), http.StatusPermanentRedirect)
	case strings.HasPrefix(r.URL.Path, uiPrefix):
		h.secured.ServeHTTP(w, r)
	default:
		h.next.ServeHTTP(w, r)
	}
}

func (h *handler) serveConsole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", h.csp)
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if !isReadMethod(r.Method) {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !h.embedded {
		h.serveUnavailable(w, r)
		return
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, uiPrefix), "/")
	if "" == rel {
		rel = indexDocument
	}
	if !fs.ValidPath(rel) {
		http.NotFound(w, r)
		return
	}
	info, statErr := fs.Stat(h.assets, rel)
	if nil != statErr && !errors.Is(statErr, fs.ErrNotExist) {
		h.fail(w, rel, statErr)
		return
	}
	isFile := nil == statErr && !info.IsDir()
	switch {
	case isFile && strings.HasPrefix(rel, immutableDirectory):
		h.serveFile(w, r, rel, immutableCacheControl)
	case isFile:
		h.serveFile(w, r, rel, revalidateCache)
	case strings.HasPrefix(rel, immutableDirectory):
		http.NotFound(w, r)
	default:
		h.serveFile(w, r, indexDocument, revalidateCache)
	}
}

func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string, cacheControl string) {
	content, coding, err := h.readVariant(name, r.Header.Get("Accept-Encoding"))
	if nil != err {
		h.fail(w, name, err)
		return
	}
	w.Header().Set("Content-Type", contentType(name))
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Vary", "Accept-Encoding")
	if "" != coding {
		w.Header().Set("Content-Encoding", coding)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}

func (h *handler) readVariant(name string, acceptEncoding string) ([]byte, string, error) {
	for _, variant := range encodedVariants {
		if !acceptsEncoding(acceptEncoding, variant.coding) {
			continue
		}
		content, err := fs.ReadFile(h.assets, name+variant.suffix)
		if nil == err {
			return content, variant.coding, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, "", err
		}
	}
	content, err := fs.ReadFile(h.assets, name)
	return content, "", err
}

func (h *handler) serveUnavailable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	if http.MethodGet != r.Method {
		return
	}
	if _, err := io.WriteString(w, unavailableDocument); nil != err {
		h.logger.Warn("write console unavailable page", zap.Error(err))
	}
}

func (h *handler) fail(w http.ResponseWriter, asset string, err error) {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	h.logger.Error("read console asset", zap.String("asset", asset), zap.Error(err))
}

func consoleRedirectTarget(r *http.Request) string {
	if "" == r.URL.RawQuery {
		return uiPrefix
	}
	return uiPrefix + "?" + r.URL.RawQuery
}

func isReadMethod(method string) bool {
	return http.MethodGet == method || http.MethodHead == method
}

func acceptsEncoding(header string, coding string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(name), coding) {
			continue
		}
		quality, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(params), "q=")), 64)
		return nil != err || quality > 0
	}
	return false
}

func contentType(name string) string {
	ext := path.Ext(name)
	if value, ok := contentTypeOverrides[ext]; ok {
		return value
	}
	if value := mime.TypeByExtension(ext); "" != value {
		return value
	}
	return "application/octet-stream"
}

func inlineScriptHashes(document []byte) []string {
	seen := make(map[string]bool)
	hashes := make([]string, 0)
	for _, match := range inlineScriptPattern.FindAllSubmatch(document, -1) {
		if srcAttributePattern.Match(match[1]) {
			continue
		}
		sum := sha256.Sum256(match[2])
		source := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !seen[source] {
			seen[source] = true
			hashes = append(hashes, source)
		}
	}
	return hashes
}

func contentSecurityPolicy(scriptHashes []string) string {
	scriptSources := append([]string{"'self'"}, scriptHashes...)
	return "default-src 'self'; script-src " + strings.Join(scriptSources, " ") + "; style-src 'self'; style-src-attr 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
}
