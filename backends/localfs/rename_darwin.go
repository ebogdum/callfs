package localfs

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(root *os.Root, dirRel, from, to string) error {
	dir, err := root.Open(dirRel)
	if nil != err {
		return err
	}
	opErr := withFd(dir, func(fd int) error {
		return unix.RenameatxNp(fd, from, fd, to, unix.RENAME_EXCL)
	})
	closeErr := dir.Close()
	if nil != opErr {
		if errors.Is(opErr, unix.ENOTSUP) || errors.Is(opErr, unix.EOPNOTSUPP) || errors.Is(opErr, unix.EINVAL) || errors.Is(opErr, unix.ENOSYS) {
			return fmt.Errorf("%w: %w", errExclusiveRenameUnsupported, opErr)
		}
		return &os.LinkError{Op: "renameatx_np", Old: from, New: to, Err: opErr}
	}
	return closeErr
}
