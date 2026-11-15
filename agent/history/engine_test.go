package history

import (
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internal/ptrs"
	"github.com/baxner/vigia/core"
)

// Puerto 1:1 de DeviceHistoryEngineTest.kt: fusión de estado + eventos
// (incluida la regla de gracia de WENT_OFFLINE), pura.

var engineT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func engineDevice(ip string) core.Device {
	return core.Device{IP: ip, MAC: "aa:bb:cc:dd:ee:01"}
}

func knownOnline(ip string, at time.Time) KnownDevice {
	return KnownDevice{
		ID:          1,
		Network:     NetworkKey("Casa|192.168.0.0/24"),
		MAC:         ptrs.To("aa:bb:cc:dd:ee:01"),
		LastIP:      ip,
		FirstSeen:   at,
		LastSeen:    at,
		Online:      true,
		OnlineSince: ptrs.To(at),
		Last:        engineDevice(ip),
	}
}

// --- Regla de gracia ---

func TestMissing_UnaVezSinPasarUmbral_SinEventoConservaOnlineSince(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	outcome := Missing(previo, 0, engineT0.Add(60*time.Second))

	if len(outcome.Events) != 0 {
		t.Fatalf("esperaba sin eventos, got %+v", outcome.Events)
	}
	if outcome.OfflineMisses != 1 {
		t.Fatalf("offlineMisses = %d, want 1", outcome.OfflineMisses)
	}
	if outcome.Device.Online {
		t.Fatal("esperaba online=false")
	}
	if outcome.Device.OnlineSince == nil || !outcome.Device.OnlineSince.Equal(engineT0) {
		t.Fatalf("onlineSince = %v, want %v", outcome.Device.OnlineSince, engineT0)
	}
}

func TestMissing_FaltarDosEscaneosSeguidos_CompromoteWentOffline(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	falta1 := Missing(previo, 0, engineT0.Add(60*time.Second))
	falta2 := Missing(falta1.Device, falta1.OfflineMisses, engineT0.Add(120*time.Second))

	if len(falta2.Events) != 1 || falta2.Events[0].Type != WentOffline {
		t.Fatalf("eventos = %+v, want [WENT_OFFLINE]", falta2.Events)
	}
	if falta2.OfflineMisses != 2 {
		t.Fatalf("offlineMisses = %d, want 2", falta2.OfflineMisses)
	}
	if falta2.Device.Online {
		t.Fatal("esperaba online=false")
	}
	if falta2.Device.OnlineSince != nil {
		t.Fatalf("onlineSince = %v, want nil", falta2.Device.OnlineSince)
	}
	// Se fecha con el último lastSeen real (t0), no con el momento en que recién se
	// confirma (t0+120s): es cuando de verdad se fue, no cuando nos enteramos.
	if !falta2.Events[0].At.Equal(engineT0) {
		t.Fatalf("at = %v, want %v", falta2.Events[0].At, engineT0)
	}
}

func TestMissing_MasDeDiezMinutos_CompromoteWentOfflineAunqueSeaPrimeraFalta(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	outcome := Missing(previo, 0, engineT0.Add(11*time.Minute))

	if outcome.OfflineMisses != 1 {
		t.Fatalf("offlineMisses = %d, want 1", outcome.OfflineMisses)
	}
	if len(outcome.Events) != 1 || outcome.Events[0].Type != WentOffline {
		t.Fatalf("eventos = %+v, want [WENT_OFFLINE]", outcome.Events)
	}
	if outcome.Device.OnlineSince != nil {
		t.Fatalf("onlineSince = %v, want nil", outcome.Device.OnlineSince)
	}
	if !outcome.Events[0].At.Equal(engineT0) {
		t.Fatalf("at = %v, want %v", outcome.Events[0].At, engineT0)
	}
}

func TestMissing_NoRepiteWentOfflineYaComprometido(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	falta1 := Missing(previo, 0, engineT0.Add(60*time.Second))
	falta2 := Missing(falta1.Device, falta1.OfflineMisses, engineT0.Add(120*time.Second))
	falta3 := Missing(falta2.Device, falta2.OfflineMisses, engineT0.Add(180*time.Second))

	if len(falta3.Events) != 0 {
		t.Fatalf("esperaba sin eventos, got %+v", falta3.Events)
	}
	if falta3.OfflineMisses != 3 {
		t.Fatalf("offlineMisses = %d, want 3", falta3.OfflineMisses)
	}
	if falta3.Device.OnlineSince != nil {
		t.Fatalf("onlineSince = %v, want nil", falta3.Device.OnlineSince)
	}
}

