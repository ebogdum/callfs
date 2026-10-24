//go:build linux || darwin

package localfs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func withFd(f *os.File, op func(fd int) error) error {
	conn, err := f.SyscallConn()
	if nil != err {
		return err
	}
	var opErr error
	ctlErr := conn.Control(func(fd uintptr) {
		for {
			opErr = op(int(fd))
			if !errors.Is(opErr, unix.EINTR) {
				return
			}
		}
	})
	if nil != ctlErr {
		return ctlErr
	}
	return opErr
}
