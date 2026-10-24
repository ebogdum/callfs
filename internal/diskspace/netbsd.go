//go:build netbsd

package diskspace

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func Of(path string) (Usage, error) {
	var stat unix.Statvfs_t
	if err := unix.Statvfs(path, &stat); nil != err {
		return Usage{}, fmt.Errorf("read disk space for %s: %w", path, err)
	}
	size := uint64(stat.Frsize)
	total := uint64(stat.Blocks) * size
	return Usage{Free: uint64(stat.Bavail) * size, Total: total, Used: total - uint64(stat.Bfree)*size}, nil
}
