//go:build darwin

package core

import (
	"errors"
	"net"
	"os"
	"syscall"

	"golang.org/x/net/ipv4"
)

const (
	ipBoundIF     = 25  // IP_BOUND_IF (<netinet/in.h>)
	ipv6BoundIF   = 125 // IPV6_BOUND_IF (<netinet6/in6.h>)
	sysIPStripHdr = 0x17
)

func bindControl(iface string) func(network, address string, c syscall.RawConn) error {
	ifc := interfaceByName(iface)
	if ifc == nil {
		return nil
	}
	idx := ifc.Index
	return func(network, _ string, c syscall.RawConn) error {
		_ = c.Control(func(fd uintptr) {
			switch network {
			case "tcp6", "udp6":
				_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, ipv6BoundIF, idx)
			default:
				if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIF, idx); err != nil {
					_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, ipv6BoundIF, idx)
				}
			}
		})
		return nil
	}
}

type darwinICMPConn struct {
	c  net.PacketConn
	p4 *ipv4.PacketConn
}

func (d *darwinICMPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, peer, err := d.p4.ReadFrom(b)
	return n, peer, err
}

func (d *darwinICMPConn) WriteTo(b []byte, dst net.Addr) (int, error) {
	return d.c.WriteTo(b, dst)
}

func (d *darwinICMPConn) Close() error {
	return d.c.Close()
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
	if err := syscall.SetsockoptInt(s, syscall.IPPROTO_IP, sysIPStripHdr, 1); err != nil {
		syscall.Close(s)
		return nil, err
	}
	_ = syscall.SetsockoptInt(s, syscall.IPPROTO_IP, ipBoundIF, ifc.Index)
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
	return &darwinICMPConn{c: c, p4: ipv4.NewPacketConn(c)}, nil
}
