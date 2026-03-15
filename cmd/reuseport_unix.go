//go:build !windows

package cmd

import (
	"net"
	"syscall"
)

// reusePortListenConfig returns a net.ListenConfig that sets SO_REUSEPORT,
// allowing multiple processes (e.g. osc-record and Protokol) to receive
// from the same UDP port simultaneously.
func reusePortListenConfig() net.ListenConfig {
	return net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var setSockOptErr error
			err := c.Control(func(fd uintptr) {
				setSockOptErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}
			return setSockOptErr
		},
	}
}
