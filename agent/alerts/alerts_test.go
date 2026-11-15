package alerts

import (
	"testing"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/core"
)

const testNetwork = "192.168.0.1|192.168.0.0/24"

func mkDevice(id int64, ip string, important bool) history.KnownDevice {
	return history.KnownDevice{ID: id, LastIP: ip, Custom: history.DeviceCustomization{Important: important}}
}

func TestEvaluateScan_NewDevice_DespuesDeLineaBase(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := t0.Add(16 * time.Minute) // > 15 min de margen
	dev := mkDevice(31, "192.168.0.24", false)
	dev.Last.Identity.Brand = "OnePlus"
	events := []EventInput{{Event: history.DeviceEvent{DeviceID: 31, Type: history.FirstSeen, At: at}, Device: dev}}

	got := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	a := got[0]
	if a.Type != NewDevice || a.Severity != High {
		t.Fatalf("type/severity = %s/%s, want NEW_DEVICE/high", a.Type, a.Severity)
	}
	if a.Title != "Equipo nuevo en tu red" {
		t.Fatalf("title = %q", a.Title)
	}
	if want := "OnePlus (192.168.0.24) se conectó por primera vez."; a.Body != want {
		t.Fatalf("body = %q, want %q", a.Body, want)
	}
	if a.DeviceID == nil || *a.DeviceID != 31 {
		t.Fatalf("deviceId = %v, want 31", a.DeviceID)
	}
	if a.Network != testNetwork {
		t.Fatalf("network = %q, want %q", a.Network, testNetwork)
	}
	if a.DedupKey == "" {
		t.Fatal("DedupKey vacío")
	}
}

func TestEvaluateScan_NewDevice_DentroDeLineaBase_NoAlerta(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	for _, delta := range []time.Duration{0, 5 * time.Minute, 15 * time.Minute} { // <= 15 min: línea base
		at := t0.Add(delta)
		events := []EventInput{{Event: history.DeviceEvent{DeviceID: 1, Type: history.FirstSeen, At: at}, Device: mkDevice(1, "192.168.0.2", false)}}
		if got := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner); len(got) != 0 {
			t.Fatalf("delta=%v: got %d alertas, want 0 (línea base)", delta, len(got))
		}
	}
}

func TestEvaluateScan_NewDevice_BaselineDesconocida_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	events := []EventInput{{Event: history.DeviceEvent{DeviceID: 1, Type: history.FirstSeen, At: at}, Device: mkDevice(1, "192.168.0.2", false)}}
	if got := EvaluateScan(events, testNetwork, time.Time{}, DefaultSettings(), RingInner); len(got) != 0 {
		t.Fatalf("got %d alertas, want 0 (baseline desconocida: no se arriesga una tormenta de alertas)", len(got))
	}
}

func TestEvaluateScan_NewDevice_Apagado_NoAlerta(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := t0.Add(time.Hour)
	events := []EventInput{{Event: history.DeviceEvent{DeviceID: 1, Type: history.FirstSeen, At: at}, Device: mkDevice(1, "192.168.0.2", false)}}
	settings := DefaultSettings()
	settings.NewDevice = false
	if got := EvaluateScan(events, testNetwork, t0, settings, RingInner); len(got) != 0 {
		t.Fatalf("got %d alertas, want 0 (NEW_DEVICE apagado)", len(got))
	}
}

// IMPORTANT_OFFLINE/IMPORTANT_ONLINE ya NO salen de EvaluateScan desde la
// Fase 5.1 (ver TestEvaluateImportantOffline_* / TestEvaluateImportantOnline_*
// más abajo): WENT_OFFLINE/CAME_ONLINE siguen sin estar en la tabla de
// reglas de EvaluateScan (igual que IP_CHANGED).
func TestEvaluateScan_WentOfflineCameOnlineIPChanged_NuncaGeneranAlerta(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	important := mkDevice(4, "192.168.0.7", true)
	detail := "192.168.0.6 → 192.168.0.7"
	events := []EventInput{
		{Event: history.DeviceEvent{DeviceID: 2, Type: history.WentOffline, At: t0}, Device: important},
		{Event: history.DeviceEvent{DeviceID: 3, Type: history.CameOnline, At: t0}, Device: important},
		{Event: history.DeviceEvent{DeviceID: 4, Type: history.IPChanged, At: t0, Detail: &detail}, Device: important},
	}
	if got := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner); len(got) != 0 {
		t.Fatalf("got %d alertas, want 0 (ninguno de los tres está en la tabla de reglas de EvaluateScan)", len(got))
	}
}

