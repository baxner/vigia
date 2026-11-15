package agent

import (
	"net"
	"sort"
)

// LocalIPv4Addresses devuelve las IPv4 de todas las interfaces activas
// (arriba, sin loopback ni link-local) — incluye tanto la LAN como, si
// está conectada, la interfaz de Tailscale (100.64.0.0/10): no hace
// falta tratarla distinto, es una interfaz real más con su propia IPv4.
// Ver GET /api/v1/info, "addresses" en docs/agente-api.md: para que la
// app alcance al agente fuera de casa. Nunca nil (lista vacía si no hay
// ninguna) y en orden estable, para que /info no cambie de orden entre
// llamadas sin motivo.
func LocalIPv4Addresses() []string {
	out := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := n.IP.To4()
			if v4 == nil || v4.IsLinkLocalUnicast() {
				continue
			}
			s := v4.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}
