//go:build windows

package localfs

import (
	"os"
	"time"
)

// Windows-specific helper to handle the fact that Windows doesn't have Unix-style syscall.Stat_t
func extractUnixMetadata(info os.FileInfo) (mode string, uid, gid int, atime, ctime time.Time) {
	// Windows defaults
	mode = "0644"
	uid = 1000
	gid = 1000
	atime = info.ModTime()
	ctime = info.ModTime()

	if info.IsDir() {
		mode = "0755"
	}

	return
}
