package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"go.uber.org/zap"
)

const (
	testIndexScript = "window.consoleReady = true;"
	testEntryScript = "console.log('entry');"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"build/index.html": {Data: []byte("<!doctype html><html><head><script>" + testIndexScript +
			"</script><script src=\"/ui/_app/immutable/entry/app.js\"></script></head><body></body></html>")},
		"build/_app/immutable/entry/app.js":    {Data: []byte(testEntryScript)},
		"build/_app/immutable/entry/app.js.br": {Data: []byte("brotli-entry")},
		"build/_app/immutable/entry/app.js.gz": {Data: []byte("gzip-entry")},
	}
}

func scriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func serveTest(t *testing.T, assets fs.FS, embedded bool, enabled bool, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler, err := Mount(next, assets, embedded, enabled, zap.NewNop())
	if nil != err {
		t.Fatalf("Mount: %v", err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMountRedirects(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		status   int
		location string
	}{
		{name: "root", target: "/", status: http.StatusFound, location: "/ui/"},
		{name: "bare ui path", target: "/ui?tab=files", status: http.StatusPermanentRedirect, location: "/ui/?tab=files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveTest(t, testAssets(), true, true, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if tt.status != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if tt.location != rec.Header().Get("Location") {
				t.Fatalf("location = %q, want %q", rec.Header().Get("Location"), tt.location)
			}
		})
	}
}

func TestUnhandledPathsReachNext(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		target  string
	}{
		{name: "disabled console", enabled: false, target: "/ui/"},
		{name: "disabled root", enabled: false, target: "/"},
		{name: "api path", enabled: true, target: "/v1/health"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveTest(t, testAssets(), true, tt.enabled, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if http.StatusTeapot != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
			}
		})
	}
}

func TestIndexServesSPAFallbackWithInlineScriptHash(t *testing.T) {
	rec := serveTest(t, testAssets(), true, true, httptest.NewRequest(http.MethodGet, "/ui/files/archive", nil))
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<script>"+testIndexScript+"</script>") {
		t.Fatalf("body does not contain index document: %s", rec.Body.String())
	}
	if "no-cache" != rec.Header().Get("Cache-Control") {
		t.Fatalf("Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' "+scriptHash(testIndexScript)+";") {
		t.Fatalf("CSP script-src lacks inline hash: %q", csp)
	}
}

func TestImmutableAssetsNegotiatePrecompressedVariants(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		encoding string
		body     string
	}{
		{name: "brotli preferred", accept: "br, gzip", encoding: "br", body: "brotli-entry"},
		{name: "gzip when brotli refused", accept: "br;q=0, gzip", encoding: "gzip", body: "gzip-entry"},
		{name: "identity without accept", accept: "", encoding: "", body: testEntryScript},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ui/_app/immutable/entry/app.js", nil)
			req.Header.Set("Accept-Encoding", tt.accept)
			rec := serveTest(t, testAssets(), true, true, req)
			if http.StatusOK != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if tt.encoding != rec.Header().Get("Content-Encoding") {
				t.Fatalf("Content-Encoding = %q, want %q", rec.Header().Get("Content-Encoding"), tt.encoding)
			}
			if tt.body != rec.Body.String() {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tt.body)
			}
			if "public, max-age=31536000, immutable" != rec.Header().Get("Cache-Control") {
				t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
			}
			if "Accept-Encoding" != rec.Header().Get("Vary") {
				t.Fatalf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
			}
		})
	}
}

func TestMissingImmutableAssetIsNotFound(t *testing.T) {
	rec := serveTest(t, testAssets(), true, true, httptest.NewRequest(http.MethodGet, "/ui/_app/immutable/missing.js", nil))
	if http.StatusNotFound != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRejectsWritesAndTraversal(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		status int
	}{
		{name: "post", method: http.MethodPost, target: "/ui/", status: http.StatusMethodNotAllowed},
		{name: "traversal", method: http.MethodGet, target: "/ui/../secret", status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveTest(t, testAssets(), true, true, httptest.NewRequest(tt.method, tt.target, nil))
			if tt.status != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
}

func TestNotEmbeddedServesUnavailablePage(t *testing.T) {
	rec := serveTest(t, fstest.MapFS{}, false, true, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if http.StatusServiceUnavailable != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "The console is not part of this build.") {
		t.Fatalf("body lacks unavailable notice: %s", body)
	}
	if strings.Contains(body, "<script") || strings.Contains(body, "<style") {
		t.Fatalf("unavailable page must not carry inline script or style: %s", body)
	}
}

func TestEmbeddedWithoutIndexFails(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	_, err := Mount(next, fstest.MapFS{"build/app.js": {Data: []byte("x")}}, true, true, zap.NewNop())
	if nil == err {
		t.Fatal("Mount without index.html returned no error")
	}
}

func TestConsoleResponsesCarrySecurityHeaders(t *testing.T) {
	rec := serveTest(t, testAssets(), true, true, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	expected := map[string]string{
		"X-Frame-Options":              "DENY",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Embedder-Policy": "require-corp",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	for header, want := range expected {
		if want != rec.Header().Get(header) {
			t.Fatalf("%s = %q, want %q", header, rec.Header().Get(header), want)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("CSP lacks frame-ancestors: %q", rec.Header().Get("Content-Security-Policy"))
	}
}
