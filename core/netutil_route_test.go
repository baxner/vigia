package core

import (
	"encoding/binary"
	"testing"
)

func TestParseProcNetRoute(t *testing.T) {
	routeTable := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0100A8C0	0003	0	0	200	00000000	0	0	0
wlan0	00000000	FE01A8C0	0003	0	0	100	00000000	0	0	0
eth0	0000A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
`
	if got := parseProcNetRoute(routeTable, binary.LittleEndian); got != "192.168.1.254" {
		t.Fatalf("LittleEndian got %q, want 192.168.1.254", got)
	}

	beTable := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	C0A80001	0003	0	0	10	00000000	0	0	0
`
	if got := parseProcNetRoute(beTable, binary.BigEndian); got != "192.168.0.1" {
		t.Fatalf("BigEndian got %q, want 192.168.0.1", got)
	}
}

func TestIPAndMACFromLine_SingleDigitOctets(t *testing.T) {
	ip, mac := ipAndMACFromLine("? (192.168.1.10) at 8:0:27:f0:fb:f9 on en0 ifscope [ethernet]")
	if ip != "192.168.1.10" || mac != "08:00:27:f0:fb:f9" {
		t.Fatalf("got (%q, %q), want (192.168.1.10, 08:00:27:f0:fb:f9)", ip, mac)
	}
	ip2, mac2 := ipAndMACFromLine("192.168.1.20 08-00-27-a-b-c dynamic")
	if ip2 != "192.168.1.20" || mac2 != "08:00:27:0a:0b:0c" {
		t.Fatalf("got (%q, %q), want (192.168.1.20, 08:00:27:0a:0b:0c)", ip2, mac2)
	}
}
