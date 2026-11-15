package core

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// Fixtures: descripción UPnP y respuestas SOAP de un router real (Tenda
// "Mesh Wi-Fi", 192.168.0.4 — ver docs/plan.md, "Lo que encontré
// al revisar la red"), con la forma típica de un IGD de 3 niveles
// (InternetGatewayDevice > WANDevice > WANConnectionDevice).
// ===========================================================================

const tendaRootDescXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
    <friendlyName>Tenda Mesh Wi-Fi</friendlyName>
    <manufacturer>Tenda</manufacturer>
    <modelName>Mesh Wi-Fi</modelName>
    <UDN>uuid:12345678-1234-1234-1234-123456789abc</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:L3Forwarding1</serviceId>
        <controlURL>/ctl/L3F</controlURL>
        <SCPDURL>/L3F.xml</SCPDURL>
      </service>
    </serviceList>
    <deviceList>
      <device>
        <deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
        <friendlyName>WANDevice</friendlyName>
        <UDN>uuid:12345678-1234-1234-1234-123456789ab1</UDN>
        <deviceList>
          <device>
            <deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
            <friendlyName>WANConnectionDevice</friendlyName>
            <UDN>uuid:12345678-1234-1234-1234-123456789ab2</UDN>
            <serviceList>
              <service>
                <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
                <serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
                <controlURL>/ctl/IPConn</controlURL>
                <eventSubURL>/evt/IPConn</eventSubURL>
                <SCPDURL>/WANIPCn.xml</SCPDURL>
              </service>
            </serviceList>
          </device>
        </deviceList>
      </device>
    </deviceList>
  </device>
</root>`

// pppOnlyRootDescXML: descripción con un solo servicio WAN, WANPPPConnection
// (routers PPPoE) — para probar que fetchWANControlURL también lo encuentra
// cuando no hay WANIPConnection.
const pppOnlyRootDescXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
    <deviceList>
      <device>
        <deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
        <serviceList>
          <service>
            <serviceType>urn:schemas-upnp-org:service:WANPPPConnection:1</serviceType>
            <controlURL>/ctl/PPPConn</controlURL>
          </service>
        </serviceList>
      </device>
    </deviceList>
  </device>
</root>`

// noWANServiceXML: un IGD que no expone ningún servicio WAN controlable
// (caso raro, pero fetchWANControlURL debe devolver ok=false, no explotar).
const noWANServiceXML = `<?xml version="1.0"?>
<root><device>
  <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
  <serviceList><service>
    <serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType>
    <controlURL>/ctl/L3F</controlURL>
  </service></serviceList>
</device></root>`

const soapExternalIPRespXML = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body><u:GetExternalIPAddressResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
<NewExternalIPAddress>%s</NewExternalIPAddress>
</u:GetExternalIPAddressResponse></s:Body></s:Envelope>`

const soapPortMappingRespXML = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body><u:GetGenericPortMappingEntryResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>%s</NewProtocol>
<NewInternalPort>%d</NewInternalPort>
<NewInternalClient>%s</NewInternalClient>
<NewEnabled>1</NewEnabled>
<NewPortMappingDescription>%s</NewPortMappingDescription>
<NewLeaseDuration>0</NewLeaseDuration>
</u:GetGenericPortMappingEntryResponse></s:Body></s:Envelope>`

