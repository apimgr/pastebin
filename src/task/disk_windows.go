//go:build windows

package task

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// diskStats returns the free and total bytes of the volume containing path
// via GetDiskFreeSpaceEx.
func diskStats(path string) (free, total uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("disk stats %s: %w", path, err)
	}
	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, fmt.Errorf("disk stats %s: %w", path, err)
	}
	return freeToCaller, totalBytes, nil
}
