package core

import (
	"net"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// ===========================================================================
// Paquetes mDNS sintéticos (golang.org/x/net/dns/dnsmessage) que reproducen
// la estructura exacta de respuestas unicast/multicast reales: el TXT huérfano
// de "_device-info._tcp", el nombre de instancia con comillas/espacio y el
// sufijo "-N" de conflicto de nombre.
// ===========================================================================

func buildRFBCapture(t *testing.T) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	mustNoErr(t, b.StartQuestions())
	mustNoErr(t, b.StartAnswers())
	mustNoErr(t, b.PTRResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "_rfb._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.PTRResource{PTR: mustName(t, "iMac de Estudio._rfb._tcp.local.")},
	))
	mustNoErr(t, b.StartAdditionals())
	mustNoErr(t, b.SRVResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "iMac de Estudio._rfb._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.SRVResource{Target: mustName(t, "iMac-Estudio.local."), Port: 5900},
	))
	mustNoErr(t, b.TXTResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "iMac de Estudio._rfb._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.TXTResource{TXT: []string{""}},
	))
	mustNoErr(t, b.TXTResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "iMac de Estudio._device-info._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.TXTResource{TXT: []string{"model=iMac18,3", "osxvers=24"}},
	))
	mustNoErr(t, b.AAAAResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "iMac-Estudio.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.AAAAResource{AAAA: [16]byte{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
	))
	mustNoErr(t, b.AResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "iMac-Estudio.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.AResource{A: [4]byte{192, 168, 0, 48}},
	))
	data, err := b.Finish()
	mustNoErr(t, err)
	return data
}

func buildAirplayCapture(t *testing.T) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	mustNoErr(t, b.StartQuestions())
	mustNoErr(t, b.StartAnswers())
	mustNoErr(t, b.PTRResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "_airplay._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.PTRResource{PTR: mustName(t, `75" QLED._airplay._tcp.local.`)},
	))
	mustNoErr(t, b.StartAdditionals())
	mustNoErr(t, b.SRVResource(
		dnsmessage.ResourceHeader{Name: mustName(t, `75" QLED._airplay._tcp.local.`), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.SRVResource{Target: mustName(t, "Samsung.local."), Port: 7000},
	))
	mustNoErr(t, b.TXTResource(
		dnsmessage.ResourceHeader{Name: mustName(t, `75" QLED._airplay._tcp.local.`), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.TXTResource{TXT: []string{
			"acl=0",
			"deviceid=02:00:00:00:01:18",
			"model=QCQ70D",
			"integrator=Samsung",
			"manufacturer=Samsung",
			"serialNumber=SYNTH0000001",
		}},
	))
	mustNoErr(t, b.AResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "Samsung.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.AResource{A: [4]byte{192, 168, 0, 118}},
	))
	data, err := b.Finish()
	mustNoErr(t, err)
	return data
}

func buildTeamViewerCapture(t *testing.T) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	mustNoErr(t, b.StartQuestions())
	mustNoErr(t, b.StartAnswers())
	mustNoErr(t, b.PTRResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "_teamviewer._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.PTRResource{PTR: mustName(t, "0._teamviewer._tcp.local.")},
	))
	mustNoErr(t, b.StartAdditionals())
	mustNoErr(t, b.SRVResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "0._teamviewer._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.SRVResource{Target: mustName(t, "DESKTOP-ABC1234-2.local."), Port: 2020},
	))
	mustNoErr(t, b.TXTResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "0._teamviewer._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.TXTResource{TXT: []string{
			"DyngateID=100000001",
			"Token=SynthToken000000",
			"UUID=00000000-0000-4000-8000-000000000001",
		}},
	))
	mustNoErr(t, b.AResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "DESKTOP-ABC1234-2.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.AResource{A: [4]byte{192, 168, 0, 55}},
	))
	data, err := b.Finish()
	mustNoErr(t, err)
	return data
}

// decodeCapture procesa un paquete DNS-SD en un accumulator nuevo,
// exactamente como runMDNSListener hace con cada datagrama que llega.
func decodeCapture(t *testing.T, data []byte) *mdnsAccumulator {
	t.Helper()
	acc := newMDNSAccumulator()
	conn, dest := loopbackSink(t)
	acc.handlePacket(data, conn, dest)
	return acc
}

