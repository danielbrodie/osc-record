//go:build darwin

package platform

import (
	"os"
	"os/exec"
)

type darwinStopper struct{}

func newStopper() Stopper { return &darwinStopper{} }

func (d *darwinStopper) Stop(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}
