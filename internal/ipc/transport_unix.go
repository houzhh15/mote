//go:build !windows
// +build !windows

package ipc

import (
	"net"
)

func listenPipe(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}

func dialPipe(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}
