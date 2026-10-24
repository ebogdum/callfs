package telemetry

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MinTrackedTransferBytes int64 = 1 << 20

	transferUnknownTotal = -1
	transferCopyChunk    = 8 << 20
)

var Transfers = NewTransferTracker()

type TransferInfo struct {
	ID         string
	UserID     string
	Kind       string
	Path       string
	BytesDone  int64
	BytesTotal *int64
	StartedAt  time.Time
	Client     string
}

type TransferTracker struct {
	mu    sync.RWMutex
	items map[string]*TransferHandle
	seq   atomic.Uint64
	now   func() time.Time
}

func NewTransferTracker() *TransferTracker {
	return &TransferTracker{items: make(map[string]*TransferHandle), now: time.Now}
}

type TransferHandle struct {
	tracker   *TransferTracker
	id        string
	userID    string
	kind      string
	path      string
	client    string
	startedAt time.Time
	done      atomic.Int64
	total     atomic.Int64
	finished  atomic.Bool
}

func (t *TransferTracker) Begin(kind, userID, path, client string, total int64) *TransferHandle {
	started := t.now()
	handle := &TransferHandle{
		tracker:   t,
		id:        fmt.Sprintf("%x-%x", started.UnixNano(), t.seq.Add(1)),
		userID:    userID,
		kind:      kind,
		path:      path,
		client:    client,
		startedAt: started,
	}
	if 0 > total {
		total = transferUnknownTotal
	}
	handle.total.Store(total)
	t.mu.Lock()
	t.items[handle.id] = handle
	t.mu.Unlock()
	return handle
}

func (t *TransferTracker) BeginIfLarge(kind, userID, path, client string, total int64) *TransferHandle {
	if 0 <= total && total < MinTrackedTransferBytes {
		return nil
	}
	return t.Begin(kind, userID, path, client, total)
}

func (t *TransferTracker) Snapshot() []TransferInfo {
	t.mu.RLock()
	items := make([]TransferInfo, 0, len(t.items))
	for _, handle := range t.items {
		info := TransferInfo{
			ID:        handle.id,
			UserID:    handle.userID,
			Kind:      handle.kind,
			Path:      handle.path,
			BytesDone: handle.done.Load(),
			StartedAt: handle.startedAt,
			Client:    handle.client,
		}
		if total := handle.total.Load(); 0 <= total {
			info.BytesTotal = &total
		}
		items = append(items, info)
	}
	t.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		if !items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].StartedAt.Before(items[j].StartedAt)
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func (h *TransferHandle) Add(n int64) {
	if nil == h {
		return
	}
	h.done.Add(n)
}

func (h *TransferHandle) SetTotal(total int64) {
	if nil == h {
		return
	}
	h.total.Store(total)
}

func (h *TransferHandle) Finish() {
	if nil == h || !h.finished.CompareAndSwap(false, true) {
		return
	}
	h.tracker.mu.Lock()
	delete(h.tracker.items, h.id)
	h.tracker.mu.Unlock()
}

type progressReadCloser struct {
	io.ReadCloser
	handle *TransferHandle
}

func (p *progressReadCloser) Read(buf []byte) (int, error) {
	n, err := p.ReadCloser.Read(buf)
	p.handle.Add(int64(n))
	return n, err
}

func (h *TransferHandle) Body(body io.ReadCloser) io.ReadCloser {
	if nil == h {
		return body
	}
	return &progressReadCloser{ReadCloser: body, handle: h}
}

type progressWriter struct {
	dst    io.Writer
	handle *TransferHandle
}

func (p *progressWriter) Write(buf []byte) (int, error) {
	n, err := p.dst.Write(buf)
	p.handle.Add(int64(n))
	return n, err
}

func (p *progressWriter) ReadFrom(src io.Reader) (int64, error) {
	readerFrom, ok := p.dst.(io.ReaderFrom)
	if !ok {
		return io.Copy(struct{ io.Writer }{p}, src)
	}
	var total int64
	for {
		n, err := readerFrom.ReadFrom(io.LimitReader(src, transferCopyChunk))
		p.handle.Add(n)
		total += n
		if nil != err || n < transferCopyChunk {
			return total, err
		}
	}
}

func (h *TransferHandle) Writer(dst io.Writer) io.Writer {
	if nil == h {
		return dst
	}
	return &progressWriter{dst: dst, handle: h}
}
