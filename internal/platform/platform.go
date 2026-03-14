package platform

import (
	"io"
	"os/exec"
)

type Stopper interface {
	Stop(cmd *exec.Cmd, stdin io.WriteCloser) error
}

type Platform = Stopper
