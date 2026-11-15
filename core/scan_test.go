package core

import (
	"context"
	"net"
	"sort"
	"testing"
	"time"
)

// TestNetbiosTargetsRedChica: en una red de tamaño normal (≤1024 hosts, p.
// ej. un /24 con 254) se consulta NetBIOS a TODA la subred, sin importar qué
// haya encontrado ya el barrido — así se puede correr en paralelo con él
// (ver Scan) en vez de esperarlo.
func TestNetbiosTargetsRedChica(t *testing.T) {
	hosts := make([]string, 254)
	for i := range hosts {
		hosts[i] = "192.168.0.X" // el contenido no importa para esta regla, solo el tamaño
	}
	live := map[string][]string{"192.168.0.53": {"icmp"}} // solo un hallazgo del barrido

	got := netbiosTargets(hosts, live)
	if len(got) != len(hosts) {
		t.Fatalf("len(netbiosTargets) = %d, want %d (la subred entera)", len(got), len(hosts))
	}
}

// TestNetbiosTargetsRedGrande: por encima del umbral, solo se consultan las
// IPs que el barrido ya marcó como vivas (demasiado caro preguntar una por
// una a una red grande entera).
func TestNetbiosTargetsRedGrande(t *testing.T) {
	hosts := make([]string, netbiosFullScanLimit+1)
	for i := range hosts {
		hosts[i] = "10.0.0.X"
	}
	live := map[string][]string{
		"10.0.0.5":  {"ping"},
		"10.0.0.9":  {"icmp"},
		"10.0.0.12": {"ping", "icmp"},
	}

	got := netbiosTargets(hosts, live)
	sort.Strings(got)
	want := []string{"10.0.0.12", "10.0.0.5", "10.0.0.9"}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("netbiosTargets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("netbiosTargets = %v, want %v", got, want)
		}
	}
}

// TestNetbiosTargetsUmbralExacto: justo en el límite (netbiosFullScanLimit
// hosts) todavía cuenta como "red chica" — la condición es `> limit`, no
// `>= limit`.
func TestNetbiosTargetsUmbralExacto(t *testing.T) {
	hosts := make([]string, netbiosFullScanLimit)
	for i := range hosts {
		hosts[i] = "172.16.0.X"
	}
	live := map[string][]string{} // sin nada vivo todavía: no debería importar en este caso

	got := netbiosTargets(hosts, live)
	if len(got) != len(hosts) {
		t.Fatalf("en el umbral exacto (%d) se esperaba la subred entera, got len=%d", netbiosFullScanLimit, len(got))
	}
}

// TestLiveIPs comprueba el helper que convierte el mapa ip→fuentes de
// sweepLiveness en la lista plana de IPs que usa netbiosTargets en redes
// grandes.
func TestLiveIPs(t *testing.T) {
	m := map[string][]string{
		"192.168.0.53": {"icmp"},
		"192.168.0.55": {"ping", "netbios"},
	}
	got := liveIPs(m)
	sort.Strings(got)
	want := []string{"192.168.0.53", "192.168.0.55"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("liveIPs(%v) = %v, want %v", m, got, want)
	}

	if got := liveIPs(nil); len(got) != 0 {
		t.Errorf("liveIPs(nil) = %v, want vacío", got)
	}
}

// TestFilterToNet comprueba el candado final de un escaneo Routed (ver
// Scan): cualquier IP fuera de la red pedida se descarta, sin tocar las que
// sí están dentro.
func TestFilterToNet(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("192.168.100.0/24")
	if err != nil {
		t.Fatal(err)
	}
	devs := map[string]*Device{
		"192.168.100.5": {IP: "192.168.100.5"},
		"192.168.100.1": {IP: "192.168.100.1"},
		"192.168.0.55":  {IP: "192.168.0.55"}, // anillo interior: no debe sobrevivir
		"10.0.0.9":      {IP: "10.0.0.9"},     // tampoco
	}

	filterToNet(devs, ipNet)

	if len(devs) != 2 {
		t.Fatalf("len(devs) = %d, want 2; devs = %v", len(devs), devs)
	}
	for _, ip := range []string{"192.168.100.5", "192.168.100.1"} {
		if _, ok := devs[ip]; !ok {
			t.Errorf("%s no debía filtrarse (está dentro de 192.168.100.0/24)", ip)
		}
	}
	for _, ip := range []string{"192.168.0.55", "10.0.0.9"} {
		if _, ok := devs[ip]; ok {
			t.Errorf("%s debía filtrarse (está fuera de 192.168.100.0/24)", ip)
		}
	}
}

func TestFilterToNet_MapaVacio(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/8")
	devs := map[string]*Device{}
	filterToNet(devs, ipNet) // no debe entrar en pánico con un mapa vacío
	if len(devs) != 0 {
		t.Errorf("len(devs) = %d, want 0", len(devs))
	}
}

// ===========================================================================
// ScanOptions.Routed: una red Routed se debe saltar mDNS/SSDP/Tuya/ARP (el
// multicast no cruza el router) — se comprueba por tiempo, igual que
// TestDiscoverTuyaTimeout. discoverMDNS/discoverSSDP escuchan la ventana
// COMPLETA de DiscoverTimeout cuando nadie contesta (no hay forma de saber
// que "ya no va a llegar nada"); sweepLiveness, en cambio, tiene un piso
// propio de ~2s (icmpSweep, ver netutil.go) sin importar DiscoverTimeout. Con
// DiscoverTimeout=5s: si Routed=true de verdad se salta mDNS/SSDP/Tuya, el
// escaneo entero queda acotado por ese piso de ~2s, bien por debajo de los
// 5s; si NO se saltaran, tardaría los 5s completos. 203.0.113.0/30
// (TEST-NET-3, RFC 5737) nunca es una interfaz local ni tiene nada vivo: sin
// red real de por medio.
// ===========================================================================

func TestScanRoutedSkipsMulticastStages_Explicito(t *testing.T) {
	opts := ScanOptions{
		CIDR:            "203.0.113.0/30",
		Routed:          true,
		SweepTimeout:    150 * time.Millisecond,
		DiscoverTimeout: 5 * time.Second,
		Ports:           []int{},
	}
	start := time.Now()
	res, err := Scan(context.Background(), opts)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("Scan(Routed=true) tardó %s; con DiscoverTimeout=5s debía saltarse mDNS/SSDP/Tuya y quedar acotado por el piso de ~2s del barrido ICMP, no por los 5s", elapsed)
	}
	if len(res.Devices) != 0 {
		t.Errorf("Devices = %+v, want vacío (203.0.113.0/30 no tiene nada vivo)", res.Devices)
	}
	if res.Self != "" {
		t.Errorf("Self = %q, want vacío (sin interfaz propia en una red Routed)", res.Self)
	}
}

// TestScanRoutedSkipsMulticastStages_AutoDetectado: lo mismo, pero sin fijar
// Routed: true — Scan debe detectarlo solo porque 203.0.113.0/30 no
// coincide con ninguna interfaz local de la máquina de prueba.
func TestScanRoutedSkipsMulticastStages_AutoDetectado(t *testing.T) {
	opts := ScanOptions{
		CIDR:            "203.0.113.0/30",
		SweepTimeout:    150 * time.Millisecond,
		DiscoverTimeout: 5 * time.Second,
		Ports:           []int{},
	}
	start := time.Now()
	_, err := Scan(context.Background(), opts)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("Scan() con CIDR ajeno a toda interfaz local tardó %s; debía autodetectarse como Routed", elapsed)
	}
}
