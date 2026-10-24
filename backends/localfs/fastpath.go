package localfs

import (
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"
)

const (
	copyBufferSize = 256 << 10
	freeSpaceTTL   = time.Second
)

var copyBuffers = sync.Pool{New: func() any {
	buf := make([]byte, copyBufferSize)
	return &buf
}}

func copyBuffered(dst io.Writer, src io.Reader) (int64, error) {
	buf := copyBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(dst, src, *buf)
	copyBuffers.Put(buf)
	return n, err
}

type freeSpaceCache struct {
	mu        sync.Mutex
	at        time.Time
	available uint64
}

func (c *freeSpaceCache) ensure(rootPath string, size int64) error {
	if 0 >= size {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if freeSpaceTTL < time.Since(c.at) || uint64(size) > c.available {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(rootPath, &stat); nil != err {
			return fmt.Errorf("failed to check free space: %w", err)
		}
		c.available = uint64(stat.Bavail) * uint64(stat.Bsize)
		c.at = time.Now()
	}
	if uint64(size) > c.available {
		return ErrInsufficientStorage
	}
	c.available -= uint64(size)
	return nil
}
