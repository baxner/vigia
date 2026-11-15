package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// Cliente UPnP IGD (SSDP + SOAP) y NAT-PMP (natpmp.go): lo que Fase 5 llama
// "Router" en docs/agente-api.md, sección Seguridad. SOLO LECTURA: en
// ningún camino de este archivo se llama AddPortMapping/DeletePortMapping,
// a propósito — el motor nunca cambia la configuración del router.
//
// Límites del contrato: 2 s por paso (upnpStepTimeout) y 6 s en total por
// QueryRouter (upnpTotalTimeout), sin seguir redirecciones HTTP y con
// respuestas acotadas a 64 KB (upnpMaxBody) — un router hostil o roto no
// debe poder colgar ni inflar de memoria un escaneo.
const (
	upnpStepTimeout     = 2 * time.Second
	upnpTotalTimeout    = 6 * time.Second
	upnpMaxBody         = 64 << 10
	upnpMaxPortMappings = 256

	// upnpErrSpecifiedArrayIndexInvalid: código UPnP IGD que GetGenericPortMappingEntry
	// devuelve cuando el índice pedido ya pasó la última redirección: fin de
	// la lista, no un error real (ver upnpPortMappings).
	upnpErrSpecifiedArrayIndexInvalid = 713
	// upnpErrNoSuchEntryInArray: algunos routers terminan la lista con este en vez del 713.
	upnpErrNoSuchEntryInArray = 714
)

// ssdpRouterPort: puerto UDP de SSDP (1900, UPnP). var en vez de const para
// poder apuntarlo a un servidor UDP falso en las pruebas (ver upnp_test.go);
// en producción siempre es 1900.
var ssdpRouterPort = 1900

// ssdpResendAfter: cuándo repetir el M-SEARCH si todavía no contestó nadie.
const ssdpResendAfter = 500 * time.Millisecond

// ssdpMulticastAddr: grupo multicast estándar de SSDP. Fijo (no var): a
// diferencia del unicast al gateway, nada en las pruebas necesita cambiarlo
// — el envío a este grupo es best-effort y las pruebas no esperan respuesta
// por este camino (ver runSSDPDiscoveryForGateway).
const ssdpMulticastAddr = "239.255.255.250:1900"

// igdSearchTargets: los ST que pide el contrato de la Fase 5, en el orden en
// que UPnP recomienda probarlos (el dispositivo raíz primero, los servicios
// de conexión después).
var igdSearchTargets = []string{
	"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
	"urn:schemas-upnp-org:device:InternetGatewayDevice:2",
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
}

// wanServiceTypes: los serviceType de la descripción UPnP que sabemos
// controlar por SOAP (GetExternalIPAddress / GetGenericPortMappingEntry),
// en orden de preferencia: WANIPConnection antes que WANPPPConnection (PPPoE
// es el caso menos común detrás de un Tenda/router de cable o fibra).
var wanServiceTypes = []string{
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
}

// RouterInfo es lo que QueryRouter averigua del router por UPnP/NAT-PMP (ver
// docs/agente-api.md, "Seguridad v1.2"). MAC sale de la tabla ARP local
// (neighborTable, best-effort: en Android puede quedar vacía).
type RouterInfo struct {
	IP     string `json:"ip"`
	MAC    string `json:"mac,omitempty"`
	UPnP   bool   `json:"upnp"`
	NATPMP bool   `json:"natpmp"`
	// ExternalIP: IP de salida del router, por UPnP o NAT-PMP (lo que haya
	// contestado; si ambos, se prefiere UPnP). "" si ninguno la dio.
	ExternalIP string `json:"externalIp"`
	// Reach: "outer_ring" (ExternalIP privada o CGNAT: solo se ve desde la
	// red de delante del router), "internet" (ExternalIP pública) o
	// "unknown" (sin ExternalIP).
	Reach        string        `json:"reach"`
	PortMappings []PortMapping `json:"portMappings"`
	// MappingsComplete: la lista de PortMappings llegó entera (terminó con el
	// código de fin de lista). false si UPnP no contestó o la lista se cortó a
	// medias: quien la guarde no debe tomar las ausentes como cerradas (si no,
	// la próxima consulta las "redescubre" y avisa de nuevo).
	MappingsComplete bool `json:"-"`
}

