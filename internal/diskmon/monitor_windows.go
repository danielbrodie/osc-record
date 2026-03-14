//go:build windows

package diskmon

import (
	"syscall"

	"github.com/danielbrodie/osc-record/internal/tui"
)

func sendDiskStat(path string, send func(tui.DiskStatMsg)) {
	if send == nil {
		return
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64
	if err := syscall.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return
	}

	send(tui.DiskStatMsg{
		Path:       path,
		FreeBytes:  totalFreeBytes,
		TotalBytes: totalBytes,
	})
}
