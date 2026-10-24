package localfs

import (
	"os"

	"golang.org/x/sys/unix"
)

func syncData(f *os.File, mode SyncMode) error {
	if SyncNone == mode {
		return nil
	}
	return withFd(f, unix.Fdatasync)
}

func syncDirHandle(f *os.File, mode SyncMode) error {
	if SyncNone == mode {
		return nil
	}
	return withFd(f, unix.Fsync)
}

func syncFiles(files []*os.File, mode SyncMode, workers int) []error {
	errs := make([]error, len(files))
	if SyncNone == mode || 0 == len(files) {
		return errs
	}
	if 1 == len(files) {
		errs[0] = syncData(files[0], mode)
		return errs
	}
	err := withFd(files[0], unix.Syncfs)
	for i := range errs {
		errs[i] = err
	}
	return errs
}
