package localfs

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/internal/diskspace"
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
		usage, err := diskspace.Of(rootPath)
		if nil != err {
			return fmt.Errorf("failed to check free space: %w", err)
		}
		c.available = usage.Free
		c.at = time.Now()
	}
	if uint64(size) > c.available {
		return ErrInsufficientStorage
	}
	c.available -= uint64(size)
	return nil
}
