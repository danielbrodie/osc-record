//go:build linux

package diskmon

import (
	"syscall"

	"github.com/danielbrodie/osc-record/internal/tui"
)

func sendDiskStat(path string, send func(tui.DiskStatMsg)) {
	if send == nil {
		return
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}

	blockSize := uint64(stat.Bsize)
	send(tui.DiskStatMsg{
		Path:       path,
		FreeBytes:  stat.Bavail * blockSize,
		TotalBytes: stat.Blocks * blockSize,
	})
}
