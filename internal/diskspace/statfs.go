//go:build linux || darwin || freebsd

package diskspace

import (
	"fmt"
	"syscall"
)

func Of(path string) (Usage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); nil != err {
		return Usage{}, fmt.Errorf("read disk space for %s: %w", path, err)
	}
	size := uint64(stat.Bsize)
	total := uint64(stat.Blocks) * size
	return Usage{Free: uint64(stat.Bavail) * size, Total: total, Used: total - uint64(stat.Bfree)*size}, nil
}
