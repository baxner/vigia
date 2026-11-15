package history

import (
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internal/ptrs"
	"github.com/baxner/vigia/core"
)

// Puerto 1:1 de DeviceReconcilerTest.kt: misma red, mismo t0, mismos casos
// (incluidos los de nombres en los comentarios). La reconciliación es pura:
// se prueba armando KnownDevice y core.Device a mano, sin disco ni reloj real.

var reconcilerT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func dev(ip string, mac string, hostname string, extra map[string]string) core.Device {
	return core.Device{IP: ip, MAC: mac, Hostname: hostname, Extra: extra}
}

// knownWithMAC: por defecto la MAC "pegajosa" es la del último Device visto
// (si no está vacía); macOverride, si no es "", la reemplaza — para el caso
// en que el conocido ya tenía MAC de una vez anterior pero el escaneo que
// quedó guardado como `last` no la traía (ver TestReconcile_EstacionPorNombreFuerteAunqueCambieIP).
func known(id int64, last core.Device, macOverride string) KnownDevice {
	mac := macOverride
	if mac == "" {
		mac = last.MAC
	}
	var macPtr *string
	if mac != "" {
		macPtr = ptrs.To(mac)
	}
	return KnownDevice{
		ID:          id,
		Network:     NetworkKey("Casa|192.168.0.0/24"),
		MAC:         macPtr,
		LastIP:      last.IP,
		FirstSeen:   reconcilerT0,
		LastSeen:    reconcilerT0,
		Online:      true,
		OnlineSince: ptrs.To(reconcilerT0),
		Last:        last,
	}
}

func TestReconcile_MismaMACJuntaAunqueCambieIP(t *testing.T) {
	k := known(1, dev("192.168.0.10", "aa:bb:cc:dd:ee:01", "", nil), "")
	escaneado := dev("192.168.0.99", "aa:bb:cc:dd:ee:01", "", nil)

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("esperaba match con id=1, got %+v", result[0].Known)
	}
}

func TestReconcile_MismoSSDPUDNJuntaAunqueNoHayaMAC(t *testing.T) {
	k := known(1, dev("192.168.0.10", "", "", map[string]string{ExtraSSDPUDN: "uuid-5678"}), "")
	escaneado := dev("192.168.0.20", "", "", map[string]string{ExtraSSDPUDN: "uuid-5678"})

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("esperaba match con id=1, got %+v", result[0].Known)
	}
}

func TestReconcile_MismoSSDPUDNPeroMACDistintaNoSeJunta(t *testing.T) {
	// Dos equipos reales con MAC propia; que el UDN coincida (p. ej. un bug del motor) no basta.
	k := known(1, dev("192.168.0.10", "aa:bb:cc:dd:ee:01", "", map[string]string{ExtraSSDPUDN: "uuid-dup"}), "")
	escaneado := dev("192.168.0.20", "11:22:33:44:55:66", "", map[string]string{ExtraSSDPUDN: "uuid-dup"})

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known != nil {
		t.Fatalf("la MAC contradice: no puede ser el mismo equipo aunque el UDN coincida, got %+v", result[0].Known)
	}
}

func TestReconcile_MismoNombreFuertePeroMACDistintaNoSeJunta(t *testing.T) {
	// Dos Echo Show reales, cada uno con su propia MAC: el nombre coincide pero no es el mismo equipo.
	k := known(1, dev("192.168.0.52", "aa:bb:cc:dd:ee:01", "", map[string]string{ExtraMDNSName: "Echo Show"}), "")
	escaneado := dev("192.168.0.53", "11:22:33:44:55:66", "", map[string]string{ExtraMDNSName: "Echo Show"})

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known != nil {
		t.Fatalf("la MAC contradice: no puede ser el mismo Echo Show, got %+v", result[0].Known)
	}
}

func TestReconcile_MismoNombreFuertePeroUDNDistintoNoSeJunta(t *testing.T) {
	k := known(1, dev("192.168.0.52", "", "", map[string]string{ExtraMDNSName: "Echo Show", ExtraSSDPUDN: "uuid-1"}), "")
	escaneado := dev("192.168.0.53", "", "", map[string]string{ExtraMDNSName: "Echo Show", ExtraSSDPUDN: "uuid-2"})

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known != nil {
		t.Fatalf("el UDN contradice aunque el nombre coincida, got %+v", result[0].Known)
	}
}

