//go:build linux || android

package core

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func bindControl(iface string) func(network, address string, c syscall.RawConn) error {
	ifc := interfaceByName(iface)
	if ifc == nil {
		return nil
	}
	name := ifc.Name
	return func(_, _ string, c syscall.RawConn) error {
		_ = c.Control(func(fd uintptr) {
			// Best-effort: fall back gracefully when CAP_NET_RAW is unavailable.
			_ = syscall.BindToDevice(int(fd), name)
		})
		return nil
	}
}

func listenBoundICMP(iface string) (icmpPacketConn, error) {
	ifc := interfaceByName(iface)
	if ifc == nil {
		return nil, errors.New("interface not found")
	}
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, protocolICMP)
	if err != nil {
		return nil, err
	}
	_ = syscall.BindToDevice(s, ifc.Name)
	if err := syscall.Bind(s, &syscall.SockaddrInet4{}); err != nil {
		syscall.Close(s)
		return nil, err
	}
	f := os.NewFile(uintptr(s), "datagram-oriented icmp")
	c, err := net.FilePacketConn(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	return c, nil
}