// --- IMPORTANT_OFFLINE (Fase 5.1) ---

func TestEvaluateImportantOffline_SoloSiImportantYOffline(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	now := t0.Add(20 * time.Minute)

	notImportant := mkDevice(2, "192.168.0.5", false)
	notImportant.LastSeen = t0
	if got := EvaluateImportantOffline([]history.KnownDevice{notImportant}, now, testNetwork, DefaultSettings()); len(got) != 0 {
		t.Fatalf("equipo no importante: got %d alertas, want 0", len(got))
	}

	important := mkDevice(2, "192.168.0.5", true)
	important.Custom.Name = strPtr("Cámara jardín")
	important.LastSeen = t0
	important.Online = false
	got := EvaluateImportantOffline([]history.KnownDevice{important}, now, testNetwork, DefaultSettings())
	if len(got) != 1 {
		t.Fatalf("equipo importante offline 20 min: got %d alertas, want 1", len(got))
	}
	a := got[0]
	if a.Type != ImportantOffline || a.Severity != Medium {
		t.Fatalf("type/severity = %s/%s, want IMPORTANT_OFFLINE/medium", a.Type, a.Severity)
	}
	if want := "Cámara jardín (192.168.0.5) lleva más de 15 minutos sin conexión."; a.Body != want {
		t.Fatalf("body = %q, want %q", a.Body, want)
	}
	if !a.At.Equal(t0) {
		t.Fatalf("At = %v, want LastSeen (%v), no el momento en que se nota", a.At, t0)
	}

	// Equipo importante pero AÚN en línea: no avisa.
	online := important
	online.Online = true
	if got := EvaluateImportantOffline([]history.KnownDevice{online}, now, testNetwork, DefaultSettings()); len(got) != 0 {
		t.Fatalf("equipo en línea: got %d alertas, want 0", len(got))
	}
}

// Caso real: el Wi-Fi se duerme de madrugada y vuelve
// a los 5 minutos — eso NO debe avisar (el umbral es 15 min).
func TestEvaluateImportantOffline_MenosDe15Min_NoAvisa(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	now := t0.Add(5 * time.Minute)
	d := mkDevice(1, "192.168.0.24", true)
	d.LastSeen, d.Online = t0, false
	if got := EvaluateImportantOffline([]history.KnownDevice{d}, now, testNetwork, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got %d alertas, want 0 (5 min < 15 min)", len(got))
	}
}

func TestEvaluateImportantOffline_Apagado_NoAvisa(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	now := t0.Add(20 * time.Minute)
	d := mkDevice(1, "192.168.0.24", true)
	d.LastSeen, d.Online = t0, false
	settings := DefaultSettings()
	settings.ImportantOffline = false
	if got := EvaluateImportantOffline([]history.KnownDevice{d}, now, testNetwork, settings); len(got) != 0 {
		t.Fatalf("got %d alertas, want 0 (IMPORTANT_OFFLINE apagado)", len(got))
	}
}

