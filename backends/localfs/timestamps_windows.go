//go:build windows

package localfs

import (
	"time"
)

// extractTimestamps extracts access and creation times on Windows
// Windows doesn't have syscall.Stat_t in the same way, so we return current time as fallback
// The main adapter.go code should handle Windows differently
func extractTimestamps(stat interface{}) (atime, ctime time.Time) {
	now := time.Now()
	return now, now
}
