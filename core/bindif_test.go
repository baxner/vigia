package core

import (
	"context"
	"net"
	"testing"
	"time"
)

func loopbackInterfaceName(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces: %v", err)
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp != 0 && ifc.Flags&net.FlagLoopback != 0 {
			return ifc.Name
		}
	}
	t.Skip("no active loopback interface found")
	return ""
}

func TestInterfaceByNameAndBindFallback(t *testing.T) {
	if ifc := interfaceByName(""); ifc != nil {
		t.Fatalf("empty name -> %+v, want nil", ifc)
	}
	if ifc := interfaceByName("   "); ifc != nil {
		t.Fatalf("whitespace name -> %+v, want nil", ifc)
	}
	if ifc := interfaceByName("nonexistent-vigia-iface-0"); ifc != nil {
		t.Fatalf("nonexistent -> %+v, want nil", ifc)
	}

	lo := loopbackInterfaceName(t)
	if ifc := interfaceByName(" " + lo + " "); ifc == nil || ifc.Name != lo {
		t.Fatalf("interfaceByName(%q) = %+v", lo, ifc)
	}

	if ctrl := bindControl("nonexistent-vigia-iface-0"); ctrl != nil {
		t.Fatal("bindControl on nonexistent interface should return nil for graceful fallback")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Dial via loopback interface and via nonexistent interface (fallback).
	for _, iface := range []string{lo, "nonexistent-vigia-iface-0", ""} {
		d := newDialer(iface, time.Second)
		c, err := d.DialContext(ctx, "tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("DialContext(iface=%q): %v", iface, err)
		}
		_ = c.Close()
	}

	// ListenConfig with nonexistent interface still binds cleanly.
	lc := newListenConfig("nonexistent-vigia-iface-0", reusePortControl)
	pc, err := lc.ListenPacket(ctx, "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket fallback: %v", err)
	}
	_ = pc.Close()
}

func TestScanHostPortsOnInterface(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	tcpAddr := ln.Addr().(*net.TCPAddr)
	lo := loopbackInterfaceName(t)

	for _, iface := range []string{lo, "nonexistent-vigia-iface-0"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		open := ScanHostPortsOnInterface(ctx, "127.0.0.1", []int{tcpAddr.Port}, 500*time.Millisecond, iface)
		cancel()
		if len(open) != 1 || open[0] != tcpAddr.Port {
			t.Fatalf("ScanHostPortsOnInterface(%q) = %v, want [%d]", iface, open, tcpAddr.Port)
		}
	}
}

func TestQueryRouterOnInterfaceValidationAndFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	if _, err := QueryRouterOnInterface(ctx, "not-an-ip", "lo0"); err == nil {
		t.Fatal("expected error for invalid gateway")
	}

	// Unreachable loopback gateway with nonexistent interface falls back cleanly without error/panic.
	info, err := QueryRouterOnInterface(ctx, "127.0.0.1", "nonexistent-vigia-iface-0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil RouterInfo when no UPnP/NAT-PMP responder, got %+v", info)
	}
}

func TestLocalNetworkOnInterfaceAndScanFallback(t *testing.T) {
	ipNet, ip, err := localNetworkOnInterface("198.51.100.10/24", "nonexistent-vigia-iface-0")
	if err != nil {
		t.Fatalf("localNetworkOnInterface with CIDR: %v", err)
	}
	if ipNet.String() != "198.51.100.0/24" || ip.String() != "198.51.100.10" {
		t.Fatalf("got (%v, %v)", ipNet, ip)
	}

	// Scan on a /32 loopback target with a nonexistent BindInterface must fall back gracefully.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Scan(ctx, ScanOptions{
		CIDR:              "127.0.0.1/32",
		BindInterface:     "nonexistent-vigia-iface-0",
		Ports:             []int{port},
		SweepTimeout:      250 * time.Millisecond,
		DiscoverTimeout:   150 * time.Millisecond,
		PortTimeout:       300 * time.Millisecond,
		SkipNeighborTable: true,
	})
	if err != nil {
		t.Fatalf("Scan with BindInterface fallback: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil ScanResult")
	}
}

func TestOpenICMPConnFallback(t *testing.T) {
	c, err := openICMPConn("nonexistent-vigia-iface-0")
	if err == nil {
		_ = c.Close()
	}
	// Even if unprivileged ICMP sockets are disabled in sandboxed environments,
	// openICMPConn must never panic and returns cleanly.
}
