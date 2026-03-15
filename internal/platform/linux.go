//go:build linux

package platform

import (
	"io"
	"os"
	"os/exec"
)

type linuxStopper struct{}

func Current() Stopper {
	return linuxStopper{}
}

func (linuxStopper) Stop(cmd *exec.Cmd, stdin io.WriteCloser) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}