// PortMapping es una redirección de puertos del router. FirstSeen/LastSeen y
// DeviceID quedan en cero-valor cuando sale de QueryRouter: ese historial
// (ver docs/agente-api.md) lo lleva el agente al guardar las redirecciones
// de un escaneo a otro, no este motor sin estado.
type PortMapping struct {
	Protocol     string     `json:"protocol"` // "TCP" | "UDP"
	ExternalPort int        `json:"externalPort"`
	InternalIP   string     `json:"internalIp"`
	InternalPort int        `json:"internalPort"`
	Description  string     `json:"description"`
	Source       string     `json:"source"` // "upnp" | "probe"
	DeviceID     *int64     `json:"deviceId"`
	FirstSeen    *time.Time `json:"firstSeen"`
	LastSeen     *time.Time `json:"lastSeen"`
}

// QueryRouter averigua lo que pueda del router en gateway por UPnP (SSDP +
// SOAP) y NAT-PMP, en paralelo. SOLO LECTURA.
//
// Devuelve (nil, nil) — no (nil, err) — si el router no contesta ni UPnP ni
// NAT-PMP: no contestar es el caso normal de la mayoría de los routers con
// esos protocolos apagados, no un error (así lo trata el contrato: "null si
// el router no contesta", ver Mobile.routerInfo en docs/agente-api.md). err
// solo se usa para un gateway mal formado.
//
// Presupuesto: 6 s en total (upnpTotalTimeout) sin importar cuánto falte por
// enumerar de las redirecciones UPnP — con muchas, QueryRouter puede
// devolver una lista parcial en vez de agotar el tiempo del llamador.
func QueryRouter(ctx context.Context, gateway string) (*RouterInfo, error) {
	return QueryRouterOnInterface(ctx, gateway, "")
}

// QueryRouterOnInterface is like QueryRouter but binds UPnP/SSDP/NAT-PMP
// sockets and HTTP requests to iface when non-empty (falling back cleanly if
// iface is missing or binding fails).
func QueryRouterOnInterface(ctx context.Context, gateway, iface string) (*RouterInfo, error) {
	if net.ParseIP(gateway) == nil {
		return nil, errors.New("gateway inválida: " + gateway)
	}

	ctx, cancel := context.WithTimeout(ctx, upnpTotalTimeout)
	defer cancel()

	var (
		upnpOK           bool
		upnpExtIP        string
		portMappings     []PortMapping
		mappingsComplete bool
		natpmpOK         bool
		natExtIP         string
		wg               sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		upnpOK, upnpExtIP, portMappings, mappingsComplete = queryUPnPWithIface(ctx, gateway, iface)
	}()
	go func() {
		defer wg.Done()
		natExtIP, natpmpOK = queryNATPMPWithIface(ctx, gateway, iface)
	}()
	wg.Wait()

	if !upnpOK && !natpmpOK {
		return nil, nil // "no contesta": ver comentario de la función
	}

	info := &RouterInfo{
		IP:               gateway,
		MAC:              neighborTable()[gateway], // best-effort; "" en Android (ARP bloqueada)
		UPnP:             upnpOK,
		NATPMP:           natpmpOK,
		PortMappings:     portMappings,
		MappingsComplete: mappingsComplete,
	}
	if info.PortMappings == nil {
		info.PortMappings = []PortMapping{}
	}
	info.ExternalIP = firstNonEmpty(upnpExtIP, natExtIP)
	info.Reach = reachFor(info.ExternalIP)
	return info, nil
}

// reachFor clasifica la IP de salida del router: "outer_ring" si es privada
// o CGNAT (10/8, 172.16/12, 192.168/16, 100.64/10 — RFC 1918 + RFC 6598),
// "internet" si es pública, "unknown" si está vacía o no se pudo parsear.
func reachFor(externalIP string) string {
	if externalIP == "" {
		return "unknown"
	}
	ip := net.ParseIP(externalIP).To4()
	if ip == nil {
		return "unknown"
	}
	if ip.IsPrivate() || isCGNAT(ip) {
		return "outer_ring"
	}
	return "internet"
}

