//go:build ios

package core

import (
	"strings"
	"syscall"
)

// selfExtra: en iOS el único dato fiable vía sysctl es el identificador de
// modelo hardware ("iPhone16,1"). No tocamos mdns:os porque kern.osrelease en
// iOS no corresponde a versiones de macOS.
func selfExtra() map[string]string {
	out := map[string]string{}
	if m, err := syscall.Sysctl("hw.machine"); err == nil {
		if m = strings.TrimSpace(strings.TrimRight(m, "\x00")); m != "" {
			out["mdns:model"] = m
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
