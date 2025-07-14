//go:build darwin

package localfs

import (
	"syscall"
	"time"
)

// extractTimestamps extracts access and creation times from syscall.Stat_t on Darwin (macOS)
func extractTimestamps(stat *syscall.Stat_t) (atime, ctime time.Time) {
	atime = time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec)
	ctime = time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec)
	return
}
