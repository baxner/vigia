package agent

import (
	"net"
	"testing"
)

// LocalIPv4Addresses depende de las interfaces reales de la máquina que
// corre el test, así que no podemos fijar qué IPs exactas esperar; lo que
// sí podemos (y debemos) verificar son sus invariantes: nunca nil, cada
// entrada es una IPv4 válida, nunca loopback ni link-local, sin
// duplicados, y estable entre llamadas.
func TestLocalIPv4Addresses_Invariantes(t *testing.T) {
	got := LocalIPv4Addresses()
	if got == nil {
		t.Fatal("LocalIPv4Addresses() = nil, want al menos [] (nunca nil)")
	}

	seen := map[string]bool{}
	for _, s := range got {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("%q no es una IP válida", s)
		}
		v4 := ip.To4()
		if v4 == nil {
			t.Fatalf("%q no es IPv4", s)
		}
		if v4.IsLoopback() {
			t.Fatalf("%q es loopback, no debería listarse", s)
		}
		if v4.IsLinkLocalUnicast() {
			t.Fatalf("%q es link-local, no debería listarse", s)
		}
		if seen[s] {
			t.Fatalf("%q está duplicada", s)
		}
		seen[s] = true
	}

	again := LocalIPv4Addresses()
	if len(again) != len(got) {
		t.Fatalf("dos llamadas consecutivas dieron distinto largo: %v vs %v", got, again)
	}
	for i := range got {
		if got[i] != again[i] {
			t.Fatalf("orden inestable entre llamadas: %v vs %v", got, again)
		}
	}
}
