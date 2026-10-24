//go:build !windows

package localfs

import (
	"os"
	"syscall"
	"time"
)

// extractPlatformMetadata extracts filesystem-level mode and timestamps.
// OS-level UIDs/GIDs are NOT extracted — they have no relationship to
// CallFS application users. Ownership is tracked via Metadata.Owner.
func extractPlatformMetadata(info os.FileInfo) (mode string, atime, ctime time.Time) {
	// Default values
	mode = "0644"
	atime = info.ModTime()
	ctime = info.ModTime()

	if info.IsDir() {
		mode = "0777"
	}

	// Extract Unix permissions and timestamps (NOT ownership)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		atime, ctime = extractTimestamps(stat)
	}

	return
}
