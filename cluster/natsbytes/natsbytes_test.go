package natsbytes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/natsrv/natstest"
)

const testInstance = "node-a"

type memStorage struct {
	mu       sync.Mutex
	files    map[string][]byte
	dirs     map[string]bool
	hookMu   sync.Mutex
	statHook func(ctx context.Context, p string) error
}

func (m *memStorage) setStatHook(hook func(ctx context.Context, p string) error) {
	m.hookMu.Lock()
	m.statHook = hook
	m.hookMu.Unlock()
}

func newMemStorage() *memStorage {
	return &memStorage{files: map[string][]byte{}, dirs: map[string]bool{}}
}

func clean(p string) string {
	return strings.Trim(path.Clean("/"+p), "/")
}

func (m *memStorage) put(p string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = clean(p)
	m.files[p] = data
	for dir := path.Dir(p); "." != dir && "/" != dir; dir = path.Dir(dir) {
		m.dirs[dir] = true
	}
}

func (m *memStorage) get(p string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[clean(p)]
	return data, ok
}

func (m *memStorage) has(p string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = clean(p)
	_, file := m.files[p]
	return file || m.dirs[p]
}

func (m *memStorage) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	data, ok := m.get(p)
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memStorage) OpenRange(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	data, ok := m.get(p)
	if !ok {
		return nil, metadata.ErrNotFound
	}
	if int64(len(data)) < offset {
		offset = int64(len(data))
	}
	end := int64(len(data))
	if 0 <= length && offset+length < end {
		end = offset + length
	}
	return io.NopCloser(bytes.NewReader(data[offset:end])), nil
}

func (m *memStorage) Create(ctx context.Context, p string, reader io.Reader, size int64) error {
	if m.has(p) {
		return metadata.ErrAlreadyExists
	}
	data, err := io.ReadAll(reader)
	if nil != err {
		return err
	}
	m.put(p, data)
	return nil
}

func (m *memStorage) Update(ctx context.Context, p string, reader io.Reader, size int64) error {
	if _, ok := m.get(p); !ok {
		return metadata.ErrNotFound
	}
	data, err := io.ReadAll(reader)
	if nil != err {
		return err
	}
	m.put(p, data)
	return nil
}

func (m *memStorage) Delete(ctx context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = clean(p)
	if _, ok := m.files[p]; ok {
		delete(m.files, p)
		return nil
	}
	if m.dirs[p] {
		for other := range m.files {
			if strings.HasPrefix(other, p+"/") {
				return fmt.Errorf("delete %s: %w", p, syscall.ENOTEMPTY)
			}
		}
		for other := range m.dirs {
			if strings.HasPrefix(other, p+"/") {
				return fmt.Errorf("delete %s: %w", p, syscall.ENOTEMPTY)
			}
		}
		delete(m.dirs, p)
		return nil
	}
	return metadata.ErrNotFound
}

func (m *memStorage) Move(ctx context.Context, oldPath, newPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, dst := clean(oldPath), clean(newPath)
	if data, ok := m.files[src]; ok {
		delete(m.files, src)
		m.files[dst] = data
		return nil
	}
	return metadata.ErrNotFound
}

