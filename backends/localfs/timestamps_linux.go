//go:build linux

package localfs

import (
	"syscall"
	"time"
)

// extractTimestamps extracts access and creation times from syscall.Stat_t on Linux
func extractTimestamps(stat *syscall.Stat_t) (atime, ctime time.Time) {
	atime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	ctime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
	return
}
