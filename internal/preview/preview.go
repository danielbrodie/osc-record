// Package preview grabs a single video frame from a capture device and opens
// it with the system default image viewer.
package preview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// GrabFrame captures one frame from the given ffmpeg input args and opens the
// result in the system image viewer. Returns the path to the JPEG on success.
func GrabFrame(ffmpegPath string, inputArgs []string, device string) (string, error) {
	// Write to a fixed temp path so repeated presses refresh the same Preview window.
	path := filepath.Join(os.TempDir(), "osc-record-preview.jpg")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	args := append(inputArgs, "-frames:v", "1", "-q:v", "2", "-y", path)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("frame grab failed: %w\n%s", err, string(out))
	}

	if err := openFile(path); err != nil {
		return path, fmt.Errorf("frame saved to %s but viewer failed: %w", path, err)
	}

	return path, nil
}

// openFile opens path with the system default application.
func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", path)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start() // fire and forget — don't wait for viewer to close
}
