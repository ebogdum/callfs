package localfs

import "os"

func syncData(f *os.File, mode SyncMode) error {
	if SyncNone == mode {
		return nil
	}
	return f.Sync()
}

func syncDirectory(root *os.Root, dirRel string, mode SyncMode) error {
	return nil
}

func syncFiles(files []*os.File, mode SyncMode, workers int) []error {
	return parallelSync(files, workers, func(f *os.File) error {
		return syncData(f, mode)
	})
}
