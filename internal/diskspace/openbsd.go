//go:build openbsd

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
	size := uint64(stat.F_bsize)
	total := uint64(stat.F_blocks) * size
	return Usage{Free: uint64(stat.F_bavail) * size, Total: total, Used: total - uint64(stat.F_bfree)*size}, nil
}
