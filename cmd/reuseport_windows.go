//go:build windows

package cmd

import "net"

// reusePortListenConfig on Windows returns a plain ListenConfig.
// SO_REUSEPORT is not available on Windows; multiple listeners on the
// same port are not supported in this configuration.
func reusePortListenConfig() net.ListenConfig {
	return net.ListenConfig{}
}
