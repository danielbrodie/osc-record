package platform

import "os/exec"

// Stopper knows how to cleanly stop an ffmpeg process on the current platform.
type Stopper interface {
	Stop(cmd *exec.Cmd) error
}

// New returns the platform-appropriate Stopper.
func New() Stopper {
	return newStopper()
}
