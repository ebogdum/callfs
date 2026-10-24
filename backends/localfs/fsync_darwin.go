package localfs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func fcntlSync(f *os.File, cmd int) error {
	return withFd(f, func(fd int) error {
		_, err := unix.FcntlInt(uintptr(fd), cmd, 0)
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.EINVAL) {
			return unix.Fsync(fd)
		}
		return err
	})
}

func syncData(f *os.File, mode SyncMode) error {
	switch mode {
	case SyncNone:
		return nil
	case SyncBarrier:
		return fcntlSync(f, unix.F_BARRIERFSYNC)
	default:
		return fcntlSync(f, unix.F_FULLFSYNC)
	}
}

func syncDirHandle(f *os.File, mode SyncMode) error {
	return syncData(f, mode)
}

func syncFiles(files []*os.File, mode SyncMode, workers int) []error {
	if SyncNone == mode || 0 == len(files) {
		return make([]error, len(files))
	}
	if SyncBarrier == mode {
		return parallelSync(files, workers, func(f *os.File) error {
			return fcntlSync(f, unix.F_BARRIERFSYNC)
		})
	}
	errs := parallelSync(files, workers, func(f *os.File) error {
		return withFd(f, unix.Fsync)
	})
	if err := fcntlSync(files[0], unix.F_FULLFSYNC); nil != err {
		for i := range errs {
			if nil == errs[i] {
				errs[i] = err
			}
		}
	}
	return errs
}