func TestReconcile_EstacionPorNombreFuerteAunqueCambieIP(t *testing.T) {
	previo := dev("192.168.0.48", "", "Estacion-Estudio", map[string]string{ExtraMDNSName: "Estacion Estudio"})
	k := known(1, previo, "28:cf:e9:12:ab:34")
	// Se fue a otra IP y esta vez ni siquiera trae la MAC resuelta (p. ej. ARP tardó).
	escaneado := dev("192.168.0.50", "", "", map[string]string{ExtraMDNSName: "Estacion Estudio"})

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("esperaba match con id=1, got %+v", result[0].Known)
	}
}

func TestReconcile_AndroidConHostnameAleatorioNoSeJuntaPorNombre(t *testing.T) {
	k := known(1, dev("192.168.0.60", "", "Android_1a2b3c4d", nil), "")
	// Mismo hostname "aleatorio" pero en otra IP y sin MAC: si el nombre contara, se juntarían.
	escaneado := dev("192.168.0.61", "", "Android_1a2b3c4d", nil)

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known != nil {
		t.Fatalf("un hostname aleatorizado por Android no debe reconciliar por nombre, got %+v", result[0].Known)
	}
}

func TestStrongNameOf_BasuraNoCuenta(t *testing.T) {
	cases := []string{"none", "none-2", "localhost", "LocalHost"}
	for _, hostname := range cases {
		if got := StrongNameOf(dev("1.2.3.4", "", hostname, nil)); got != "" {
			t.Errorf("StrongNameOf(hostname=%q) = %q, want \"\"", hostname, got)
		}
	}
}

func TestReconcile_DosEquiposSinNombreNiMACSeDistinguenPorIP(t *testing.T) {
	k1 := known(1, dev("192.168.0.10", "", "", nil), "")
	k2 := known(2, dev("192.168.0.11", "", "", nil), "")
	// El escaneo ve un equipo en cada IP de siempre: sin MAC ni nombre, la única pista es
	// la IP, así que cada escaneado reconcilia con el conocido de esa IP.
	enDiez := dev("192.168.0.10", "", "", nil)
	enOnce := dev("192.168.0.11", "", "", nil)

	result := Reconcile([]KnownDevice{k1, k2}, []core.Device{enDiez, enOnce})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("result[0]: esperaba id=1, got %+v", result[0].Known)
	}
	if result[1].Known == nil || result[1].Known.ID != 2 {
		t.Fatalf("result[1]: esperaba id=2, got %+v", result[1].Known)
	}
}

func TestReconcile_PCConMACPorNetBIOSPorEsaMACAunqueCambieIP(t *testing.T) {
	previo := dev("192.168.0.4", "3c:97:0e:44:51:0a", "DESKTOP-ABC1234", nil)
	k := known(1, previo, "")
	escaneado := dev("192.168.0.5", "3c:97:0e:44:51:0a", "DESKTOP-ABC1234", nil)

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("esperaba match con id=1, got %+v", result[0].Known)
	}
}

func TestReconcile_MACDistintaContradiceYBloqueaMatchPorIP(t *testing.T) {
	k := known(1, dev("192.168.0.10", "aa:bb:cc:dd:ee:01", "", nil), "")
	// Misma IP, pero otra MAC: no puede ser el mismo equipo.
	escaneado := dev("192.168.0.10", "ff:ee:dd:cc:bb:02", "", nil)

	result := Reconcile([]KnownDevice{k}, []core.Device{escaneado})

	if result[0].Known != nil {
		t.Fatalf("esperaba nil, got %+v", result[0].Known)
	}
}

func TestReconcile_SinNingunaPistaEsEquipoNuevo(t *testing.T) {
	result := Reconcile(nil, []core.Device{dev("192.168.0.200", "", "", nil)})

	if result[0].Known != nil {
		t.Fatalf("esperaba nil (equipo nuevo), got %+v", result[0].Known)
	}
}

func TestReconcile_CadaConocidoSeUsaUnaSolaVezPorEscaneo(t *testing.T) {
	k := known(1, dev("192.168.0.10", "", "", nil), "")
	// Dos escaneados reclamando la misma IP (caso límite): solo el primero se queda el conocido.
	result := Reconcile([]KnownDevice{k}, []core.Device{
		dev("192.168.0.10", "", "", nil),
		dev("192.168.0.10", "", "", nil),
	})

	if result[0].Known == nil || result[0].Known.ID != 1 {
		t.Fatalf("result[0]: esperaba id=1, got %+v", result[0].Known)
	}
	if result[1].Known != nil {
		t.Fatalf("result[1]: esperaba nil (ya se usó el conocido), got %+v", result[1].Known)
	}
}
