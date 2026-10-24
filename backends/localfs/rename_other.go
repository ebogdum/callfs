//go:build !linux && !darwin

package localfs

import "os"

func renameExclusive(root *os.Root, dirRel, from, to string) error {
	return errExclusiveRenameUnsupported
}
