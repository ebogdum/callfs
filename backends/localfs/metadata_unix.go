//go:build !windows

package localfs

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// Unix-specific helper to extract metadata using syscall.Stat_t
func extractUnixMetadata(info os.FileInfo) (mode string, uid, gid int, atime, ctime time.Time) {
	// Default values
	mode = "0644"
	uid = 1000
	gid = 1000
	atime = info.ModTime()
	ctime = info.ModTime()

	if info.IsDir() {
		mode = "0755"
	}

	// Extract Unix permissions and ownership
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		mode = fmt.Sprintf("0%o", stat.Mode&0777)
		uid = int(stat.Uid)
		gid = int(stat.Gid)
		// Extract timestamps using platform-specific approach
		atime, ctime = extractTimestamps(stat)
	}

	return
}
