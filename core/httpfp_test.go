package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// splitTestServer separa el host:puerto de un httptest.Server en (ip, puerto
// int), que es como fetchTitle/httpGET/fingerprint* reciben los suyos (IP y
// puerto por separado, no una URL armada).
func splitTestServer(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	u := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "https://"), "http://")
	host, portStr, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatalf("no se pudo separar host:puerto de %q: %v", srv.URL, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("puerto no numérico en %q: %v", srv.URL, err)
	}
	return host, port
}

// ---------------------------------------------------------------------------
// fetchTitle: GET "/" genérico (cámara .58, OctoPrint, routers...).
// ---------------------------------------------------------------------------

func TestFetchTitle_CameraLikeResponse(t *testing.T) {
	// Respuesta real de la cámara .58: HTTP/1.0 200, sin cabecera Server,
	// <title>Web Viewer</title>.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "Vigia" {
			t.Errorf("User-Agent = %q, want %q", got, "Vigia")
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!DOCTYPE html>\n<html><head><title>Web Viewer</title></head><body></body></html>")
	}))
	defer srv.Close()

	ip, port := splitTestServer(t, srv)
	title, server, ok := fetchTitle(context.Background(), ip, port, false)
	if !ok {
		t.Fatal("fetchTitle: ok = false")
	}
	if title != "Web Viewer" {
		t.Errorf("title = %q, want %q", title, "Web Viewer")
	}
	if server != "" {
		t.Errorf("server = %q, want vacío (la cámara no manda Server)", server)
	}
}

func TestFetchTitle_ServerHeaderAndEntities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "lighttpd/1.4")
		fmt.Fprint(w, `<html><head><title>75&quot; QLED</title></head></html>`)
	}))
	defer srv.Close()

	ip, port := splitTestServer(t, srv)
	title, server, ok := fetchTitle(context.Background(), ip, port, false)
	if !ok {
		t.Fatal("ok = false")
	}
	if title != `75" QLED` {
		t.Errorf("title = %q, want %q (entidad HTML decodificada)", title, `75" QLED`)
	}
	if server != "lighttpd/1.4" {
		t.Errorf("server = %q, want %q", server, "lighttpd/1.4")
	}
}

func TestFetchTitle_BodyCapAt64KB(t *testing.T) {
	huge := strings.Repeat("x", 200<<10) // 200 KB, bien por encima del tope de 64 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><head><title>")
		fmt.Fprint(w, huge)
		// el título nunca cierra dentro de los primeros 64 KB: no debe haber
		// title, y sobre todo no debe explotar ni colgarse leyendo 200 KB.
	}))
	defer srv.Close()

	ip, port := splitTestServer(t, srv)
	title, _, _ := fetchTitle(context.Background(), ip, port, false)
	if title != "" {
		t.Errorf("title = %q, want vacío (el cierre de <title> está más allá de 64 KB)", title)
	}
}

func TestFetchTitle_NoRedirectToOtherHost(t *testing.T) {
	// 192.0.2.1 (TEST-NET-1, RFC 5737) nunca es alcanzable: si CheckRedirect
	// dejara seguir la redirección, el cliente intentaría conectarse ahí. Con
	// http.ErrUseLastResponse no llega a intentarlo: se queda con la 302
	// original, que no tiene <title>.
	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.1/otro-host", http.StatusFound)
	}))
	defer main.Close()

	ip, port := splitTestServer(t, main)
	title, _, ok := fetchTitle(context.Background(), ip, port, false)
	if ok {
		t.Errorf("ok = true, want false (solo hay una 302 sin <title> ni Server)")
	}
	if title != "" {
		t.Errorf("title = %q, want vacío (no debía seguir la redirección a otro host)", title)
	}
}

// ---------------------------------------------------------------------------
// APIs propietarias de TV: Samsung/Roku/Chromecast.
// ---------------------------------------------------------------------------

// samsungRealPayload: respuesta real de GET http://192.168.0.118:8001/api/v2/
// (ver AGENTS.md / hechos medidos), recortada a los campos que parseamos.
const samsungRealPayload = `{"device":{"OS":"Tizen","PowerState":"on","countryCode":"MX","model":"23_PTML_QTV_T09","modelName":"QN75Q70CDFXZA","name":"75&quot; QLED","networkType":"wireless","type":"Samsung SmartTV","wifiMac":"02:00:00:00:01:18"},"id":"uuid:00000000-0000-4000-8000-000000000118","name":"75&quot; QLED","type":"Samsung SmartTV","version":"2.0.25"}`

func TestFingerprintSamsung_RealPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/")
		}
		fmt.Fprint(w, samsungRealPayload)
	}))
	defer srv.Close()
	ip, port := splitTestServer(t, srv)

	d := &Device{IP: ip}
	fingerprintSamsung(context.Background(), d, port)

	want := map[string]string{
		"api:type":  "tv",
		"api:os":    "Tizen",
		"api:model": "QN75Q70CDFXZA",
		"api:name":  `75" QLED`,
	}
	for k, v := range want {
		if d.Extra[k] != v {
			t.Errorf("Extra[%q] = %q, want %q", k, d.Extra[k], v)
		}
	}
	if d.MAC != "02:00:00:00:01:18" {
		t.Errorf("MAC = %q, want %q (adoptada de wifiMac, sin ARP)", d.MAC, "02:00:00:00:01:18")
	}
	if len(d.Sources) == 0 || d.Sources[0] != "api" {
		t.Errorf("Sources = %v, want [\"api\"]", d.Sources)
	}
}