// loopbackSink da un conn/dest inofensivos para pasarle a handlePacket: los
// usa internamente solo si una instancia trae SRV pero su A/AAAA todavía no
// se vio en el mismo paquete (ver addSRV) — en ese caso repregunta, y en el
// test no queremos que le pregunte de verdad a la red.
func loopbackSink(t *testing.T) (*net.UDPConn, *net.UDPAddr) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("no se pudo abrir el socket UDP del test: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5353}
}

// TestMDNSRealCapture_RFBDeviceInfo reproduce el hallazgo central de
// identificacion-v2: _device-info._tcp nunca tiene su propio PTR/SRV, pero
// macOS cuela su TXT (model=/osxvers=) como adicional de _rfb. El resultado
// debe traer Model, OSVersion e Instances igual, y Services con AMBOS tipos
// (_rfb y _device-info) aunque el segundo nunca resolvió por su cuenta.
func TestMDNSRealCapture_RFBDeviceInfo(t *testing.T) {
	acc := decodeCapture(t, buildRFBCapture(t))
	out := acc.result()

	info, ok := out["192.168.0.48"]
	if !ok {
		t.Fatalf("192.168.0.48 no está en el resultado: %+v", out)
	}
	if info.Hostname != "iMac-Estudio" {
		t.Errorf("Hostname = %q, want %q", info.Hostname, "iMac-Estudio")
	}
	if info.Model != "iMac18,3" {
		t.Errorf("Model = %q, want %q (TXT huérfano de _device-info)", info.Model, "iMac18,3")
	}
	if info.OSVersion != "24" {
		t.Errorf("OSVersion = %q, want %q", info.OSVersion, "24")
	}
	wantServices := map[string]bool{"_rfb._tcp": false, "_device-info._tcp": false}
	for _, s := range info.Services {
		if _, ok := wantServices[s]; ok {
			wantServices[s] = true
		}
	}
	for svc, seen := range wantServices {
		if !seen {
			t.Errorf("Services = %v, falta %q", info.Services, svc)
		}
	}
	found := false
	for _, inst := range info.Instances {
		if inst == "iMac de Estudio" {
			found = true
		}
	}
	if !found {
		t.Errorf("Instances = %v, falta %q", info.Instances, "iMac de Estudio")
	}
}

