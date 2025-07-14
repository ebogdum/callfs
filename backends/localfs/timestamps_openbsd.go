//go:build openbsd

package localfs

import (
	"syscall"
	"time"
)

// extractTimestamps extracts access and creation times from syscall.Stat_t on OpenBSD
func extractTimestamps(stat *syscall.Stat_t) (atime, ctime time.Time) {
	atime = time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec))
	ctime = time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
	return
}