func TestFingerprintSamsung_DoesNotOverrideExistingMAC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, samsungRealPayload)
	}))
	defer srv.Close()
	ip, port := splitTestServer(t, srv)

	d := &Device{IP: ip, MAC: "aa:bb:cc:dd:ee:ff"}
	fingerprintSamsung(context.Background(), d, port)
	if d.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC = %q, no debía pisar una MAC que ya teníamos (de ARP)", d.MAC)
	}
}

func TestFingerprintRoku(t *testing.T) {
	const rokuXML = `<?xml version="1.0" encoding="UTF-8" ?>
<device-info>
	<model-name>Roku Ultra</model-name>
	<vendor-name>Roku</vendor-name>
	<wifi-mac>b0:a7:37:11:22:33</wifi-mac>
	<friendly-device-name>Sala</friendly-device-name>
</device-info>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/device-info" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/query/device-info")
		}
		fmt.Fprint(w, rokuXML)
	}))
	defer srv.Close()
	ip, port := splitTestServer(t, srv)

	d := &Device{IP: ip}
	fingerprintRoku(context.Background(), d, port)

	want := map[string]string{
		"api:type":  "tv",
		"api:os":    "Roku OS",
		"api:model": "Roku Ultra",
		"api:name":  "Sala",
	}
	for k, v := range want {
		if d.Extra[k] != v {
			t.Errorf("Extra[%q] = %q, want %q", k, d.Extra[k], v)
		}
	}
	if d.MAC != "b0:a7:37:11:22:33" {
		t.Errorf("MAC = %q, want %q", d.MAC, "b0:a7:37:11:22:33")
	}
}

func TestFingerprintChromecast_BestEffort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Living Room TV","model_name":"Chromecast Ultra","cast_build_revision":"1.56"}`)
	}))
	defer srv.Close()
	ip, port := splitTestServer(t, srv)

	d := &Device{IP: ip}
	fingerprintChromecast(context.Background(), d, port)

	if d.Extra["api:type"] != "tv" || d.Extra["api:model"] != "Chromecast Ultra" || d.Extra["api:name"] != "Living Room TV" {
		t.Errorf("Extra = %v, faltan campos esperados", d.Extra)
	}
}

func TestFingerprintChromecast_UnexpectedBodyIsHarmless(t *testing.T) {
	// "best effort": una respuesta que no es el JSON esperado no debe entrar
	// en pánico ni dejar Extra a medio llenar con basura.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json at all`)
	}))
	defer srv.Close()
	ip, port := splitTestServer(t, srv)

	d := &Device{IP: ip}
	fingerprintChromecast(context.Background(), d, port)
	if len(d.Extra) != 0 {
		t.Errorf("Extra = %v, want vacío (el body no era JSON)", d.Extra)
	}
}

// ---------------------------------------------------------------------------
// fingerprintHTTP: orquestación (puertos relevantes, salida temprana).
// ---------------------------------------------------------------------------

func TestFingerprintHTTP_NoRelevantPorts(t *testing.T) {
	d := &Device{IP: "192.168.0.1", OpenPorts: []int{22, 445}}
	fingerprintHTTP(context.Background(), d) // no debe tocar la red ni entrar en pánico
	if len(d.Extra) != 0 {
		t.Errorf("Extra = %v, want vacío (ningún puerto relevante abierto)", d.Extra)
	}
}

// TestFingerprintHTTP_EndToEndOnRealAPIPort prueba fingerprintHTTP completo
// (no solo fingerprintSamsung) escuchando en el puerto 8001 de verdad: es el
// único modo fiel, porque apiPort()/fingerprintHTTP usan ese número tal cual
// para armar la URL, no lo reciben como parámetro. Si el entorno no permite
// abrir ese puerto (ya ocupado, sandbox sin permiso), el test se salta en vez
// de fallar: no es señal de una regresión.
func TestFingerprintHTTP_EndToEndOnRealAPIPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:8001")
	if err != nil {
		t.Skipf("no se pudo escuchar en 127.0.0.1:8001 en este entorno: %v", err)
	}
	srv := &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, samsungRealPayload) })},
	}
	srv.Start()
	defer srv.Close()

	d := &Device{IP: "127.0.0.1", OpenPorts: []int{8001}}
	fingerprintHTTP(context.Background(), d)

	if d.Extra["api:type"] != "tv" || d.Extra["api:model"] != "QN75Q70CDFXZA" || d.Extra["api:os"] != "Tizen" {
		t.Errorf("Extra = %v, want api:type=tv api:model=QN75Q70CDFXZA api:os=Tizen", d.Extra)
	}
}
