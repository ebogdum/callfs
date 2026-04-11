//go:build windows

package localfs

import (
	"os"
	"time"
)

// extractPlatformMetadata extracts filesystem-level mode and timestamps.
// Windows has no Unix-style ownership; CallFS tracks ownership via Metadata.Owner.
func extractPlatformMetadata(info os.FileInfo) (mode string, atime, ctime time.Time) {
	mode = "0644"
	atime = info.ModTime()
	ctime = info.ModTime()

	if info.IsDir() {
		mode = "0755"
	}

	return
}
