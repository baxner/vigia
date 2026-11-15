package core

import (
	"net"
	"testing"
)

// TestAddrIP comprueba que addrIP saca la IP de los dos tipos de net.Addr que
// de verdad vemos (ReadFrom de un socket "udp4" con icmp.PacketConn o con
// net.ListenConfig, ver icmp.go/tuya.go), y que no entra en pánico con otra
// cosa.
func TestAddrIP(t *testing.T) {
	casos := []struct {
		nombre string
		addr   net.Addr
		want   string
	}{
		{"UDPAddr", &net.UDPAddr{IP: net.ParseIP("192.168.0.53"), Port: 6666}, "192.168.0.53"},
		{"IPAddr", &net.IPAddr{IP: net.ParseIP("192.168.0.143")}, "192.168.0.143"},
		{"nil", nil, ""},
	}
	for _, c := range casos {
		if got := addrIP(c.addr); got != c.want {
			t.Errorf("%s: addrIP(%v) = %q, want %q", c.nombre, c.addr, got, c.want)
		}
	}
}

// otroAddr: un net.Addr que no es ni *net.UDPAddr ni *net.IPAddr, para
// comprobar que addrIP no asume uno de esos dos tipos a la fuerza.
type otroAddr struct{}

func (otroAddr) Network() string { return "otro" }
func (otroAddr) String() string  { return "otro" }

func TestAddrIPTipoDesconocido(t *testing.T) {
	if got := addrIP(otroAddr{}); got != "" {
		t.Errorf("addrIP(otroAddr{}) = %q, want \"\"", got)
	}
}

// mustCIDR: ParseCIDR que falla el test en vez de devolver un error, para no
// repetir el manejo de err en cada caso de TestNetworkHasLocalAddr.
func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("net.ParseCIDR(%q): %v", s, err)
	}
	return n
}

// TestNetworkHasLocalAddr prueba, con una lista de direcciones armada a
// mano (sin net.Interfaces() real), la regla que isRoutedNetwork usa para
// decidir si una red es "de delante del router" (ver ScanOptions.Routed):
// Routed es, exactamente, que NINGUNA dirección local caiga dentro de la
// red pedida.
func TestNetworkHasLocalAddr(t *testing.T) {
	innerRingAddrs := []net.IP{net.ParseIP("192.168.0.48"), net.ParseIP("127.0.0.1")}

	// La red del anillo interior (la nuestra): SÍ tiene una dirección local
	// ahí -> no es Routed.
	if !networkHasLocalAddr(mustCIDR(t, "192.168.0.0/24"), innerRingAddrs) {
		t.Error("networkHasLocalAddr(192.168.0.0/24) = false, want true (192.168.0.48 es nuestra)")
	}
	// La red del anillo exterior (la de delante del router): ninguna
	// dirección local cae ahí -> es Routed.
	if networkHasLocalAddr(mustCIDR(t, "192.168.100.0/24"), innerRingAddrs) {
		t.Error("networkHasLocalAddr(192.168.100.0/24) = true, want false (no tenemos interfaz ahí)")
	}
	// Sin direcciones que comprobar: nunca hay match.
	if networkHasLocalAddr(mustCIDR(t, "192.168.0.0/24"), nil) {
		t.Error("networkHasLocalAddr(..., nil) = true, want false")
	}
}

// TestIsRoutedNetwork_RedPropiaDeEsteEquipo: isRoutedNetwork sobre la red
// real de la máquina de prueba (localhost siempre tiene 127.0.0.0/8) debe
// decir que NO es Routed: es justo la definición ("ninguna interfaz local
// tiene una dirección ahí").
func TestIsRoutedNetwork_RedPropiaDeEsteEquipo(t *testing.T) {
	if isRoutedNetwork(mustCIDR(t, "127.0.0.0/8")) {
		t.Error("isRoutedNetwork(127.0.0.0/8) = true, want false (loopback es siempre una interfaz local)")
	}
}

// TestIsRoutedNetwork_RedDocumentalInalcanzable: un rango reservado para
// documentación (RFC 5737, TEST-NET-1) nunca está asignado a una interfaz
// real: isRoutedNetwork debe tratarlo como Routed.
func TestIsRoutedNetwork_RedDocumentalInalcanzable(t *testing.T) {
	if !isRoutedNetwork(mustCIDR(t, "192.0.2.0/24")) {
		t.Error("isRoutedNetwork(192.0.2.0/24) = false, want true (TEST-NET-1, nunca es una interfaz local)")
	}
}
