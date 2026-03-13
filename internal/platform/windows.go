//go:build windows

package platform

import (
	"fmt"
	"os/exec"
)

type windowsStopper struct{}

func newStopper() Stopper { return &windowsStopper{} }

func (w *windowsStopper) Stop(cmd *exec.Cmd) error {
	if cmd.Stdin == nil {
		return fmt.Errorf("ffmpeg stdin pipe not available")
	}
	_, err := fmt.Fprintf(cmd.Stdin, "q")
	return err
}