const soapFault713XML = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body><s:Fault>
<faultcode>s:Client</faultcode>
<faultstring>UPnPError</faultstring>
<detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
<errorCode>713</errorCode>
<errorDescription>SpecifiedArrayIndexInvalid</errorDescription>
</UPnPError></detail>
</s:Fault></s:Body></s:Envelope>`

// testMapping: una redirección que el servidor SOAP falso de
// newFakeRouterServer debe devolver en GetGenericPortMappingEntry.
type testMapping struct {
	extPort, intPort   int
	proto, intIP, desc string
}

// newFakeRouterServer levanta un httptest.Server que imita rootDesc.xml +
// el control SOAP de un Tenda real: sirve descXML en "/rootDesc.xml" y, en
// "/ctl/IPConn", GetExternalIPAddress (externalIP) y
// GetGenericPortMappingEntry (mappings, terminando en el 713 de rigor).
func newFakeRouterServer(t *testing.T, descXML, externalIP string, mappings []testMapping) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rootDesc.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, descXML)
	})
	mux.HandleFunc("/ctl/IPConn", func(w http.ResponseWriter, r *http.Request) {
		soapHandler(w, r, externalIP, mappings)
	})
	mux.HandleFunc("/ctl/PPPConn", func(w http.ResponseWriter, r *http.Request) {
		soapHandler(w, r, externalIP, mappings)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func soapHandler(w http.ResponseWriter, r *http.Request, externalIP string, mappings []testMapping) {
	body, _ := io.ReadAll(r.Body)
	action := r.Header.Get("SOAPACTION")
	switch {
	case strings.Contains(action, "GetExternalIPAddress"):
		fmt.Fprintf(w, soapExternalIPRespXML, externalIP)
	case strings.Contains(action, "GetGenericPortMappingEntry"):
		idx := indexFromRequestBody(string(body))
		if idx < 0 || idx >= len(mappings) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, soapFault713XML)
			return
		}
		m := mappings[idx]
		fmt.Fprintf(w, soapPortMappingRespXML, m.extPort, m.proto, m.intPort, m.intIP, m.desc)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// indexFromRequestBody saca NewPortMappingIndex del cuerpo SOAP crudo
// (basta un strings.Cut: no hace falta un parser XML en el servidor falso).
func indexFromRequestBody(body string) int {
	start := strings.Index(body, "<NewPortMappingIndex>")
	if start < 0 {
		return -1
	}
	start += len("<NewPortMappingIndex>")
	end := strings.Index(body[start:], "<")
	if end < 0 {
		return -1
	}
	n, err := strconv.Atoi(body[start : start+end])
	if err != nil {
		return -1
	}
	return n
}

// fakeSSDPGateway levanta un servidor UDP en 127.0.0.1 que contesta
// CUALQUIER datagrama que reciba con una respuesta SSDP mínima cuya
// LOCATION es locationURL — no hace falta parsear el M-SEARCH para probar
// discoverGatewayLocation/queryUPnP. Devuelve el puerto en el que quedó
// escuchando, para sobreescribir ssdpRouterPort (ver withSSDPPort).
func fakeSSDPGateway(t *testing.T, locationURL string) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("no se pudo abrir el SSDP falso: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 2048)
		for {
			_, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // socket cerrado al terminar el test
			}
			resp := "HTTP/1.1 200 OK\r\n" +
				"CACHE-CONTROL: max-age=1800\r\n" +
				"LOCATION: " + locationURL + "\r\n" +
				"SERVER: Vigia-test/1.0 UPnP/1.1\r\n" +
				"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
			_, _ = conn.WriteToUDP([]byte(resp), raddr)
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

// withSSDPPort apunta ssdpRouterPort a port durante el test y lo restaura al
// terminar — así cada test usa su propio servidor UDP falso, nunca el 1900
// real.
func withSSDPPort(t *testing.T, port int) {
	t.Helper()
	old := ssdpRouterPort
	ssdpRouterPort = port
	t.Cleanup(func() { ssdpRouterPort = old })
}

// withNoNATPMPResponder apunta natpmpPort a un puerto UDP que se abre y
// cierra de inmediato (nadie escucha ahí): para los tests de este archivo,
// que solo verifican UPnP, así NAT-PMP falla rápido en vez de agotar los 2s
// de upnpStepTimeout esperando una respuesta que nunca llega.
func withNoNATPMPResponder(t *testing.T) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("no se pudo reservar un puerto NAT-PMP falso: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close() // cerrado a propósito: un connect+write ahí falla pronto (ECONNREFUSED)

	old := natpmpPort
	natpmpPort = port
	t.Cleanup(func() { natpmpPort = old })
}

// ===========================================================================
// queryUPnP: el camino completo (SSDP -> descripción -> SOAP).
// ===========================================================================

func TestQueryUPnP_CasoReal(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", []testMapping{
		{extPort: 8080, intPort: 80, proto: "TCP", intIP: "192.168.0.55", desc: "cam"},
	})
	port := fakeSSDPGateway(t, srv.URL+"/rootDesc.xml")
	withSSDPPort(t, port)

	ok, extIP, mappings, complete := queryUPnP(context.Background(), "127.0.0.1")
	if !ok {
		t.Fatal("queryUPnP ok = false, want true")
	}
	if !complete {
		t.Error("complete = false, want true (la lista terminó con el 713 de rigor)")
	}
	if extIP != "192.168.100.2" {
		t.Errorf("externalIP = %q, want %q", extIP, "192.168.100.2")
	}
	if len(mappings) != 1 {
		t.Fatalf("len(mappings) = %d, want 1: %+v", len(mappings), mappings)
	}
	m := mappings[0]
	if m.Protocol != "TCP" || m.ExternalPort != 8080 || m.InternalIP != "192.168.0.55" || m.InternalPort != 80 || m.Source != "upnp" {
		t.Errorf("mapping = %+v, no es lo esperado", m)
	}
}

func TestQueryUPnP_SinRespuestaSSDP(t *testing.T) {
	// ssdpRouterPort apunta a un puerto donde nadie escucha: sin LOCATION,
	// sin descripción, sin servicio WAN — ok debe quedar en false.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	withSSDPPort(t, port)

	ok, extIP, mappings, complete := queryUPnP(context.Background(), "127.0.0.1")
	if ok || extIP != "" || mappings != nil || complete {
		t.Errorf("queryUPnP = (%v, %q, %v, %v), want (false, \"\", nil, false)", ok, extIP, mappings, complete)
	}
}

func TestQueryUPnP_ListaDeRedireccionesVacia(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", nil) // 713 desde el índice 0
	port := fakeSSDPGateway(t, srv.URL+"/rootDesc.xml")
	withSSDPPort(t, port)

	ok, extIP, mappings, complete := queryUPnP(context.Background(), "127.0.0.1")
	if !ok || extIP != "192.168.100.2" || !complete {
		t.Fatalf("queryUPnP = (%v, %q, ..., %v), want (true, 192.168.100.2, ..., true)", ok, extIP, complete)
	}
	if len(mappings) != 0 {
		t.Errorf("mappings = %v, want vacío (el router no tiene redirecciones)", mappings)
	}
}

func TestQueryUPnP_VariasRedireccionesOrdenadas(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", []testMapping{
		{extPort: 9000, intPort: 9000, proto: "TCP", intIP: "192.168.0.10", desc: "b"},
		{extPort: 2222, intPort: 22, proto: "TCP", intIP: "192.168.0.20", desc: "ssh"},
	})
	port := fakeSSDPGateway(t, srv.URL+"/rootDesc.xml")
	withSSDPPort(t, port)

	ok, _, mappings, _ := queryUPnP(context.Background(), "127.0.0.1")
	if !ok || len(mappings) != 2 {
		t.Fatalf("queryUPnP ok=%v mappings=%+v", ok, mappings)
	}
	if mappings[0].ExternalPort != 2222 || mappings[1].ExternalPort != 9000 {
		t.Errorf("mappings no están ordenadas por ExternalPort: %+v", mappings)
	}
}

// El filtro "quedarse SOLO con respuestas cuya IP sea el gateway" se prueba
// aislado (sin sockets) en TestSSDPResponseIsFromGateway, más abajo: montar
// un segundo equipo que conteste por multicast desde otra IP de verdad
// necesitaría una segunda interfaz local, que no es algo que un test
// unitario deba depender de tener disponible.

// ===========================================================================
// fetchWANControlURL: hallar el controlURL correcto en el árbol de la
// descripción, con la preferencia WANIPConnection > WANPPPConnection.
// ===========================================================================

func TestFetchWANControlURL_PrefiereWANIPConnection(t *testing.T) {
	srv := newFakeRouterServer(t, tendaRootDescXML, "192.168.100.2", nil)
	cu, st, ok := fetchWANControlURL(context.Background(), srv.URL+"/rootDesc.xml")
	if !ok {
		t.Fatal("ok = false")
	}
	if st != "urn:schemas-upnp-org:service:WANIPConnection:1" {
		t.Errorf("serviceType = %q, want WANIPConnection:1", st)
	}
	if cu != srv.URL+"/ctl/IPConn" {
		t.Errorf("controlURL = %q, want %q (resuelto contra LOCATION)", cu, srv.URL+"/ctl/IPConn")
	}
}

func TestFetchWANControlURL_SoloWANPPP(t *testing.T) {
	srv := newFakeRouterServer(t, pppOnlyRootDescXML, "192.168.100.2", nil)
	cu, st, ok := fetchWANControlURL(context.Background(), srv.URL+"/rootDesc.xml")
	if !ok {
		t.Fatal("ok = false, want true (debe aceptar WANPPPConnection si no hay WANIPConnection)")
	}
	if st != "urn:schemas-upnp-org:service:WANPPPConnection:1" {
		t.Errorf("serviceType = %q, want WANPPPConnection:1", st)
	}
	if cu != srv.URL+"/ctl/PPPConn" {
		t.Errorf("controlURL = %q, want %q", cu, srv.URL+"/ctl/PPPConn")
	}
}

func TestFetchWANControlURL_SinServicioWAN(t *testing.T) {
	srv := newFakeRouterServer(t, noWANServiceXML, "", nil)
	_, _, ok := fetchWANControlURL(context.Background(), srv.URL+"/rootDesc.xml")
	if ok {
		t.Error("ok = true, want false (la descripción no trae WANIPConnection ni WANPPPConnection)")
	}
}

func TestFetchWANControlURL_LocationInalcanzable(t *testing.T) {
	_, _, ok := fetchWANControlURL(context.Background(), "http://192.0.2.1:9999/rootDesc.xml")
	if ok {
		t.Error("ok = true, want false (LOCATION inalcanzable)")
	}
}

// ===========================================================================
// upnpXMLNode: el árbol genérico que parsea la descripción.
// ===========================================================================

func TestUpnpXMLNode_FindAndText(t *testing.T) {
	var root upnpXMLNode
	if err := xml.Unmarshal([]byte(tendaRootDescXML), &root); err != nil {
		t.Fatal(err)
	}
	if got := root.text("friendlyName"); got != "Tenda Mesh Wi-Fi" {
		t.Errorf("text(friendlyName) = %q, want %q", got, "Tenda Mesh Wi-Fi")
	}
	services := root.findAll("service")
	if len(services) != 2 {
		t.Fatalf("findAll(service) = %d, want 2 (Layer3Forwarding + WANIPConnection)", len(services))
	}
	if _, ok := root.find("noExiste"); ok {
		t.Error("find(noExiste) ok = true, want false")
	}
}

// ===========================================================================
// reachFor: clasificación de la IP externa.
// ===========================================================================

func TestReachFor(t *testing.T) {
	cases := []struct {
		ip   string
		want string
	}{
		{"", "unknown"},
		{"no-es-una-ip", "unknown"},
		{"192.168.100.2", "outer_ring"}, // RFC 1918
		{"10.0.0.5", "outer_ring"},
		{"172.20.0.1", "outer_ring"},
		{"100.64.0.1", "outer_ring"}, // CGNAT, RFC 6598
		{"100.100.50.1", "outer_ring"},
		{"8.8.8.8", "internet"},
		{"200.23.45.6", "internet"},
	}
	for _, c := range cases {
		if got := reachFor(c.ip); got != c.want {
			t.Errorf("reachFor(%q) = %q, want %q", c.ip, got, c.want)
		}
	}
}

// ===========================================================================
// ssdpResponseIsFromGateway: el filtro de IP, aislado.
// ===========================================================================

func TestSSDPResponseIsFromGateway(t *testing.T) {
	gw := "192.168.0.4"
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.0.4", true},
		{"192.168.0.55", false},
		{"192.168.100.1", false},
	}
	for _, c := range cases {
		addr := &net.UDPAddr{IP: net.ParseIP(c.ip), Port: 1900}
		if got := ssdpResponseIsFromGateway(addr, gw); got != c.want {
			t.Errorf("ssdpResponseIsFromGateway(%s, %s) = %v, want %v", c.ip, gw, got, c.want)
		}
	}
	if ssdpResponseIsFromGateway(nil, gw) {
		t.Error("ssdpResponseIsFromGateway(nil, ...) = true, want false")
	}
}

// ===========================================================================
// QueryRouter: límites de tiempo (2s por paso) — con un router que nunca
// contesta, no debe quedarse esperando los 6s completos.
// ===========================================================================

func TestQueryRouter_GatewayInvalida(t *testing.T) {
	info, err := QueryRouter(context.Background(), "no-es-una-ip")
	if err == nil {
		t.Error("err = nil, want error (gateway inválida)")
	}
	if info != nil {
		t.Errorf("info = %+v, want nil", info)
	}
}

func TestQueryRouter_RespetaElPasoDeDosSegundos(t *testing.T) {
	withSSDPPort(t, closedUDPPort(t))
	withNoNATPMPResponder(t)

	start := time.Now()
	info, err := QueryRouter(context.Background(), "127.0.0.1")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("err = %v, want nil (no contestar no es un error)", err)
	}
	if info != nil {
		t.Errorf("info = %+v, want nil (ni UPnP ni NAT-PMP contestaron)", info)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("QueryRouter tardó %s; con upnpStepTimeout=2s no debería acercarse a los 6s totales", elapsed)
	}
}

// closedUDPPort reserva un puerto UDP en 127.0.0.1 y lo cierra de inmediato:
// un número de puerto válido donde se sabe que nadie escucha.
func closedUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	return port
}

func TestURLHostIs(t *testing.T) {
	cases := []struct {
		url, host string
		want      bool
	}{
		{"http://192.168.0.4:5500/rootDesc.xml", "192.168.0.4", true},
		{"http://192.168.0.4:5500/ctl/IPConn", "192.168.0.4", true},
		{"http://192.168.0.66:5500/rootDesc.xml", "192.168.0.4", false}, // otro equipo de la red
		{"http://203.0.113.9/ctl", "192.168.0.4", false},                // fuera de la casa
		{"ftp://192.168.0.4/rootDesc.xml", "192.168.0.4", false},        // solo http(s)
		{"http://192.168.0.4.evil.example/x", "192.168.0.4", false},     // no basta con el prefijo
		{"::not a url::", "192.168.0.4", false},
	}
	for _, c := range cases {
		if got := urlHostIs(c.url, c.host); got != c.want {
			t.Errorf("urlHostIs(%q, %q) = %v, want %v", c.url, c.host, got, c.want)
		}
	}
}

// Una lista que se corta con un error que no es el de fin de lista queda
// marcada como incompleta: el agente no debe tomar las ausentes como cerradas.
func TestUPnPPortMappings_CortadaAMediasEsIncompleta(t *testing.T) {
	fault501 := strings.Replace(soapFault713XML, "<errorCode>713</errorCode>", "<errorCode>501</errorCode>", 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if indexFromRequestBody(string(body)) == 0 {
			fmt.Fprintf(w, soapPortMappingRespXML, 7070, "TCP", 7070, "192.168.0.63", "AnyDesk")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, fault501)
	}))
	t.Cleanup(srv.Close)

	mappings, complete := upnpPortMappingsFor(context.Background(), srv.URL, "urn:schemas-upnp-org:service:WANIPConnection:1")
	if complete {
		t.Error("complete = true, want false (la lista se cortó con un 501)")
	}
	if len(mappings) != 1 || mappings[0].InternalIP != "192.168.0.63" {
		t.Errorf("mappings = %+v, want la que alcanzó a leer", mappings)
	}
}
