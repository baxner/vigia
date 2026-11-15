package mobile

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/baxner/vigia/core"
)

func TestParsePorts(t *testing.T) {
	casos := []struct {
		spec    string
		want    []int
		wantErr bool
	}{
		{"22,80,443", []int{22, 80, 443}, false},
		{" 22 , 80 ", []int{22, 80}, false},
		{"8000-8003", []int{8000, 8001, 8002, 8003}, false},
		{"22,22,80", []int{22, 80}, false}, // sin duplicados
		{"0", nil, true},
		{"70000", nil, true},
		{"90-80", nil, true},
		{"abc", nil, true},
		{",", nil, true},
	}
	for _, c := range casos {
		got, err := parsePorts(c.spec)
		if (err != nil) != c.wantErr {
			t.Errorf("parsePorts(%q) err=%v, wantErr=%v", c.spec, err, c.wantErr)
			continue
		}
		if !c.wantErr && !reflect.DeepEqual(got, c.want) {
			t.Errorf("parsePorts(%q) = %v, want %v", c.spec, got, c.want)
		}
	}
	if got, err := parsePorts(""); err != nil || len(got) == 0 {
		t.Errorf("parsePorts(\"\") debe dar los puertos por defecto, got %v err %v", got, err)
	}
}

func TestCancelSinEscaneo(t *testing.T) {
	s := NewScanner()
	s.Cancel() // no debe hacer panic sin escaneo en curso
}

// ===========================================================================
// PortCatalog / RouterInfo / SecurityReport: la frontera JSON de la Fase 5
// (ver docs/agente-api.md, "Un solo motor"). Ida y vuelta: lo que mobile
// devuelve tiene que decodificar de vuelta al mismo tipo de core/.
// ===========================================================================

func TestPortCatalog_JSON(t *testing.T) {
	raw := PortCatalog()
	var entries []core.PortEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("PortCatalog() no decodifica como []core.PortEntry: %v\nJSON: %s", err, raw)
	}
	if len(entries) == 0 {
		t.Fatal("PortCatalog() = [], want el catálogo completo")
	}
	var smb core.PortEntry
	found := false
	for _, e := range entries {
		if e.Port == 445 {
			smb, found = e, true
		}
	}
	if !found || smb.Name != "SMB" || smb.Risk != "medium" {
		t.Errorf("entrada del puerto 445 = %+v (found=%v), want Name=SMB Risk=medium", smb, found)
	}
}

// TestRouterInfo_GatewayInvalida: una gateway mal formada debe devolver un
// error de Go (que gomobile convierte en excepción del lado de Kotlin), no
// la cadena "null" (que es solo para "no contestó").
func TestRouterInfo_GatewayInvalida(t *testing.T) {
	out, err := RouterInfo("no-es-una-ip")
	if err == nil {
		t.Error("err = nil, want un error (gateway inválida)")
	}
	if out != "" {
		t.Errorf("out = %q, want \"\" cuando hay error", out)
	}
}

// TestRouterInfo_SinRespuesta: un router que no contesta (dirección de
// documentación, RFC 5737, nunca asignada) debe devolver "null" sin error,
// y sin colgarse (core.QueryRouter se acota sola a unos segundos).
func TestRouterInfo_SinRespuesta(t *testing.T) {
	start := time.Now()
	out, err := RouterInfo("192.0.2.1")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("err = %v, want nil (no contestar no es un error)", err)
	}
	if out != "null" {
		t.Errorf("out = %q, want %q", out, "null")
	}
	if elapsed > 7*time.Second {
		t.Fatalf("RouterInfo tardó %s; core.QueryRouter debía acotarse sola", elapsed)
	}
}

