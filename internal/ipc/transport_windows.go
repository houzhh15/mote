//go:build windows
// +build windows

package ipc

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func listenPipe(path string) (net.Listener, error) {
	return winio.ListenPipe(path, nil)
}

func dialPipe(path string) (net.Conn, error) {
	return winio.DialPipe(path, nil)
}
