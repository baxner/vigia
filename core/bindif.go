package core

import (
	"net"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
)

type icmpPacketConn interface {
	ReadFrom(b []byte) (int, net.Addr, error)
	WriteTo(b []byte, dst net.Addr) (int, error)
	Close() error
}

func interfaceByName(name string) *net.Interface {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return nil
	}
	return ifc
}

func newDialer(iface string, timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	if ctrl := bindControl(iface); ctrl != nil {
		d.Control = ctrl
	}
	return d
}

func newListenConfig(iface string, extra func(network, address string, c syscall.RawConn) error) net.ListenConfig {
	return net.ListenConfig{Control: composeControls(extra, bindControl(iface))}
}

func composeControls(fns ...func(network, address string, c syscall.RawConn) error) func(network, address string, c syscall.RawConn) error {
	var active []func(network, address string, c syscall.RawConn) error
	for _, fn := range fns {
		if fn != nil {
			active = append(active, fn)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		for _, fn := range active {
			if err := fn(network, address, c); err != nil {
				return err
			}
		}
		return nil
	}
}

func openICMPConn(iface string) (icmpPacketConn, error) {
	if strings.TrimSpace(iface) != "" {
		if c, err := listenBoundICMP(iface); err == nil {
			return c, nil
		}
	}
	return icmp.ListenPacket("udp4", "0.0.0.0")
}
