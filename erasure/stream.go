package erasure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

const blockSize = 256 << 10

var blockPool = sync.Pool{New: func() any {
	buf := make([]byte, blockSize)
	return &buf
}}

func getBlock() *[]byte { return blockPool.Get().(*[]byte) }

func putBlock(buf *[]byte) { blockPool.Put(buf) }

type shardCheck struct {
	index    int
	checksum string
	size     int64
	read     int64
	hash     hash.Hash
}

func newShardCheck(index int, checksum string, size int64) *shardCheck {
	return &shardCheck{index: index, checksum: checksum, size: size, hash: sha256.New()}
}

func (c *shardCheck) consume(p []byte) error {
	if _, err := c.hash.Write(p); err != nil {
		return err
	}
	c.read += int64(len(p))
	if c.read >= c.size && hex.EncodeToString(c.hash.Sum(nil)) != c.checksum {
		return fmt.Errorf("shard %d: %w", c.index, ErrShardCorrupted)
	}
	return nil
}

type shardReader struct {
	src    io.Reader
	closer io.Closer
	check  *shardCheck
}

func (r *shardReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if cErr := r.check.consume(p[:n]); cErr != nil {
		return n, cErr
	}
	if errors.Is(err, io.EOF) && r.check.read < r.check.size {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func (r *shardReader) Close() error {
	return r.closer.Close()
}

func (m *Manager) openShard(ctx context.Context, si metadata.ErasureShardInfo) (io.ReadCloser, error) {
	if si.InstanceID == m.instanceID {
		return m.localBackend.Open(ctx, si.Path)
	}
	return m.openRemoteShard(ctx, si.InstanceID, extractShardPrefix(si.Path), si.Index)
}

func (m *Manager) openFeed(ctx context.Context, si metadata.ErasureShardInfo, shardSize int64) *shardReader {
	rc, err := m.openShard(ctx, si)
	if err != nil {
		m.logger.Warn("Failed to open shard",
			zap.Int("index", si.Index),
			zap.String("instance", si.InstanceID),
			zap.Error(err))
		return nil
	}
	return &shardReader{
		src:    io.LimitReader(rc, shardSize),
		closer: rc,
		check:  newShardCheck(si.Index, si.Checksum, shardSize),
	}
}

func (m *Manager) openBatch(ctx context.Context, infos []*metadata.ErasureShardInfo, indices []int, shardSize int64) []*shardReader {
	feeds := make([]*shardReader, len(indices))
	var wg sync.WaitGroup
	for n, idx := range indices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			feeds[n] = m.openFeed(ctx, *infos[idx], shardSize)
		}()
	}
	wg.Wait()
	return feeds
}

func closeFeeds(feeds []*shardReader) {
	for _, f := range feeds {
		if f != nil {
			_ = f.Close()
		}
	}
}

func (m *Manager) openSources(ctx context.Context, mdInfo *metadata.ErasureFileInfo, infos []*metadata.ErasureShardInfo, exclude int) ([]*shardReader, error) {
	k := mdInfo.DataShards
	candidates := make([]int, 0, len(infos))
	for j, si := range infos {
		if j != exclude && si != nil {
			candidates = append(candidates, j)
		}
	}

	feeds := make([]*shardReader, 0, k)
	for start := 0; start < len(candidates) && len(feeds) < k; {
		batch := candidates[start:min(start+k-len(feeds), len(candidates))]
		for _, f := range m.openBatch(ctx, infos, batch, mdInfo.ShardSize) {
			if f != nil {
				feeds = append(feeds, f)
			}
		}
		start += len(batch)
	}
	if len(feeds) < k {
		closeFeeds(feeds)
		return nil, ErrInsufficientShards
	}
	return feeds, nil
}