// TestMDNSRealCapture_AirplaySamsung cubre el nombre de instancia con
// comillas y espacio (`75" QLED`, sin escapes: Name.String() no escapa nada,
// ver el comentario de instanceTypeRe) y manufacturer/model por TXT de
// _airplay, sin SSDP de por medio.
func TestMDNSRealCapture_AirplaySamsung(t *testing.T) {
	acc := decodeCapture(t, buildAirplayCapture(t))
	out := acc.result()

	info, ok := out["192.168.0.118"]
	if !ok {
		t.Fatalf("192.168.0.118 no está en el resultado: %+v", out)
	}
	if info.Hostname != "Samsung" {
		t.Errorf("Hostname = %q, want %q", info.Hostname, "Samsung")
	}
	if info.Model != "QCQ70D" {
		t.Errorf("Model = %q, want %q", info.Model, "QCQ70D")
	}
	if info.Manufacturer != "Samsung" {
		t.Errorf("Manufacturer = %q, want %q", info.Manufacturer, "Samsung")
	}
	wantInstance := `75" QLED`
	found := false
	for _, inst := range info.Instances {
		if inst == wantInstance {
			found = true
		}
		if strings.Contains(inst, `\`) {
			t.Errorf("Instances = %v: trae una barra invertida — Name.String() no escapa, no debería haber ninguna", info.Instances)
		}
	}
	if !found {
		t.Errorf("Instances = %v, falta %q", info.Instances, wantInstance)
	}
}

// TestMDNSRealCapture_TeamViewerConflictSuffix: el hostname mDNS crudo del PC
// (.55) trae el sufijo de conflicto "-2" — discover_extra.go/result() lo
// reporta tal cual (la limpieza "-N" vs. NetBIOS es responsabilidad de
// scan.go/enrich, no de aquí). El "0" de la instancia TeamViewer es un
// identificador interno, no un nombre: no debe llegar a Instances ni a Name.
func TestMDNSRealCapture_TeamViewerConflictSuffix(t *testing.T) {
	acc := decodeCapture(t, buildTeamViewerCapture(t))
	out := acc.result()

	info, ok := out["192.168.0.55"]
	if !ok {
		t.Fatalf("192.168.0.55 no está en el resultado: %+v", out)
	}
	if info.Hostname != "DESKTOP-ABC1234-2" {
		t.Errorf("Hostname = %q, want %q (la limpieza del sufijo es cosa de scan.go)", info.Hostname, "DESKTOP-ABC1234-2")
	}
	if len(info.Instances) != 0 || info.Name != "" {
		t.Errorf("Instances = %v, Name = %q: el \"0\" de TeamViewer no es un nombre", info.Instances, info.Name)
	}
}

// ---------------------------------------------------------------------------
// Unidad: splitInstance, humanInstanceName, parseMDNSTXT.
// ---------------------------------------------------------------------------

func TestSplitInstance(t *testing.T) {
	cases := []struct {
		name        string
		wantBase    string
		wantSvcType string
		wantOK      bool
	}{
		{"iMac de Estudio._device-info._tcp.local", "iMac de Estudio", "_device-info._tcp", true},
		{`75" QLED._airplay._tcp.local`, `75" QLED`, "_airplay._tcp", true},
		{"0._teamviewer._tcp.local", "0", "_teamviewer._tcp", true},
		{"_sleep-proxy._udp.local", "", "", false}, // sin nombre de instancia: es el propio tipo
		{"foo._sleep-proxy._udp.local", "foo", "_sleep-proxy._udp", true},
		{"iMac-Estudio.local", "", "", false}, // un hostname A/AAAA, no una instancia DNS-SD
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, svcType, ok := splitInstance(tc.name)
			if ok != tc.wantOK || base != tc.wantBase || svcType != tc.wantSvcType {
				t.Errorf("splitInstance(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.name, base, svcType, ok, tc.wantBase, tc.wantSvcType, tc.wantOK)
			}
		})
	}
}

// TestHumanInstanceName: casos medidos en la red real. Los identificadores
// internos (Spotify Connect, Matter, TeamViewer…) no son nombres visibles.
func TestHumanInstanceName(t *testing.T) {
	cases := []struct {
		base, svcType string
		want          string // "" = no da nombre
		prio          int
	}{
		{"d02281698a64fd53391d08d8c36d440992c48bfc", "_spotify-connect._tcp", "", 0},
		{"SpotifyConnect #3", "_spotify-connect._tcp", "", 0},
		{"SpotifyConnect", "_raop._tcp", "", 0},
		{"ACDC919AAFED4D50-014F17209F77AB81", "_matter._tcp", "", 0},
		{"ACDC919AAFED4D50-014F17209F77AB81", "_http._tcp", "", 0},
		{"0", "_teamviewer._tcp", "", 0},
		{"12345", "_http._tcp", "", 0},
		{"AABBCCDDEEFF", "_http._tcp", "", 0},
		{"0f8fad5b-d9cb-469f-a165-70867728950e", "_http._tcp", "", 0},
		{"Algo", "_FC9F5ED42C8A._tcp", "", 0},
		{`75" QLED`, "_airplay._tcp", `75" QLED`, 2},
		{"iMac de Estudio", "_device-info._tcp", "iMac de Estudio", 0},
		{"A1B2C3D4E5F6@Sala", "_raop._tcp", "Sala", 3},
		{"OctoPrint instance on octopi", "_octoprint._tcp", "OctoPrint instance on octopi", 6},
		{"Impresora", "_algo-nuevo._tcp", "Impresora", otherInstancePriority},
	}
	for _, tc := range cases {
		name, prio, ok := humanInstanceName(tc.base, tc.svcType)
		if (tc.want == "") == ok || name != tc.want || (ok && prio != tc.prio) {
			t.Errorf("humanInstanceName(%q, %q) = (%q, %d, %v), want (%q, %d)", tc.base, tc.svcType, name, prio, ok, tc.want, tc.prio)
		}
	}
}

// TestOfferNamePrioridad: gana el nombre del servicio con mejor prioridad,
// no el más largo.
func TestOfferNamePrioridad(t *testing.T) {
	var info MDNSInfo
	info.offerName("Living-Room-Speaker-Group-Largo", otherInstancePriority)
	info.offerName(`75" QLED`, 2)
	info.offerName("Otro", 5)
	if info.Name != `75" QLED` {
		t.Errorf("Name = %q, want %q", info.Name, `75" QLED`)
	}
	if len(info.Instances) != 3 {
		t.Errorf("Instances = %v, want los 3 nombres", info.Instances)
	}
}

func TestParseMDNSTXT(t *testing.T) {
	f := parseMDNSTXT([]string{
		"am=AudioAccessory5,1", // AirPlay/raop
		"manufacturer=Sonos",
		"fn=Living Room",
		"otracosa=ignorar",
	})
	if f.model != "AudioAccessory5,1" {
		t.Errorf("model = %q, want %q (clave am=)", f.model, "AudioAccessory5,1")
	}
	if f.manufacturer != "Sonos" {
		t.Errorf("manufacturer = %q, want %q", f.manufacturer, "Sonos")
	}
	if f.friendlyName != "Living Room" {
		t.Errorf("friendlyName = %q, want %q", f.friendlyName, "Living Room")
	}

	// rpMd (companion-link) y fillMissing: la propia instancia manda, el TXT
	// huérfano solo rellena huecos.
	own := parseMDNSTXT([]string{"rpMd=J105AP"})
	meta := parseMDNSTXT([]string{"model=iMac18,3", "osxvers=24", "manufacturer=Apple"})
	own.fillMissing(meta)
	if own.model != "J105AP" {
		t.Errorf("model = %q, want %q (la propia instancia no se pisa)", own.model, "J105AP")
	}
	if own.osVersion != "24" {
		t.Errorf("osVersion = %q, want %q (rellenado desde el TXT huérfano)", own.osVersion, "24")
	}
	if own.manufacturer != "Apple" {
		t.Errorf("manufacturer = %q, want %q (rellenado desde el TXT huérfano)", own.manufacturer, "Apple")
	}
}

// ---------------------------------------------------------------------------
// Sintético (paquete dnsmessage armado a mano): nombre de instancia basura
// (hex-12, como una MAC cruda) de punta a punta, vía result().
// ---------------------------------------------------------------------------

func TestMDNSResult_GarbageInstanceFiltered(t *testing.T) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	mustNoErr(t, b.StartQuestions())
	mustNoErr(t, b.StartAnswers())
	ptrTarget := mustName(t, "aabbccddeeff._ipp._tcp.local.")
	mustNoErr(t, b.PTRResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "_ipp._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.PTRResource{PTR: ptrTarget},
	))
	mustNoErr(t, b.StartAdditionals())
	mustNoErr(t, b.SRVResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "aabbccddeeff._ipp._tcp.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.SRVResource{Target: mustName(t, "impresora.local."), Port: 631},
	))
	mustNoErr(t, b.AResource(
		dnsmessage.ResourceHeader{Name: mustName(t, "impresora.local."), Class: dnsmessage.ClassINET, TTL: 10},
		dnsmessage.AResource{A: [4]byte{192, 168, 0, 90}},
	))
	data, err := b.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	acc := newMDNSAccumulator()
	conn, dest := loopbackSink(t)
	acc.handlePacket(data, conn, dest)
	out := acc.result()

	info, ok := out["192.168.0.90"]
	if !ok {
		t.Fatalf("192.168.0.90 no está en el resultado: %+v", out)
	}
	if len(info.Instances) != 0 {
		t.Errorf("Instances = %v, want vacío (el nombre de instancia es una MAC cruda)", info.Instances)
	}
	if len(info.Services) != 1 || info.Services[0] != "_ipp._tcp" {
		t.Errorf("Services = %v, want [_ipp._tcp] (el servicio en sí sigue viéndose, solo se descarta el nombre)", info.Services)
	}
}

func mustName(t *testing.T, s string) dnsmessage.Name {
	t.Helper()
	n, err := dnsmessage.NewName(s)
	if err != nil {
		t.Fatalf("NewName(%q): %v", s, err)
	}
	return n
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado armando el paquete de prueba: %v", err)
	}
}
