//go:build darwin && !ios

package core

import (
	"strings"
	"syscall"
)

// selfExtra: lo que el propio equipo sabe de sí mismo y no puede oír por mDNS
// (una Mac no se ve a sí misma en su respuesta Bonjour). Mismas claves que
// enrich() saca del TXT _device-info de las demás Macs: modelo ("iMac18,3") y
// versión mayor de Darwin ("24" → macOS 15 Sequoia).
func selfExtra() map[string]string {
	out := map[string]string{}
	if m, err := syscall.Sysctl("hw.model"); err == nil && m != "" {
		out["mdns:model"] = m
	}
	if r, err := syscall.Sysctl("kern.osrelease"); err == nil {
		if major, _, ok := strings.Cut(r, "."); ok && major != "" {
			out["mdns:os"] = major
		}
	}
	return out
}