func fillSources(feeds []*shardReader, bufs []*[]byte, n int) error {
	errs := make([]error, len(feeds))
	var wg sync.WaitGroup
	for j, f := range feeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := io.ReadFull(f, (*bufs[f.check.index])[:n]); err != nil {
				errs[j] = fmt.Errorf("shard %d: %w", f.check.index, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (m *Manager) reconstructShard(ctx context.Context, mdInfo *metadata.ErasureFileInfo, infos []*metadata.ErasureShardInfo, i int, emit func(fileOff int64, p []byte) error) error {
	if infos[i] == nil {
		return ErrInsufficientShards
	}
	k := mdInfo.DataShards
	total := k + mdInfo.ParityShards
	shardSize := mdInfo.ShardSize

	enc, err := m.codec.encoder(k, mdInfo.ParityShards)
	if err != nil {
		return fmt.Errorf("erasure: failed to create decoder: %w", err)
	}
	feeds, err := m.openSources(ctx, mdInfo, infos, i)
	if err != nil {
		return err
	}
	defer closeFeeds(feeds)

	bufs := make([]*[]byte, total)
	for j := range bufs {
		bufs[j] = getBlock()
		defer putBlock(bufs[j])
	}
	shards := make([][]byte, total)
	required := make([]bool, total)
	required[i] = true
	check := newShardCheck(i, infos[i].Checksum, shardSize)
	base := int64(i) * shardSize

	for pos := int64(0); pos < shardSize; {
		n := min(int64(blockSize), shardSize-pos)
		if err := fillSources(feeds, bufs, int(n)); err != nil {
			return err
		}
		clear(shards)
		for _, f := range feeds {
			shards[f.check.index] = (*bufs[f.check.index])[:n]
		}
		shards[i] = (*bufs[i])[:0]
		if err := enc.ReconstructSome(shards, required); err != nil {
			return fmt.Errorf("erasure: reconstruction failed: %w", err)
		}
		out := shards[i]
		if err := check.consume(out); err != nil {
			return err
		}
		if err := emit(base+pos, out); err != nil {
			return err
		}
		pos += n
	}
	return nil
}

func emitDirect(feed *shardReader, base, shardSize int64, emit func(fileOff int64, p []byte) error) error {
	block := getBlock()
	defer putBlock(block)
	for pos := int64(0); pos < shardSize; {
		n := min(int64(blockSize), shardSize-pos)
		buf := (*block)[:n]
		if _, err := io.ReadFull(feed, buf); err != nil {
			return fmt.Errorf("shard %d: %w", feed.check.index, err)
		}
		if err := emit(base+pos, buf); err != nil {
			return err
		}
		pos += n
	}
	return nil
}

func (m *Manager) streamShards(ctx context.Context, mdInfo *metadata.ErasureFileInfo, offset, n int64, w io.Writer) error {
	if n == 0 {
		return nil
	}
	shardSize := mdInfo.ShardSize
	if shardSize <= 0 {
		return fmt.Errorf("erasure: invalid shard size %d", shardSize)
	}
	total := mdInfo.DataShards + mdInfo.ParityShards
	infos := make([]*metadata.ErasureShardInfo, total)
	for idx := range mdInfo.Shards {
		si := &mdInfo.Shards[idx]
		if si.Index >= 0 && si.Index < total {
			infos[si.Index] = si
		}
	}

	end := offset + n
	first := int(offset / shardSize)
	last := int((end - 1) / shardSize)
	indices := make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		if infos[i] != nil {
			indices = append(indices, i)
		}
	}
	direct := make([]*shardReader, total)
	for pos, feed := range m.openBatch(ctx, infos, indices, shardSize) {
		direct[indices[pos]] = feed
	}
	defer closeFeeds(direct)

	hold := &holdBack{w: w}
	emit := func(fileOff int64, p []byte) error {
		lo := max(fileOff, offset)
		hi := min(fileOff+int64(len(p)), end)
		if lo >= hi {
			return nil
		}
		_, err := hold.Write(p[lo-fileOff : hi-fileOff])
		return err
	}

	for i := first; i <= last; i++ {
		if feed := direct[i]; feed != nil {
			if err := emitDirect(feed, int64(i)*shardSize, shardSize, emit); err != nil {
				return err
			}
			continue
		}
		if err := m.reconstructShard(ctx, mdInfo, infos, i, emit); err != nil {
			return err
		}
	}
	return hold.Flush()
}

type holdBack struct {
	w       io.Writer
	pending []byte
}

func (h *holdBack) Write(p []byte) (int, error) {
	if 0 < len(h.pending) {
		if _, err := h.w.Write(h.pending); nil != err {
			return 0, err
		}
	}
	h.pending = append(h.pending[:0], p...)
	return len(p), nil
}

func (h *holdBack) Flush() error {
	if 0 == len(h.pending) {
		return nil
	}
	_, err := h.w.Write(h.pending)
	return err
}

type signalWriter struct {
	w     io.Writer
	ready chan struct{}
	once  sync.Once
}

func (s *signalWriter) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.ready) })
	return s.w.Write(p)
}

