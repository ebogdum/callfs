//go:build freebsd || netbsd

package localfs

import (
	"syscall"
	"time"
)

// extractTimestamps extracts access and creation times from syscall.Stat_t on FreeBSD/NetBSD
func extractTimestamps(stat *syscall.Stat_t) (atime, ctime time.Time) {
	atime = time.Unix(int64(stat.Atimespec.Sec), int64(stat.Atimespec.Nsec))
	ctime = time.Unix(int64(stat.Ctimespec.Sec), int64(stat.Ctimespec.Nsec))
	return
}