func (m *memStorage) Stat(ctx context.Context, p string) (*metadata.Metadata, error) {
	m.hookMu.Lock()
	hook := m.statHook
	m.hookMu.Unlock()
	if nil != hook {
		if err := hook(ctx, p); nil != err {
			return nil, err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p = clean(p)
	now := time.Now()
	if data, ok := m.files[p]; ok {
		return &metadata.Metadata{Name: path.Base(p), Path: "/" + p, Type: "file", Size: int64(len(data)), Mode: "0644", MTime: now, ATime: now, CTime: now}, nil
	}
	if "" == p || m.dirs[p] {
		return &metadata.Metadata{Name: path.Base("/" + p), Path: "/" + p, Type: "directory", Mode: "0755", MTime: now, ATime: now, CTime: now}, nil
	}
	return nil, metadata.ErrNotFound
}

func (m *memStorage) ListDirectory(ctx context.Context, p string) ([]*metadata.Metadata, error) {
	m.mu.Lock()
	p = clean(p)
	prefix := p + "/"
	if "" == p {
		prefix = ""
	}
	names := map[string]*metadata.Metadata{}
	for file, data := range m.files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		rest := strings.TrimPrefix(file, prefix)
		if strings.Contains(rest, "/") {
			continue
		}
		names[rest] = &metadata.Metadata{Name: rest, Type: "file", Size: int64(len(data)), Mode: "0644"}
	}
	for dir := range m.dirs {
		if !strings.HasPrefix(dir, prefix) || dir == p {
			continue
		}
		rest := strings.TrimPrefix(dir, prefix)
		if strings.Contains(rest, "/") {
			continue
		}
		names[rest] = &metadata.Metadata{Name: rest, Type: "directory", Mode: "0755"}
	}
	m.mu.Unlock()
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	out := make([]*metadata.Metadata, 0, len(keys))
	for _, name := range keys {
		out = append(out, names[name])
	}
	return out, nil
}

func (m *memStorage) CreateDirectory(ctx context.Context, p string) error {
	if m.has(p) {
		return metadata.ErrAlreadyExists
	}
	m.mu.Lock()
	m.dirs[clean(p)] = true
	m.mu.Unlock()
	return nil
}

func (m *memStorage) Close() error {
	return nil
}

type rig struct {
	server  *Server
	client  *Client
	storage *memStorage
}

func newRig(t *testing.T, serverOpts ServerOptions, clientOpts ClientOptions) *rig {
	t.Helper()
	nodes := natstest.StartCluster(t, 3)
	storage := newMemStorage()
	server, err := NewServer(nodes[0].Conn, testInstance, storage, serverOpts, zap.NewNop())
	if nil != err {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := server.Close(); nil != closeErr {
			t.Errorf("close server: %v", closeErr)
		}
	})
	client, err := NewClient(nodes[2].Conn, "node-c", clientOpts)
	if nil != err {
		t.Fatalf("create client: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, statErr := client.Stat(context.Background(), testInstance, "/")
		if nil == statErr {
			break
		}
		if !errors.Is(statErr, cluster.ErrNoResponders) || !time.Now().Before(deadline) {
			t.Fatalf("server never became reachable: %v", statErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return &rig{server: server, client: client, storage: storage}
}

func payload(size int, seed int64) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(seed)).Read(data)
	return data
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestControlCalls(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second})
	ctx := context.Background()
	r.storage.put("docs/a.txt", []byte("hello"))

	md, err := r.client.Stat(ctx, testInstance, "docs/a.txt")
	if nil != err {
		t.Fatalf("stat: %v", err)
	}
	if "file" != md.Type || 5 != md.Size || "/docs/a.txt" != md.Path || "a.txt" != md.Name {
		t.Fatalf("unexpected stat result: %+v", md)
	}
	if _, err := r.client.Stat(ctx, testInstance, "docs/missing"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("stat missing: want ErrNotFound, got %v", err)
	}

	for i := 0; i < 2500; i++ {
		r.storage.put(fmt.Sprintf("big/f%05d", i), []byte("x"))
	}
	children, err := r.client.List(ctx, testInstance, "big")
	if nil != err {
		t.Fatalf("list: %v", err)
	}
	if 2500 != len(children) || "f00000" != children[0].Name || "f02499" != children[2499].Name || "/big/f00000" != children[0].Path {
		t.Fatalf("unexpected listing: %d entries, first %+v", len(children), children[0])
	}

	if err := r.client.Mkdir(ctx, testInstance, "fresh"); nil != err {
		t.Fatalf("mkdir: %v", err)
	}
	if err := r.client.Mkdir(ctx, testInstance, "fresh"); !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("mkdir twice: want ErrAlreadyExists, got %v", err)
	}

	if err := r.client.Move(ctx, testInstance, "docs/a.txt", []byte(`{"destination":"/docs/b.txt"}`)); nil != err {
		t.Fatalf("move: %v", err)
	}
	if r.storage.has("docs/a.txt") || !r.storage.has("docs/b.txt") {
		t.Fatalf("move did not relocate the file")
	}
	r.storage.put("docs/c.txt", []byte("c"))
	if err := r.client.Move(ctx, testInstance, "docs/b.txt", []byte(`{"destination":"/docs/c.txt"}`)); !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("move onto existing: want ErrAlreadyExists, got %v", err)
	}
	var badRequest *internalproxy.BadRequestError
	if err := r.client.Move(ctx, testInstance, "docs/b.txt", []byte(`{"destination":"/docs/b.txt"}`)); !errors.As(err, &badRequest) {
		t.Fatalf("move onto itself: want BadRequestError, got %v", err)
	}

	if err := r.client.Delete(ctx, testInstance, "docs", false); !errors.Is(err, internalproxy.ErrRemoteConflict) {
		t.Fatalf("delete non-empty: want ErrRemoteConflict, got %v", err)
	}
	if err := r.client.Delete(ctx, testInstance, "docs", true); nil != err {
		t.Fatalf("delete tree: %v", err)
	}
	if r.storage.has("docs") || r.storage.has("docs/b.txt") {
		t.Fatalf("delete tree left content behind")
	}
	if err := r.client.Delete(ctx, testInstance, "docs/b.txt", false); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