func TestSeen_ReapareceAntesDeLaGracia_SinEventoConservaOnlineSince(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	falta := Missing(previo, 0, engineT0.Add(60*time.Second))

	vuelve := Seen(falta.Device, engineDevice("192.168.0.10"), engineT0.Add(90*time.Second))

	if len(vuelve.Events) != 0 {
		t.Fatalf("no debe haber ni WENT_OFFLINE ni CAME_ONLINE, got %+v", vuelve.Events)
	}
	if !vuelve.Device.Online {
		t.Fatal("esperaba online=true")
	}
	if vuelve.Device.OnlineSince == nil || !vuelve.Device.OnlineSince.Equal(engineT0) {
		t.Fatalf("onlineSince = %v, want %v", vuelve.Device.OnlineSince, engineT0)
	}
	if vuelve.OfflineMisses != 0 {
		t.Fatalf("offlineMisses = %d, want 0", vuelve.OfflineMisses)
	}
}

func TestSeen_TrasWentOfflineComprometido_DisparaCameOnlineConOnlineSinceAhora(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	falta1 := Missing(previo, 0, engineT0.Add(60*time.Second))
	falta2 := Missing(falta1.Device, falta1.OfflineMisses, engineT0.Add(120*time.Second))

	ahora := engineT0.Add(500 * time.Second)
	vuelve := Seen(falta2.Device, engineDevice("192.168.0.10"), ahora)

	if len(vuelve.Events) != 1 || vuelve.Events[0].Type != CameOnline {
		t.Fatalf("eventos = %+v, want [CAME_ONLINE]", vuelve.Events)
	}
	if !vuelve.Device.Online {
		t.Fatal("esperaba online=true")
	}
	if vuelve.Device.OnlineSince == nil || !vuelve.Device.OnlineSince.Equal(ahora) {
		t.Fatalf("onlineSince = %v, want %v", vuelve.Device.OnlineSince, ahora)
	}
	if vuelve.OfflineMisses != 0 {
		t.Fatalf("offlineMisses = %d, want 0", vuelve.OfflineMisses)
	}
}

// --- IP_CHANGED ---

func TestSeen_IPDistintaEnEquipoQueSeguiaConectado_GeneraIPChanged(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	outcome := Seen(previo, engineDevice("192.168.0.99"), engineT0.Add(30*time.Second))

	if len(outcome.Events) != 1 {
		t.Fatalf("eventos = %+v, want 1 evento", outcome.Events)
	}
	evento := outcome.Events[0]
	if evento.Type != IPChanged {
		t.Fatalf("type = %v, want IP_CHANGED", evento.Type)
	}
	if evento.Detail == nil || *evento.Detail != "192.168.0.10 → 192.168.0.99" {
		t.Fatalf("detail = %v, want %q", evento.Detail, "192.168.0.10 → 192.168.0.99")
	}
	if outcome.Device.LastIP != "192.168.0.99" {
		t.Fatalf("lastIp = %q, want 192.168.0.99", outcome.Device.LastIP)
	}
	// Sigue conectado sin interrupción: la IP no toca onlineSince.
	if outcome.Device.OnlineSince == nil || !outcome.Device.OnlineSince.Equal(engineT0) {
		t.Fatalf("onlineSince = %v, want %v", outcome.Device.OnlineSince, engineT0)
	}
}

func TestSeen_MismaIPNoGeneraIPChanged(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	outcome := Seen(previo, engineDevice("192.168.0.10"), engineT0.Add(30*time.Second))

	if len(outcome.Events) != 0 {
		t.Fatalf("esperaba sin eventos, got %+v", outcome.Events)
	}
}

func TestSeen_ReaparecerConIPDistinta_GeneraCameOnlineEIPChangedEnEseOrden(t *testing.T) {
	previo := knownOnline("192.168.0.10", engineT0)
	falta1 := Missing(previo, 0, engineT0.Add(60*time.Second))
	falta2 := Missing(falta1.Device, falta1.OfflineMisses, engineT0.Add(120*time.Second))

	vuelve := Seen(falta2.Device, engineDevice("192.168.0.77"), engineT0.Add(500*time.Second))

	if len(vuelve.Events) != 2 || vuelve.Events[0].Type != CameOnline || vuelve.Events[1].Type != IPChanged {
		t.Fatalf("eventos = %+v, want [CAME_ONLINE, IP_CHANGED]", vuelve.Events)
	}
}