// Dedup por episodio: mientras LastSeen no cambie, la misma evaluación
// (en escaneos sucesivos) da la MISMA DedupKey — es agent/store quien de
// verdad evita repetir el aviso (restricción UNIQUE), no esta función.
func TestEvaluateImportantOffline_MismoEpisodio_MismaDedupKey(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d := mkDevice(1, "192.168.0.24", true)
	d.LastSeen, d.Online = t0, false

	a1 := EvaluateImportantOffline([]history.KnownDevice{d}, t0.Add(20*time.Minute), testNetwork, DefaultSettings())
	a2 := EvaluateImportantOffline([]history.KnownDevice{d}, t0.Add(40*time.Minute), testNetwork, DefaultSettings())
	if len(a1) != 1 || len(a2) != 1 {
		t.Fatalf("len = %d/%d, want 1/1", len(a1), len(a2))
	}
	if a1[0].DedupKey != a2[0].DedupKey {
		t.Fatalf("DedupKey cambió entre escaneos del mismo episodio: %q != %q", a1[0].DedupKey, a2[0].DedupKey)
	}
}

// --- IMPORTANT_ONLINE (Fase 5.1) ---

func TestEvaluateImportantOnline_SoloSiPendiente(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	important := mkDevice(3, "192.168.0.6", true)
	important.Custom.Name = strPtr("NAS")
	important.Online = true

	// Sin pendiente (nunca se avisó la caída, o ya se resolvió): nada.
	out, resolved := EvaluateImportantOnline([]history.KnownDevice{important}, nil, t0, testNetwork, DefaultSettings())
	if len(out) != 0 || len(resolved) != 0 {
		t.Fatalf("sin pendiente: out=%+v resolved=%v, want ambos vacíos", out, resolved)
	}

	// Con pendiente: avisa y lo marca resuelto.
	pending := map[int64]bool{3: true}
	out2, resolved2 := EvaluateImportantOnline([]history.KnownDevice{important}, pending, t0, testNetwork, DefaultSettings())
	if len(out2) != 1 || len(resolved2) != 1 || resolved2[0] != 3 {
		t.Fatalf("out=%+v resolved=%v, want 1 alerta y resolved=[3]", out2, resolved2)
	}
	a := out2[0]
	if a.Type != ImportantOnline || a.Severity != Low {
		t.Fatalf("type/severity = %s/%s, want IMPORTANT_ONLINE/low", a.Type, a.Severity)
	}
	if want := "NAS (192.168.0.6) volvió a estar en línea."; a.Body != want {
		t.Fatalf("body = %q, want %q", a.Body, want)
	}

	// Pendiente pero TODAVÍA offline: no avisa ni resuelve.
	offline := important
	offline.Online = false
	out3, resolved3 := EvaluateImportantOnline([]history.KnownDevice{offline}, pending, t0, testNetwork, DefaultSettings())
	if len(out3) != 0 || len(resolved3) != 0 {
		t.Fatalf("todavía offline: out=%+v resolved=%v, want ambos vacíos", out3, resolved3)
	}
}

// El ajuste apagado silencia la ALERTA, no el estado: igual se resuelve el
// pendiente (mismo criterio que RISKY_SERVICE con su memoria permanente).
func TestEvaluateImportantOnline_Apagado_NoAlertaPeroSiResuelve(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	important := mkDevice(3, "192.168.0.6", true)
	important.Online = true
	pending := map[int64]bool{3: true}
	settings := DefaultSettings()
	settings.ImportantOnline = false

	out, resolved := EvaluateImportantOnline([]history.KnownDevice{important}, pending, t0, testNetwork, settings)
	if len(out) != 0 {
		t.Fatalf("out = %+v, want 0 (IMPORTANT_ONLINE apagado)", out)
	}
	if len(resolved) != 1 || resolved[0] != 3 {
		t.Fatalf("resolved = %v, want [3] (se resuelve igual)", resolved)
	}
}

