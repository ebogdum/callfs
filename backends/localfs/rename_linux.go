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
		return unix.Renameat2(fd, from, fd, to, unix.RENAME_NOREPLACE)
	})
	closeErr := dir.Close()
	if nil != opErr {
		if errors.Is(opErr, unix.ENOSYS) || errors.Is(opErr, unix.EINVAL) || errors.Is(opErr, unix.EOPNOTSUPP) {
			return fmt.Errorf("%w: %w", errExclusiveRenameUnsupported, opErr)
		}
		return &os.LinkError{Op: "renameat2", Old: from, New: to, Err: opErr}
	}
	return closeErr
}