type streamReader struct {
	reader *io.PipeReader
	cancel context.CancelFunc
}

func (s *streamReader) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *streamReader) Close() error {
	s.cancel()
	return s.reader.Close()
}

func (m *Manager) openStream(ctx context.Context, path string, mdInfo *metadata.ErasureFileInfo, offset, n int64) (io.ReadCloser, error) {
	select {
	case m.decodeSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	streamCtx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		err := m.streamShards(streamCtx, mdInfo, offset, n, &signalWriter{w: pw, ready: ready})
		<-m.decodeSlots
		m.finishStream(path, mdInfo, offset, n, err)
		pw.CloseWithError(err)
		done <- err
	}()

	select {
	case <-ready:
	case err := <-done:
		if err != nil {
			cancel()
			_ = pr.Close()
			return nil, err
		}
	case <-ctx.Done():
		cancel()
		_ = pr.Close()
		return nil, ctx.Err()
	}
	return &streamReader{reader: pr, cancel: cancel}, nil
}

func (m *Manager) finishStream(path string, mdInfo *metadata.ErasureFileInfo, offset, n int64, err error) {
	if nil != err {
		if errors.Is(err, ErrShardCorrupted) {
			m.runBackground(path, "repair", func(ctx context.Context) error {
				return m.RepairFile(ctx, path)
			})
		}
		return
	}
	if 0 < n && 0 == offset && n == mdInfo.OriginalSize {
		m.runBackground(path, "parity verification", func(ctx context.Context) error {
			return m.verifyParity(ctx, path)
		})
	}
}

func RangeLength(size, offset, length int64) (int64, error) {
	if offset < 0 || length < -1 {
		return 0, fmt.Errorf("invalid range offset %d length %d", offset, length)
	}
	if offset > size {
		return 0, fmt.Errorf("range offset %d beyond file size %d", offset, size)
	}
	n := size - offset
	if length >= 0 && length < n {
		n = length
	}
	return n, nil
}

func (m *Manager) OpenReader(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get erasure info: %w", err)
	}
	rc, err := m.openStream(ctx, path, mdInfo, 0, mdInfo.OriginalSize)
	if err != nil {
		return nil, 0, err
	}
	return rc, mdInfo.OriginalSize, nil
}

func (m *Manager) OpenRangeReader(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	rc, _, err := m.openRange(ctx, path, offset, length)
	return rc, err
}

func (m *Manager) openRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, int64, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get erasure info: %w", err)
	}
	n, err := RangeLength(mdInfo.OriginalSize, offset, length)
	if err != nil {
		return nil, 0, err
	}
	rc, err := m.openStream(ctx, path, mdInfo, offset, n)
	if err != nil {
		return nil, 0, err
	}
	return rc, n, nil
}

func (m *Manager) OpenShard(ctx context.Context, path string, index int) (io.ReadCloser, int64, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get erasure info: %w", err)
	}
	for _, si := range mdInfo.Shards {
		if si.Index == index {
			rc, err := m.openShard(ctx, si)
			if err != nil {
				return nil, 0, err
			}
			return &shardReader{
				src:    io.LimitReader(rc, si.Size),
				closer: rc,
				check:  newShardCheck(si.Index, si.Checksum, si.Size),
			}, si.Size, nil
		}
	}
	return nil, 0, ErrShardNotFound
}
