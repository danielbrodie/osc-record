//go:build darwin

package platform

import (
	"io"
	"os"
	"os/exec"
)

type darwinStopper struct{}

func Current() Stopper {
	return darwinStopper{}
}

func (darwinStopper) Stop(cmd *exec.Cmd, stdin io.WriteCloser) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}