// Idempotencia: evaluar el MISMO evento dos veces (p. ej. tras un reinicio
// a mitad de un escaneo) produce la MISMA DedupKey las dos veces — la
// restricción UNIQUE de agent/store sobre esa columna es lo que de verdad
// evita la alerta duplicada, pero solo funciona si la clave es estable
// para el mismo evento.
func TestEvaluateScan_Idempotente_MismoEventoMismaDedupKey(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := t0.Add(time.Hour)
	events := []EventInput{{Event: history.DeviceEvent{DeviceID: 9, Type: history.FirstSeen, At: at}, Device: mkDevice(9, "192.168.0.9", false)}}

	got1 := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner)
	got2 := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner)
	if len(got1) != 1 || len(got2) != 1 {
		t.Fatalf("len = %d/%d, want 1/1", len(got1), len(got2))
	}
	if got1[0].DedupKey != got2[0].DedupKey {
		t.Fatalf("DedupKey cambió entre evaluaciones: %q != %q", got1[0].DedupKey, got2[0].DedupKey)
	}
}

func TestEvaluateScan_DedupKeyDistintoPorEquipoYPorInstante(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := t0.Add(time.Hour)
	evA := EventInput{Event: history.DeviceEvent{DeviceID: 10, Type: history.FirstSeen, At: at}, Device: mkDevice(10, "192.168.0.10", false)}
	evB := EventInput{Event: history.DeviceEvent{DeviceID: 11, Type: history.FirstSeen, At: at}, Device: mkDevice(11, "192.168.0.11", false)}
	evC := EventInput{Event: history.DeviceEvent{DeviceID: 10, Type: history.FirstSeen, At: at.Add(time.Minute)}, Device: mkDevice(10, "192.168.0.10", false)}

	got := EvaluateScan([]EventInput{evA, evB, evC}, testNetwork, t0, DefaultSettings(), RingInner)
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].DedupKey == got[1].DedupKey || got[0].DedupKey == got[2].DedupKey || got[1].DedupKey == got[2].DedupKey {
		t.Fatalf("DedupKeys deberían ser todas distintas: %+v", got)
	}
}

func TestEvaluateHealth_PrimeraVezCiego_GeneraAlertaYTransicion(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	transitioned, alert := EvaluateHealth(false, true, "sin permiso de red local", testNetwork, at, DefaultSettings())
	if !transitioned {
		t.Fatal("transitioned = false, want true")
	}
	if alert == nil {
		t.Fatal("alert = nil, want AGENT_BLIND")
	}
	if alert.Type != AgentBlind || alert.Severity != Medium {
		t.Fatalf("type/severity = %s/%s, want AGENT_BLIND/medium", alert.Type, alert.Severity)
	}
	if alert.Body != "sin permiso de red local" {
		t.Fatalf("body = %q, want el warning reutilizado tal cual", alert.Body)
	}
	if alert.DeviceID != nil {
		t.Fatalf("deviceId = %v, want nil (AGENT_BLIND)", alert.DeviceID)
	}
}

func TestEvaluateHealth_SigueCiego_NoRepiteAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	transitioned, alert := EvaluateHealth(true, true, "sin permiso", testNetwork, at, DefaultSettings())
	if transitioned {
		t.Fatal("transitioned = true, want false (ya estaba ciego)")
	}
	if alert != nil {
		t.Fatalf("alert = %+v, want nil: una sola alerta hasta AGENT_RECOVERED", alert)
	}
}

func TestEvaluateHealth_Recupera_GeneraAgentRecovered(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	transitioned, alert := EvaluateHealth(true, false, "", testNetwork, at, DefaultSettings())
	if !transitioned {
		t.Fatal("transitioned = false, want true")
	}
	if alert == nil || alert.Type != AgentRecovered || alert.Severity != Low {
		t.Fatalf("alert = %+v, want AGENT_RECOVERED/low", alert)
	}
}

func TestEvaluateHealth_SigueNormal_SinAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	transitioned, alert := EvaluateHealth(false, false, "", testNetwork, at, DefaultSettings())
	if transitioned || alert != nil {
		t.Fatalf("transitioned=%v alert=%+v, want false/nil", transitioned, alert)
	}
}