func TestSecurityReport_RoundTrip(t *testing.T) {
	in := core.SecurityInput{
		Router: &core.RouterInfo{
			IP: "192.168.0.4", UPnP: true, ExternalIP: "192.168.100.2", Reach: "outer_ring",
			PortMappings: []core.PortMapping{},
		},
		Devices: []core.SecurityDevice{
			{Online: true, Name: "DESKTOP-ABC1234", Device: core.Device{IP: "192.168.0.55", OpenPorts: []int{445}}},
		},
	}
	inputJSON, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("no se pudo armar el input de prueba: %v", err)
	}

	out, err := SecurityReport(string(inputJSON))
	if err != nil {
		t.Fatalf("SecurityReport() error = %v", err)
	}

	var report core.SecurityReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("SecurityReport() no decodifica como core.SecurityReport: %v\nJSON: %s", err, out)
	}
	if len(report.Findings) == 0 {
		t.Error("Findings = [], want al menos NO_INBOUND/UPNP_ENABLED/RISKY_SERVICE(445)")
	}
	if report.Router == nil || report.Router.IP != "192.168.0.4" {
		t.Errorf("Router = %+v, want el mismo que mandamos", report.Router)
	}
	if report.Score <= 0 || report.Score >= 100 {
		t.Errorf("Score = %d, want algo entre 0 y 100 (hay hallazgos que restan)", report.Score)
	}
	// GeneratedAt debe ser "ahora" (SecurityReport usa time.Now(), no un
	// reloj fijo: eso solo lo pide la prueba de core.BuildSecurityReport).
	if time.Since(report.GeneratedAt) > time.Minute {
		t.Errorf("GeneratedAt = %v, muy lejos de ahora", report.GeneratedAt)
	}
}

// El JSON de entrada de SecurityReport acepta "acknowledged" (v1.3, ver
// docs/agente-api.md "Hallazgos reconocidos") y lo aplica: el hallazgo
// reconocido no debe restar del puntaje.
func TestSecurityReport_Acknowledged_RoundTrip(t *testing.T) {
	findingID := "RISKY_SERVICE|192.168.0.55|445"
	in := core.SecurityInput{
		Devices: []core.SecurityDevice{
			{Online: true, Name: "DESKTOP-ABC1234", Device: core.Device{IP: "192.168.0.55", OpenPorts: []int{445}}},
		},
		Acknowledged: []core.Acknowledgement{{FindingID: findingID, At: time.Now().UTC()}},
	}
	inputJSON, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("no se pudo armar el input de prueba: %v", err)
	}

	out, err := SecurityReport(string(inputJSON))
	if err != nil {
		t.Fatalf("SecurityReport() error = %v", err)
	}
	var report core.SecurityReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("SecurityReport() no decodifica como core.SecurityReport: %v\nJSON: %s", err, out)
	}
	if report.Score != 100 {
		t.Errorf("Score = %d, want 100 (el único hallazgo está reconocido, no debe restar)", report.Score)
	}
	var found bool
	for _, f := range report.Findings {
		if f.ID == findingID {
			found = true
			if !f.Acknowledged {
				t.Errorf("Finding %q.Acknowledged = false, want true", f.ID)
			}
		}
	}
	if !found {
		t.Fatalf("no se encontró el hallazgo %q en %+v", findingID, report.Findings)
	}
}

func TestSecurityReport_JSONInvalido(t *testing.T) {
	out, err := SecurityReport("esto no es json")
	if err == nil {
		t.Error("err = nil, want un error de JSON inválido")
	}
	if out != "" {
		t.Errorf("out = %q, want \"\"", out)
	}
}

func TestSecurityReport_EntradaVacia(t *testing.T) {
	// Un input "{}" (sin router, sin dispositivos, sin wifi) es válido: debe
	// dar un informe sin hallazgos, no un error.
	out, err := SecurityReport("{}")
	if err != nil {
		t.Fatalf("SecurityReport(\"{}\") error = %v", err)
	}
	if !strings.Contains(out, `"findings":[]`) {
		t.Errorf("out = %s, want findings vacío (\"[]\", no null)", out)
	}
	if !strings.Contains(out, `"score":100`) {
		t.Errorf("out = %s, want score 100 (nada que reste)", out)
	}
}
