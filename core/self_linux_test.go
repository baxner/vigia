//go:build linux

package core

import "testing"

func TestParseOSRelease(t *testing.T) {
	in := `PRETTY_NAME="Armbian 24.5.1 bookworm"
NAME="Debian GNU/Linux"
VERSION_ID="12"
`
	if got := parseOSRelease(in); got != "Armbian 24.5.1 bookworm" {
		t.Fatalf("got %q, want Armbian 24.5.1 bookworm", got)
	}

	fallback := `NAME='Alpine Linux'
VERSION_ID=3.20
`
	if got := parseOSRelease(fallback); got != "Alpine Linux 3.20" {
		t.Fatalf("got %q, want Alpine Linux 3.20", got)
	}
}

func TestParseDeviceTreeModel(t *testing.T) {
	if got := parseDeviceTreeModel("Orange Pi Zero 3\x00\n"); got != "Orange Pi Zero 3" {
		t.Fatalf("got %q, want Orange Pi Zero 3", got)
	}
}
