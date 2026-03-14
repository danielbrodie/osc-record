//go:build windows

package diskmon

import (
	"syscall"
	"unsafe"

	"github.com/danielbrodie/osc-record/internal/tui"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func sendDiskStat(path string, send func(tui.DiskStatMsg)) {
	if send == nil {
		return
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r == 0 {
		return
	}

	send(tui.DiskStatMsg{
		Path:       path,
		FreeBytes:  totalFreeBytes,
		TotalBytes: totalBytes,
	})
}
