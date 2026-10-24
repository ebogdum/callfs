package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/telemetry"
)

func autoSplitManager(threshold config.ByteSize) *erasure.Manager {
	cfg := &config.ErasureConfig{DataShards: 4, ParityShards: 2, MinFileSize: threshold, ShardPath: ".callfs/shards"}
	return erasure.NewManager(nil, nil, cfg, "local", nil, "", zap.NewNop())
}

func TestErasureAutoSplitThreshold(t *testing.T) {
	em := autoSplitManager(100)
	cases := []struct {
		name   string
		length int64
		want   bool
	}{
		{"below", 99, false},
		{"equal", 100, true},
		{"above", 5000, true},
		{"unknown", -1, false},
		{"over spool limit", maxErasureUpload + 1, false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodPut, "/v1/files/a", strings.NewReader(""))
		r.ContentLength = tc.length
		if got := erasureAutoSplit(r, em); tc.want != got {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
	r := httptest.NewRequest(http.MethodPut, "/v1/files/a", nil)
	r.ContentLength = 500
	if erasureAutoSplit(r, nil) {
		t.Fatal("auto split without manager")
	}
	if erasureAutoSplit(r, autoSplitManager(0)) {
		t.Fatal("auto split with threshold 0")
	}
}

func TestErasureAutoSplitOptOut(t *testing.T) {
	em := autoSplitManager(100)
	for _, target := range []struct{ header, query string }{{"false", ""}, {"", "false"}} {
		r := httptest.NewRequest(http.MethodPut, "/v1/files/a?erasure="+target.query, nil)
		r.ContentLength = 500
		if "" != target.header {
			r.Header.Set("X-CallFS-Erasure", target.header)
		}
		if erasureAutoSplit(r, em) {
			t.Fatalf("opt-out %+v ignored", target)
		}
	}
	r := httptest.NewRequest(http.MethodPut, "/v1/files/a", nil)
	r.Header.Set("X-CallFS-Erasure", "true")
	if !erasureExplicit(r) {
		t.Fatal("explicit header not detected")
	}
}

func TestParseShardPathUsesRoot(t *testing.T) {
	got, index, err := parseShardPath("custom/pieces", "/v1/internal/shards/abc123/4")
	if nil != err || 4 != index || "custom/pieces/abc123/4" != got {
		t.Fatalf("got %q, %d, %v", got, index, err)
	}
}

type handlerErasureStore struct {
	infos map[string]*metadata.ErasureFileInfo
}

func (s *handlerErasureStore) CreateErasureInfo(_ context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	s.infos[filePath] = info
	return nil
}

func (s *handlerErasureStore) GetErasureInfo(_ context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	info, ok := s.infos[filePath]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return info, nil
}

func (s *handlerErasureStore) DeleteErasureInfo(_ context.Context, filePath string) error {
	delete(s.infos, filePath)
	return nil
}

type trackingRecorder struct {
	*httptest.ResponseRecorder
	path string
	seen bool
}

func (t *trackingRecorder) Write(buf []byte) (int, error) {
	for _, info := range telemetry.Transfers.Snapshot() {
		if t.path == info.Path && "download" == info.Kind {
			t.seen = true
		}
	}
	return t.ResponseRecorder.Write(buf)
}

func TestErasureDownloadIsTracked(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if nil != err {
		t.Fatal(err)
	}
	backend, err := localfs.NewLocalFSAdapter(root)
	if nil != err {
		t.Fatal(err)
	}
	store := &handlerErasureStore{infos: map[string]*metadata.ErasureFileInfo{}}
	cfg := &config.ErasureConfig{DataShards: 4, ParityShards: 2, ShardBackend: "localfs", ShardPath: ".callfs/shards"}
	em := erasure.NewManager(store, backend, cfg, "local", nil, "", zap.NewNop())

	data := bytes.Repeat([]byte("x"), int(telemetry.MinTrackedTransferBytes)+1)
	info, err := em.StoreFile(ctx, "/tracked.bin", data, int64(len(data)), nil)
	if nil != err {
		t.Fatal(err)
	}
	if err := em.CommitErasureInfo(ctx, info); nil != err {
		t.Fatal(err)
	}

	md := &metadata.Metadata{Size: int64(len(data)), ErasureCoded: true}
	recorder := &trackingRecorder{ResponseRecorder: httptest.NewRecorder(), path: "/tracked.bin"}
	HandleErasureDownload(recorder, httptest.NewRequest(http.MethodGet, "/v1/files/tracked.bin", nil), em, "/tracked.bin", md, zap.NewNop())
	if !recorder.seen {
		t.Fatal("download not visible in transfer tracker while streaming")
	}
	for _, item := range telemetry.Transfers.Snapshot() {
		if "/tracked.bin" == item.Path {
			t.Fatal("transfer left in tracker after download")
		}
	}
}