func TestControlHooksAndOnBehalf(t *testing.T) {
	var (
		mu       sync.Mutex
		seenMove MoveRequest
		seenUser string
		deleted  string
		tree     bool
	)
	hooks := Hooks{
		Move: func(ctx context.Context, req MoveRequest) error {
			mu.Lock()
			defer mu.Unlock()
			seenMove = req
			seenUser = OnBehalfOf(ctx)
			return nil
		},
		Delete: func(ctx context.Context, p string, recursive bool) error {
			mu.Lock()
			defer mu.Unlock()
			deleted = p
			tree = recursive
			return nil
		},
	}
	r := newRig(t, ServerOptions{Hooks: hooks}, ClientOptions{})
	ctx := WithOnBehalfOf(context.Background(), "alice")
	if err := r.client.Move(ctx, testInstance, "a/b", []byte(`{"destination":"c/d","overwrite":true,"create_parents":true}`)); nil != err {
		t.Fatalf("move: %v", err)
	}
	if err := r.client.Delete(ctx, testInstance, "a/b", true); nil != err {
		t.Fatalf("delete: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if "/a/b" != seenMove.Source || "/c/d" != seenMove.Destination || !seenMove.Overwrite || !seenMove.CreateParents || "alice" != seenUser {
		t.Fatalf("hook saw %+v as %q", seenMove, seenUser)
	}
	if "/a/b" != deleted || !tree {
		t.Fatalf("delete hook saw %q recursive=%v", deleted, tree)
	}
	if _, err := r.client.Stat(WithOnBehalfOf(context.Background(), "internal-proxy"), testInstance, "/"); !errors.Is(err, cluster.ErrInvalidOnBehalf) {
		t.Fatalf("reserved on-behalf user: want ErrInvalidOnBehalf, got %v", err)
	}
}

func TestChunkedTransfer(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second, Window: 2})
	ctx := context.Background()
	data := payload(10*DefaultChunkSize+12345, 1)

	etag, err := r.client.Create(ctx, testInstance, "blob/big.bin", bytes.NewReader(data), int64(len(data)), "", false)
	if nil != err {
		t.Fatalf("create: %v", err)
	}
	if digest(data) != etag {
		t.Fatalf("create etag %s, want %s", etag, digest(data))
	}
	stored, ok := r.storage.get("blob/big.bin")
	if !ok || !bytes.Equal(stored, data) {
		t.Fatalf("stored content differs from the upload")
	}

	reader, err := r.client.Open(ctx, testInstance, "blob/big.bin")
	if nil != err {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(reader)
	if nil != err {
		t.Fatalf("read: %v", err)
	}
	if err := reader.Close(); nil != err {
		t.Fatalf("close: %v", err)
	}
	if digest(data) != digest(got) {
		t.Fatalf("downloaded content differs: %d bytes", len(got))
	}
	if n, err := reader.Read(make([]byte, 1)); 0 != n || !errors.Is(err, io.EOF) {
		t.Fatalf("read after EOF: n=%d err=%v", n, err)
	}

	ranged, err := r.client.OpenRange(ctx, testInstance, "blob/big.bin", 300000, 900000)
	if nil != err {
		t.Fatalf("open range: %v", err)
	}
	gotRange, err := io.ReadAll(ranged)
	_ = ranged.Close()
	if nil != err || !bytes.Equal(gotRange, data[300000:1200000]) {
		t.Fatalf("range read: err=%v len=%d", err, len(gotRange))
	}
	tail, err := r.client.OpenRange(ctx, testInstance, "blob/big.bin", int64(len(data)-1000), -1)
	if nil != err {
		t.Fatalf("open tail: %v", err)
	}
	gotTail, err := io.ReadAll(tail)
	_ = tail.Close()
	if nil != err || !bytes.Equal(gotTail, data[len(data)-1000:]) {
		t.Fatalf("tail read: err=%v len=%d", err, len(gotTail))
	}

	updated := payload(3*DefaultChunkSize+1, 2)
	if _, err := r.client.Update(ctx, testInstance, "blob/big.bin", bytes.NewReader(updated), int64(len(updated)), "not-the-etag", false); !errors.Is(err, metadata.ErrPrecondition) {
		t.Fatalf("update with wrong etag: want ErrPrecondition, got %v", err)
	}
	newETag, err := r.client.Update(ctx, testInstance, "blob/big.bin", bytes.NewReader(updated), int64(len(updated)), etag, false)
	if nil != err || digest(updated) != newETag {
		t.Fatalf("update: etag=%s err=%v", newETag, err)
	}
	if stored, _ := r.storage.get("blob/big.bin"); !bytes.Equal(stored, updated) {
		t.Fatalf("update did not replace the content")
	}
	if _, err := r.client.Create(ctx, testInstance, "blob/big.bin", strings.NewReader("x"), 1, "", true); !errors.Is(err, metadata.ErrPrecondition) {
		t.Fatalf("create with if-none-match on existing: want ErrPrecondition, got %v", err)
	}
	if _, err := r.client.Create(ctx, testInstance, "blob/big.bin", strings.NewReader("x"), 1, "", false); !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("create on existing: want ErrAlreadyExists, got %v", err)
	}
	if _, err := r.client.Update(ctx, testInstance, "blob/missing.bin", strings.NewReader("x"), 1, "", false); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	small := []byte("unknown length")
	smallETag, err := r.client.Create(ctx, testInstance, "blob/small.bin", bytes.NewReader(small), 0, "", false)
	if nil != err || digest(small) != smallETag {
		t.Fatalf("create with unknown size: etag=%s err=%v", smallETag, err)
	}
	emptyETag, err := r.client.Create(ctx, testInstance, "blob/empty.bin", bytes.NewReader(nil), 0, "", false)
	if nil != err || digest(nil) != emptyETag {
		t.Fatalf("create empty: etag=%s err=%v", emptyETag, err)
	}
	if _, err := r.client.Open(ctx, testInstance, "blob/none.bin"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("open missing: want ErrNotFound, got %v", err)
	}
}