// cgnatBlock: 100.64.0.0/10 (RFC 6598), el rango que usan los proveedores
// para CGNAT. net.IP.IsPrivate() (RFC 1918 + ULA) no lo incluye.
var cgnatBlock = mustParseCIDR("100.64.0.0/10")

func isCGNAT(ip net.IP) bool {
	return cgnatBlock.Contains(ip)
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err) // constante propia, mal formada sería un error del programa
	}
	return n
}

// queryUPnP hace el SSDP + SOAP completo para gateway: descubre la
// descripción del dispositivo, encuentra el controlURL de
// WANIPConnection/WANPPPConnection y pide GetExternalIPAddress +
// GetGenericPortMappingEntry. ok=false si el router no contestó SSDP o no
// tiene (o no expone) un servicio WAN controlable — en ese caso los demás
// valores de vuelta son cero-valor, no un error: UPnP simplemente no está
// disponible.
func queryUPnP(ctx context.Context, gateway string) (ok bool, externalIP string, mappings []PortMapping, complete bool) {
	return queryUPnPWithIface(ctx, gateway, "")
}

func queryUPnPWithIface(ctx context.Context, gateway, iface string) (ok bool, externalIP string, mappings []PortMapping, complete bool) {
	location, found := discoverGatewayLocationWithIface(ctx, gateway, iface)
	if !found {
		return false, "", nil, false
	}
	// Solo se habla con el router: la respuesta SSDP viene de su IP, pero las
	// URL de dentro (LOCATION, URLBase, controlURL) las escribe quien contesta,
	// y un equipo de la red haciéndose pasar por él podría mandarnos a
	// cualquier lado (incluso fuera de la casa).
	if !urlHostIs(location, gateway) {
		return true, "", nil, false
	}
	client := lanClientFor(iface)
	controlURL, serviceType, found := fetchWANControlURLWithClient(ctx, client, location)
	if found && !urlHostIs(controlURL, gateway) {
		found = false
	}
	if !found {
		// Contestó SSDP (sabemos que el router tiene UPnP) pero no pudimos
		// bajar/entender su descripción o no trae un servicio WAN conocido:
		// se reporta upnp=true igual (SSDP sí contestó) sin IP ni redirecciones.
		return true, "", nil, false
	}

	extIP, _ := upnpExternalIPWithClient(ctx, client, controlURL, serviceType)
	pm, complete := upnpPortMappingsForWithClient(ctx, client, controlURL, serviceType)
	return true, extIP, pm, complete
}

// discoverGatewayLocation manda M-SEARCH (unicast directo a gateway y
// multicast al grupo SSDP) y se queda con la primera LOCATION de una
// respuesta cuya IP sea exactamente gateway — cualquier otra respuesta
// (de otro equipo que conteste al multicast) se ignora. Tope: upnpStepTimeout.
func discoverGatewayLocation(ctx context.Context, gateway string) (location string, ok bool) {
	return discoverGatewayLocationWithIface(ctx, gateway, "")
}

func discoverGatewayLocationWithIface(ctx context.Context, gateway, iface string) (location string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, upnpStepTimeout)
	defer cancel()

	lc := newListenConfig(iface, nil)
	conn, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return "", false
	}
	defer conn.Close()

	if ifc := interfaceByName(iface); ifc != nil {
		_ = ipv4.NewPacketConn(conn).SetMulticastInterface(ifc)
	}

	stop := make(chan struct{})
	var bgWG sync.WaitGroup
	defer bgWG.Wait()
	defer close(stop)

	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		select {
		case <-ctx.Done():
			conn.Close() // desbloquea ReadFrom, que no toma ctx
		case <-stop:
		}
	}()

	unicastDest := &net.UDPAddr{IP: net.ParseIP(gateway), Port: ssdpRouterPort}
	var multicastDest *net.UDPAddr
	if d, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddr); err == nil {
		multicastDest = d
	}

	sendSSDPSearch(conn, unicastDest, multicastDest)
	// Una segunda tanda un poco después: las dos rondas de sendSSDPSearch
	// salen juntas, y si el router no atendió en ese instante se perdían las dos.
	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		timer := time.NewTimer(ssdpResendAfter)
		defer timer.Stop()
		select {
		case <-timer.C:
			sendSSDPSearch(conn, unicastDest, multicastDest)
		case <-stop:
		case <-ctx.Done():
		}
	}()

	buf := make([]byte, 65535)
	for {
		n, raddr, err := conn.ReadFrom(buf)
		if err != nil {
			return "", false // socket cerrado (ctx listo) u otro error: no hubo respuesta útil
		}
		ua, _ := raddr.(*net.UDPAddr)
		if !ssdpResponseIsFromGateway(ua, gateway) {
			continue // "quedarse SOLO con respuestas cuya IP sea el gateway"
		}
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(buf[:n])), nil)
		if err != nil {
			continue
		}
		loc := strings.TrimSpace(resp.Header.Get("LOCATION"))
		resp.Body.Close()
		if loc != "" {
			return loc, true
		}
	}
}

