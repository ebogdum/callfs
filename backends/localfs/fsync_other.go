//go:build !linux && !darwin && !windows

package localfs

import "os"

func syncData(f *os.File, mode SyncMode) error {
	if SyncNone == mode {
		return nil
	}
	return f.Sync()
}

func syncDirHandle(f *os.File, mode SyncMode) error {
	return syncData(f, mode)
}

func syncFiles(files []*os.File, mode SyncMode, workers int) []error {
	return parallelSync(files, workers, func(f *os.File) error {
		return syncData(f, mode)
	})
}
