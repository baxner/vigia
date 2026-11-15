package core

import (
	"context"
	"testing"
)

// router_test.go: QueryRouter como orquestador (UPnP + NAT-PMP juntos,
// fusión de ExternalIP, Reach). Los caminos de cada protocolo por separado
// están en upnp_test.go/natpmp_test.go.

func TestQueryRouter_UPnPYNATPMPJuntos(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", []testMapping{
		{extPort: 8080, intPort: 80, proto: "TCP", intIP: "192.168.0.55", desc: "cam"},
	})
	withSSDPPort(t, fakeSSDPGateway(t, srv.URL+"/rootDesc.xml"))
	withNATPMPPort(t, fakeNATPMPServer(t, func(req []byte) []byte {
		return natpmpSuccessResponse("192.168.100.2")
	}))

	info, err := QueryRouter(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if info == nil {
		t.Fatal("info = nil, want *RouterInfo (UPnP y NAT-PMP contestaron)")
	}
	if !info.UPnP || !info.NATPMP {
		t.Errorf("UPnP=%v NATPMP=%v, want ambos true", info.UPnP, info.NATPMP)
	}
	if info.IP != "127.0.0.1" {
		t.Errorf("IP = %q, want %q", info.IP, "127.0.0.1")
	}
	if info.ExternalIP != "192.168.100.2" {
		t.Errorf("ExternalIP = %q, want %q", info.ExternalIP, "192.168.100.2")
	}
	if info.Reach != "outer_ring" {
		t.Errorf("Reach = %q, want outer_ring (ExternalIP privada)", info.Reach)
	}
	if len(info.PortMappings) != 1 || info.PortMappings[0].ExternalPort != 8080 {
		t.Errorf("PortMappings = %+v, want 1 redirección en el puerto 8080", info.PortMappings)
	}
	// 127.0.0.1 nunca tiene entrada ARP real (loopback): MAC debe quedar
	// vacía, no inventada — sanity check de que neighborTable() es
	// best-effort y no rompe nada cuando no hay nada que encontrar.
	if info.MAC != "" {
		t.Errorf("MAC = %q, want vacío (gateway de loopback, sin ARP)", info.MAC)
	}
}

// TestQueryRouter_SoloNATPMP: UPnP apagado en el router, NAT-PMP contestando
// con una IP pública — Reach debe salir "internet" y PortMappings vacío (sin
// UPnP no hay de dónde sacar redirecciones).
func TestQueryRouter_SoloNATPMP(t *testing.T) {
	withSSDPPort(t, closedUDPPort(t))
	withNATPMPPort(t, fakeNATPMPServer(t, func(req []byte) []byte {
		return natpmpSuccessResponse("8.8.8.8")
	}))

	info, err := QueryRouter(context.Background(), "127.0.0.1")
	if err != nil || info == nil {
		t.Fatalf("info=%+v err=%v, want info no nil, err nil", info, err)
	}
	if info.UPnP {
		t.Error("UPnP = true, want false (nadie contestó SSDP)")
	}
	if !info.NATPMP {
		t.Error("NATPMP = false, want true")
	}
	if info.ExternalIP != "8.8.8.8" {
		t.Errorf("ExternalIP = %q, want %q", info.ExternalIP, "8.8.8.8")
	}
	if info.Reach != "internet" {
		t.Errorf("Reach = %q, want internet (ExternalIP pública)", info.Reach)
	}
	if len(info.PortMappings) != 0 {
		t.Errorf("PortMappings = %v, want vacío", info.PortMappings)
	}
}

// TestQueryRouter_PrefiereExternalIPDeUPnP: si UPnP y NAT-PMP contestan con
// IPs distintas (no debería pasar en la práctica, pero el router podría
// estar a medio reconfigurar), QueryRouter se queda con la de UPnP.
func TestQueryRouter_PrefiereExternalIPDeUPnP(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", nil)
	withSSDPPort(t, fakeSSDPGateway(t, srv.URL+"/rootDesc.xml"))
	withNATPMPPort(t, fakeNATPMPServer(t, func(req []byte) []byte {
		return natpmpSuccessResponse("192.168.100.99")
	}))

	info, err := QueryRouter(context.Background(), "127.0.0.1")
	if err != nil || info == nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if info.ExternalIP != "192.168.100.2" {
		t.Errorf("ExternalIP = %q, want %q (preferencia por UPnP)", info.ExternalIP, "192.168.100.2")
	}
}