// ssdpResponseIsFromGateway dice si una respuesta SSDP cuenta para esta
// consulta: solo las que vienen exactamente de gateway (ver
// discoverGatewayLocation) — cualquier otro equipo que conteste al
// multicast se ignora. Función pura, separada del socket para poder
// probarla sin red real.
func ssdpResponseIsFromGateway(raddr *net.UDPAddr, gateway string) bool {
	return raddr != nil && raddr.IP.String() == gateway
}

// sendSSDPSearch manda un M-SEARCH por cada ST del contrato, dos veces
// (UDP se pierde) y por los dos caminos: unicast directo al gateway (lo que
// de verdad necesitamos) y multicast al grupo SSDP (por si el router solo
// contesta ahí; igual se descarta cualquier respuesta que no venga del
// gateway). Todo best-effort: un error al mandar un paquete no aborta el resto.
func sendSSDPSearch(conn net.PacketConn, unicastDest, multicastDest *net.UDPAddr) {
	for round := 0; round < 2; round++ {
		for _, st := range igdSearchTargets {
			req := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"MAN: \"ssdp:discover\"\r\n"+
				"MX: 2\r\n"+
				"ST: %s\r\n\r\n", ssdpMulticastAddr, st)
			_, _ = conn.WriteTo([]byte(req), unicastDest)
			if multicastDest != nil {
				_, _ = conn.WriteTo([]byte(req), multicastDest)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Descripción UPnP: bajar LOCATION y hallar el controlURL de un servicio WAN.
// ---------------------------------------------------------------------------

// upnpXMLNode es un árbol XML genérico: la descripción IGD anida
// <device><deviceList><device>...</device></deviceList></device> a varios
// niveles (InternetGatewayDevice > WANDevice > WANConnectionDevice) y el
// servicio que nos interesa puede estar en cualquiera, así que se parsea
// como árbol en vez de con structs fijos por nivel.
type upnpXMLNode struct {
	XMLName xml.Name
	Content string        `xml:",chardata"`
	Nodes   []upnpXMLNode `xml:",any"`
}

// find busca, en profundidad, el primer nodo cuyo nombre local sea name.
func (n upnpXMLNode) find(name string) (upnpXMLNode, bool) {
	if n.XMLName.Local == name {
		return n, true
	}
	for _, c := range n.Nodes {
		if found, ok := c.find(name); ok {
			return found, true
		}
	}
	return upnpXMLNode{}, false
}

// findAll junta TODOS los nodos con ese nombre local, en profundidad.
func (n upnpXMLNode) findAll(name string) []upnpXMLNode {
	var out []upnpXMLNode
	if n.XMLName.Local == name {
		out = append(out, n)
	}
	for _, c := range n.Nodes {
		out = append(out, c.findAll(name)...)
	}
	return out
}

// text: el contenido de texto del primer nodo con ese nombre, recortado.
func (n upnpXMLNode) text(name string) string {
	if c, ok := n.find(name); ok {
		return strings.TrimSpace(c.Content)
	}
	return ""
}

// fetchWANControlURL baja la descripción XML de location (GET, ≤upnpMaxBody,
// sin redirecciones fuera del host — mismo lanClient que httpfp.go usa para
// la LAN) y busca, entre TODOS los <service> del árbol, el primero cuyo
// serviceType esté en wanServiceTypes (en ese orden de preferencia).
// Resuelve controlURL (relativo casi siempre) contra URLBase si la
// descripción trae uno válido, o si no, contra location.
func fetchWANControlURL(ctx context.Context, location string) (controlURL, serviceType string, ok bool) {
	return fetchWANControlURLWithClient(ctx, lanClient, location)
}

func fetchWANControlURLWithClient(ctx context.Context, client *http.Client, location string) (controlURL, serviceType string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, upnpStepTimeout)
	defer cancel()

	body, err := lanGETWithClient(ctx, client, location)
	if err != nil {
		return "", "", false
	}

	var root upnpXMLNode
	if err := xml.Unmarshal(body, &root); err != nil {
		return "", "", false
	}

	base, err := url.Parse(location)
	if err != nil {
		return "", "", false
	}
	if ub := root.text("URLBase"); ub != "" {
		if b, err := url.Parse(ub); err == nil {
			base = b
		}
	}

	services := root.findAll("service")
	for _, want := range wanServiceTypes {
		for _, svc := range services {
			if svc.text("serviceType") != want {
				continue
			}
			cu := svc.text("controlURL")
			if cu == "" {
				continue
			}
			ref, err := url.Parse(cu)
			if err != nil {
				continue
			}
			return base.ResolveReference(ref).String(), want, true
		}
	}
	return "", "", false
}

// urlHostIs: true si rawURL es http(s) y apunta exactamente a host.
func urlHostIs(rawURL, host string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return u.Hostname() == host
}

// lanGET: GET genérico (no solo a dispositivos de la LAN propia: el router
// también cuenta) con el cliente sin proxy/keep-alive de httpfp.go, cuerpo
// acotado a upnpMaxBody.
func lanGET(ctx context.Context, rawURL string) ([]byte, error) {
	return lanGETWithClient(ctx, lanClient, rawURL)
}

func lanGETWithClient(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Vigia")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, upnpMaxBody))
}

// ---------------------------------------------------------------------------
// SOAP: GetExternalIPAddress / GetGenericPortMappingEntry.
// ---------------------------------------------------------------------------

// soapFaultError es un SOAP Fault de UPnP (detail/UPnPError/errorCode): lo
// usa upnpPortMappingsFor para reconocer el 713 (SpecifiedArrayIndexInvalid)
// que marca el fin de la lista de redirecciones, sin tratarlo como falla.
type soapFaultError struct {
	Code int
	Desc string
}

func (e *soapFaultError) Error() string {
	return fmt.Sprintf("SOAP fault %d: %s", e.Code, e.Desc)
}

// soapEnvelope arma el cuerpo SOAP 1.1 de una acción UPnP con argumentos
// (orden preservado: GetGenericPortMappingEntry solo tiene uno,
// NewPortMappingIndex, pero una lista ordenada es más honesta que un map).
func soapEnvelope(serviceType, action string, args [][2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&b, `<u:%s xmlns:u=%q>`, action, serviceType)
	for _, kv := range args {
		fmt.Fprintf(&b, "<%s>%s</%s>", kv[0], xmlEscape(kv[1]), kv[0])
	}
	fmt.Fprintf(&b, `</u:%s>`, action)
	b.WriteString(`</s:Body></s:Envelope>`)
	return b.String()
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// doSOAP manda la acción SOAP a controlURL y devuelve el árbol de la
// respuesta. Si el router contestó con un SOAP Fault, err es *soapFaultError
// (con el errorCode de UPnP, si lo traía). timeout propio de upnpStepTimeout,
// independiente del presupuesto total de QueryRouter.
func doSOAP(ctx context.Context, controlURL, serviceType, action string, args [][2]string) (upnpXMLNode, error) {
	return doSOAPWithClient(ctx, lanClient, controlURL, serviceType, action, args)
}

func doSOAPWithClient(ctx context.Context, client *http.Client, controlURL, serviceType, action string, args [][2]string) (upnpXMLNode, error) {
	ctx, cancel := context.WithTimeout(ctx, upnpStepTimeout)
	defer cancel()

	body := soapEnvelope(serviceType, action, args)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(body))
	if err != nil {
		return upnpXMLNode{}, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"%s#%s"`, serviceType, action))
	req.Header.Set("User-Agent", "Vigia")

	resp, err := client.Do(req)
	if err != nil {
		return upnpXMLNode{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, upnpMaxBody))
	if err != nil {
		return upnpXMLNode{}, err
	}

	var env upnpXMLNode
	if err := xml.Unmarshal(raw, &env); err != nil {
		return upnpXMLNode{}, err
	}
	if resp.StatusCode == http.StatusInternalServerError {
		code, _ := strconv.Atoi(env.text("errorCode"))
		return env, &soapFaultError{Code: code, Desc: env.text("errorDescription")}
	}
	if resp.StatusCode != http.StatusOK {
		return upnpXMLNode{}, fmt.Errorf("SOAP %s: HTTP %d", action, resp.StatusCode)
	}
	return env, nil
}

// upnpExternalIP pide GetExternalIPAddress. ok=false si la acción falló o
// vino vacía (algunos routers la dejan en "0.0.0.0" mientras no hay enlace;
// eso también cuenta como "sin IP externa").
func upnpExternalIP(ctx context.Context, controlURL, serviceType string) (string, bool) {
	return upnpExternalIPWithClient(ctx, lanClient, controlURL, serviceType)
}

func upnpExternalIPWithClient(ctx context.Context, client *http.Client, controlURL, serviceType string) (string, bool) {
	env, err := doSOAPWithClient(ctx, client, controlURL, serviceType, "GetExternalIPAddress", nil)
	if err != nil {
		return "", false
	}
	ip := env.text("NewExternalIPAddress")
	if ip == "" || ip == "0.0.0.0" {
		return "", false
	}
	return ip, true
}

// upnpPortMappingsFor enumera las redirecciones con GetGenericPortMappingEntry,
// índice 0, 1, 2... hasta el primer error (normalmente 713,
// SpecifiedArrayIndexInvalid: fin de la lista) o hasta upnpMaxPortMappings.
// Cualquier otro error (router raro, se cortó la red) también termina el
// bucle en vez de reintentar: SOLO LECTURA y best-effort, una lista parcial
// es mejor que colgarse. El contexto total (QueryRouter) puede cortar esto
// antes de llegar al final si hay muchas redirecciones; ver upnpTotalTimeout.
func upnpPortMappingsFor(ctx context.Context, controlURL, serviceType string) (mappings []PortMapping, complete bool) {
	return upnpPortMappingsForWithClient(ctx, lanClient, controlURL, serviceType)
}

func upnpPortMappingsForWithClient(ctx context.Context, client *http.Client, controlURL, serviceType string) (mappings []PortMapping, complete bool) {
	var out []PortMapping
	for i := 0; i < upnpMaxPortMappings; i++ {
		select {
		case <-ctx.Done():
			return out, false
		default:
		}
		env, err := doSOAPWithClient(ctx, client, controlURL, serviceType, "GetGenericPortMappingEntry",
			[][2]string{{"NewPortMappingIndex", strconv.Itoa(i)}})
		if err != nil {
			// 713/714 es el fin normal de la lista: entonces está completa.
			// Cualquier otro error (tiempo, red, respuesta rara) la deja a
			// medias: se devuelve lo leído, marcado como incompleto.
			var fault *soapFaultError
			complete = errors.As(err, &fault) &&
				(fault.Code == upnpErrSpecifiedArrayIndexInvalid || fault.Code == upnpErrNoSuchEntryInArray)
			sortPortMappings(out)
			return out, complete
		}
		extPort, _ := strconv.Atoi(env.text("NewExternalPort"))
		intPort, _ := strconv.Atoi(env.text("NewInternalPort"))
		out = append(out, PortMapping{
			Protocol:     strings.ToUpper(env.text("NewProtocol")),
			ExternalPort: extPort,
			InternalIP:   env.text("NewInternalClient"),
			InternalPort: intPort,
			Description:  env.text("NewPortMappingDescription"),
			Source:       "upnp",
		})
	}
	// Tope alcanzado: puede haber más.
	sortPortMappings(out)
	return out, false
}

func sortPortMappings(pm []PortMapping) {
	sort.SliceStable(pm, func(i, j int) bool {
		if pm[i].ExternalPort != pm[j].ExternalPort {
			return pm[i].ExternalPort < pm[j].ExternalPort
		}
		return pm[i].Protocol < pm[j].Protocol
	})
}
