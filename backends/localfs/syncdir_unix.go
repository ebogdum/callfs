//go:build !windows

package localfs

import (
	"fmt"
	"os"
)

func syncDirectory(root *os.Root, dirRel string, mode SyncMode) error {
	if SyncNone == mode {
		return nil
	}
	dir, err := root.Open(dirRel)
	if nil != err {
		return fmt.Errorf("failed to open directory %s for fsync: %w", dirRel, err)
	}
	syncErr := syncDirHandle(dir, mode)
	closeErr := dir.Close()
	if nil != syncErr {
		return fmt.Errorf("failed to fsync directory %s: %w", dirRel, syncErr)
	}
	if nil != closeErr {
		return fmt.Errorf("failed to close directory %s after fsync: %w", dirRel, closeErr)
	}
	return nil
}
