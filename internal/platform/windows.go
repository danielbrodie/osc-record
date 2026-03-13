//go:build windows

package platform

import (
	"io"
	"os/exec"
)

type windowsStopper struct{}

func Current() Stopper {
	return windowsStopper{}
}

func (windowsStopper) Stop(cmd *exec.Cmd, stdin io.WriteCloser) error {
	if stdin == nil {
		return nil
	}
	_, err := io.WriteString(stdin, "q\n")
	return err
}
