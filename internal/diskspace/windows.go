//go:build windows

package diskspace

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func Of(path string) (Usage, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if nil != err {
		return Usage{}, fmt.Errorf("read disk space for %s: %w", path, err)
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &free, &total, &totalFree); nil != err {
		return Usage{}, fmt.Errorf("read disk space for %s: %w", path, err)
	}
	return Usage{Free: free, Total: total, Used: total - totalFree}, nil
}
