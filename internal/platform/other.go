//go:build !darwin && !windows

package platform

import (
	"os"
	"os/exec"
)

type genericStopper struct{}

func newStopper() Stopper { return &genericStopper{} }

func (g *genericStopper) Stop(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}