func TestSizeMismatchIsRejected(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second})
	data := payload(DefaultChunkSize+10, 3)
	_, err := r.client.Create(context.Background(), testInstance, "short.bin", bytes.NewReader(data), int64(len(data))+5, "", false)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short reader: want ErrUnexpectedEOF, got %v", err)
	}
	if r.storage.has("short.bin") {
		t.Fatalf("aborted upload left a file behind")
	}
}

func TestDownloadCancellation(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second, Window: 2})
	r.storage.put("huge.bin", payload(16*DefaultChunkSize, 4))

	ctx, cancel := context.WithCancel(context.Background())
	reader, err := r.client.Open(ctx, testInstance, "huge.bin")
	if nil != err {
		cancel()
		t.Fatalf("open: %v", err)
	}
	if _, err := io.ReadFull(reader, make([]byte, 100)); nil != err {
		cancel()
		t.Fatalf("first read: %v", err)
	}
	cancel()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err = reader.Read(make([]byte, 64<<10))
		if nil != err {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("reads kept succeeding after cancel")
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("read after cancel: want context.Canceled, got %v", err)
	}

	closed, err := r.client.Open(context.Background(), testInstance, "huge.bin")
	if nil != err {
		t.Fatalf("open: %v", err)
	}
	if _, err := io.ReadFull(closed, make([]byte, 100)); nil != err {
		t.Fatalf("first read: %v", err)
	}
	if err := closed.Close(); nil != err {
		t.Fatalf("close: %v", err)
	}
	if err := closed.Close(); nil != err {
		t.Fatalf("second close: %v", err)
	}
	if _, err := closed.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("read after close: want ErrClosedPipe, got %v", err)
	}
}