// El estado persistido debe actualizarse SIEMPRE que hay transición, aunque
// el ajuste correspondiente esté apagado — si no, el día que se reactive
// quedaría una alerta pendiente de más (o de menos).
func TestEvaluateHealth_SettingsApagado_TransicionaPeroSinAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	settings.AgentBlind = false

	transitioned, alert := EvaluateHealth(false, true, "sin permiso", testNetwork, at, settings)
	if !transitioned {
		t.Fatal("transitioned = false, want true (el estado se guarda igual)")
	}
	if alert != nil {
		t.Fatalf("alert = %+v, want nil (AGENT_BLIND apagado)", alert)
	}
}

func TestEvaluateHealth_DedupKeyEstableParaLaMismaTransicion(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	_, a1 := EvaluateHealth(false, true, "x", testNetwork, at, DefaultSettings())
	_, a2 := EvaluateHealth(false, true, "x", testNetwork, at, DefaultSettings())
	if a1.DedupKey != a2.DedupKey {
		t.Fatalf("DedupKey cambió: %q != %q", a1.DedupKey, a2.DedupKey)
	}
}

func TestBaselineOf(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []history.KnownDevice{
		{FirstSeen: t0.Add(99 * time.Second)},
		{FirstSeen: t0},
		{FirstSeen: t0.Add(time.Hour)},
	}
	if got := BaselineOf(devices); !got.Equal(t0) {
		t.Fatalf("BaselineOf = %v, want %v", got, t0)
	}
	if got := BaselineOf(nil); !got.IsZero() {
		t.Fatalf("BaselineOf(nil) = %v, want zero", got)
	}
}

func TestLabel_OrdenDeFuentes(t *testing.T) {
	custom := "Laptop Estudio"
	tests := []struct {
		name string
		dev  history.KnownDevice
		want string
	}{
		{
			name: "custom.name gana a todo",
			dev:  history.KnownDevice{LastIP: "192.168.0.2", Custom: history.DeviceCustomization{Name: &custom}, Last: deviceWith("mdns", "host", "Marca", "Modelo")},
			want: "Laptop Estudio (192.168.0.2)",
		},
		{
			name: "mdns:name gana a hostname",
			dev:  history.KnownDevice{LastIP: "192.168.0.3", Last: deviceWith("Estacion Estudio", "otronombre", "", "")},
			want: "Estacion Estudio (192.168.0.3)",
		},
		{
			name: "hostname cuando no hay mdns",
			dev:  history.KnownDevice{LastIP: "192.168.0.4", Last: deviceWith("", "mi-laptop", "", "")},
			want: "mi-laptop (192.168.0.4)",
		},
		{
			name: "hostname basura se descarta, cae a marca+modelo",
			dev:  history.KnownDevice{LastIP: "192.168.0.5", Last: deviceWith("", "localhost", "Tenda", "AX6")},
			want: "Tenda AX6 (192.168.0.5)",
		},
		{
			name: "hostname aleatorio de Android se descarta",
			dev:  history.KnownDevice{LastIP: "192.168.0.6", Last: deviceWith("", "Android_1a2b3c4d", "Samsung", "")},
			want: "Samsung (192.168.0.6)",
		},
		{
			name: "marca y modelo",
			dev:  history.KnownDevice{LastIP: "192.168.0.7", Last: deviceWith("", "", "Hikvision", "DS-2CD")},
			want: "Hikvision DS-2CD (192.168.0.7)",
		},
		{
			name: "solo marca",
			dev:  history.KnownDevice{LastIP: "192.168.0.8", Last: deviceWith("", "", "Hikvision", "")},
			want: "Hikvision (192.168.0.8)",
		},
		{
			name: "sin nada: solo la IP, sin paréntesis duplicados",
			dev:  history.KnownDevice{LastIP: "192.168.0.9", Last: deviceWith("", "", "", "")},
			want: "192.168.0.9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := label(tt.dev); got != tt.want {
				t.Fatalf("label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func deviceWith(mdnsName, hostname, brand, model string) (d core.Device) {
	d.Hostname = hostname
	d.Identity.Brand = brand
	d.Identity.Model = model
	if mdnsName != "" {
		d.Extra = map[string]string{history.ExtraMDNSName: mdnsName}
	}
	return d
}

func strPtr(s string) *string { return &s }