type abortingReader struct {
	cancel context.CancelFunc
	ctx    context.Context
	left   int
}

func (a *abortingReader) Read(p []byte) (int, error) {
	if 0 >= a.left {
		a.cancel()
		return 0, a.ctx.Err()
	}
	n := min(len(p), a.left)
	for i := 0; i < n; i++ {
		p[i] = byte(i)
	}
	a.left -= n
	return n, nil
}

func TestUploadCancellation(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second, Window: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &abortingReader{cancel: cancel, ctx: ctx, left: 3 * DefaultChunkSize}
	_, err := r.client.Create(ctx, testInstance, "aborted.bin", source, 0, "", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled upload: want context.Canceled, got %v", err)
	}
	if r.storage.has("aborted.bin") {
		t.Fatalf("canceled upload left a file behind")
	}
}

func TestTimeouts(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 300 * time.Millisecond})
	r.storage.put("slow.txt", []byte("x"))
	r.storage.setStatHook(func(ctx context.Context, p string) error {
		if "slow.txt" == clean(p) {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	start := time.Now()
	_, err := r.client.Stat(context.Background(), testInstance, "slow.txt")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow stat: want deadline exceeded, got %v", err)
	}
	if 5*time.Second < time.Since(start) {
		t.Fatalf("stat took %s, the rpc timeout is 300ms", time.Since(start))
	}
	if _, err := r.client.Open(context.Background(), testInstance, "slow.txt"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow open: want deadline exceeded, got %v", err)
	}
	if _, err := r.client.Stat(context.Background(), "ghost", "/"); !errors.Is(err, cluster.ErrNoResponders) {
		t.Fatalf("stat on unknown node: want ErrNoResponders, got %v", err)
	}
	if _, err := r.client.Open(context.Background(), "ghost", "x"); !errors.Is(err, cluster.ErrNoResponders) {
		t.Fatalf("open on unknown node: want ErrNoResponders, got %v", err)
	}
}

func TestAdapterUsesTransports(t *testing.T) {
	r := newRig(t, ServerOptions{}, ClientOptions{RPCTimeout: 5 * time.Second})
	adapter, err := internalproxy.NewInternalProxyAdapter(
		map[string]string{testInstance: "https://127.0.0.1:1"},
		"secret",
		false,
		zap.NewNop(),
		internalproxy.WithControl(r.client),
		internalproxy.WithByteTransport(r.client),
	)
	if nil != err {
		t.Fatalf("create adapter: %v", err)
	}
	ctx := internalproxy.WithInstanceID(context.Background(), testInstance)
	data := payload(2*DefaultChunkSize+9, 5)
	if err := adapter.Create(ctx, "via/adapter.bin", bytes.NewReader(data), int64(len(data))); nil != err {
		t.Fatalf("adapter create: %v", err)
	}
	md, err := adapter.Stat(ctx, "via/adapter.bin")
	if nil != err || int64(len(data)) != md.Size {
		t.Fatalf("adapter stat: %+v %v", md, err)
	}
	reader, err := adapter.Open(ctx, "via/adapter.bin")
	if nil != err {
		t.Fatalf("adapter open: %v", err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if nil != err || !bytes.Equal(got, data) {
		t.Fatalf("adapter read: err=%v len=%d", err, len(got))
	}
	listing, err := adapter.ListDirectory(ctx, "via")
	if nil != err || 1 != len(listing) {
		t.Fatalf("adapter list: %v %v", listing, err)
	}
	if err := adapter.DeleteOnInstance(ctx, testInstance, "via/adapter.bin"); nil != err {
		t.Fatalf("adapter delete: %v", err)
	}
	if r.storage.has("via/adapter.bin") {
		t.Fatalf("adapter delete left the file behind")
	}
}
