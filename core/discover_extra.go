package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Helpers de core/discover.go, separados por protocolo para no abultar el
// archivo con el contrato. Nada de aquí se usa fuera de discover.go.

// ===========================================================================
// mDNS / Bonjour (UDP 5353, grupos 224.0.0.251 / ff02::fb)
// ===========================================================================

const (
	mdnsIPv4Group = "224.0.0.251"
	mdnsIPv6Group = "ff02::fb"
	mdnsPort      = 5353

	// mdnsEnumName: "service type enumeration" de RFC 6763 §9 — preguntar su PTR
	// devuelve, como target, los tipos de servicio presentes en la red (no una
	// instancia concreta). Lo tratamos como caso especial en addPTR.
	mdnsEnumName = "_services._dns-sd._udp.local."
)

// mdnsEnumNameTrimmed: mdnsEnumName sin el punto final, para comparar contra los
// nombres ya desempaquetados (que llegan recortados igual).
var mdnsEnumNameTrimmed = strings.TrimSuffix(mdnsEnumName, ".")

// mdnsQueryNames: enumeración dns-sd + tipos de servicio comunes (ver contrato de
// discoverMDNS en discover.go). Todos terminan en "." porque dnsmessage.NewName
// exige nombres canónicos.
var mdnsQueryNames = []string{
	mdnsEnumName,
	"_airplay._tcp.local.",
	"_googlecast._tcp.local.",
	"_raop._tcp.local.",
	"_spotify-connect._tcp.local.",
	"_ipp._tcp.local.",
	"_printer._tcp.local.",
	"_hap._tcp.local.",
	"_homekit._tcp.local.",
	"_companion-link._tcp.local.",
	"_device-info._tcp.local.",
	"_smb._tcp.local.",
	"_afpovertcp._tcp.local.",
	"_ssh._tcp.local.",
	"_sleep-proxy._udp.local.",
	// _rfb._tcp (Screen Sharing/VNC): verificado en la red real que preguntar
	// "_device-info._tcp.local" DIRECTO no contesta nada en macOS, pero
	// "_rfb._tcp.local" sí, y trae de regalo en su adicional el TXT de
	// _device-info (model=/osxvers=) del mismo Mac — ver metadataOnlyService
	// en este archivo. Se pregunta explícito en vez de confiar en que la
	// enumeración dns-sd lo encuentre a tiempo.
	"_rfb._tcp.local.",
}

// mdnsAccumulator junta, de forma segura entre goroutines, todo lo que vamos
// viendo en los paquetes mDNS —propios y ajenos: al estar en el grupo multicast
// vemos también anuncios/respuestas de otros clientes de la red— hasta que se
// arma el resultado final en result().
type mdnsAccumulator struct {
	mu sync.Mutex

	services     map[string][]string // instancia ("Foo._googlecast._tcp.local") -> tipos de servicio que ofrece
	srv          map[string]string   // instancia -> hostname destino (target del SRV)
	txt          map[string][]string // instancia -> registros TXT crudos ("clave=valor")
	addrs        map[string][]string // hostname -> IPs (v4 y/o v6) vistas por A/AAAA
	queriedTypes map[string]bool     // tipos de servicio (de la enumeración) ya repreguntados
	queriedHosts map[string]bool     // hosts ya repreguntados por A/AAAA
}

func newMDNSAccumulator() *mdnsAccumulator {
	return &mdnsAccumulator{
		services:     map[string][]string{},
		srv:          map[string]string{},
		txt:          map[string][]string{},
		addrs:        map[string][]string{},
		queriedTypes: map[string]bool{},
		queriedHosts: map[string]bool{},
	}
}

// runMDNSListener abre un socket multicast (v4 o v6), manda la consulta inicial y
// procesa todo lo que llegue hasta que ctx se cancele. Best-effort: si esta
// familia no se puede abrir (interfaz sin IPv6, multicast bloqueado, permisos...)
// simplemente vuelve sin hacer nada; la otra familia sigue en su propia goroutine.
func runMDNSListener(ctx context.Context, network, group, iface string, acc *mdnsAccumulator) {
	gaddr := &net.UDPAddr{IP: net.ParseIP(group), Port: mdnsPort}
	ifc := interfaceByName(iface)
	conn, err := net.ListenMulticastUDP(network, ifc, gaddr)
	if err != nil && ifc != nil {
		// Fallback to default interface when bound multicast fails (e.g. no IPv6 on ifc).
		ifc = nil
		conn, err = net.ListenMulticastUDP(network, nil, gaddr)
	}
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(65535)
	if ifc != nil {
		if sc, scErr := conn.SyscallConn(); scErr == nil {
			if ctrl := bindControl(ifc.Name); ctrl != nil {
				_ = ctrl(network, "", sc)
			}
		}
		if network == "udp4" {
			_ = ipv4.NewPacketConn(conn).SetMulticastInterface(ifc)
		} else if network == "udp6" {
			_ = ipv6.NewPacketConn(conn).SetMulticastInterface(ifc)
		}
	}

	// ReadFromUDP no toma ctx: lo desbloqueamos cerrando el socket a mano cuando
	// el contexto termine.
	var wg sync.WaitGroup
	defer wg.Wait()
	stop := make(chan struct{})
	defer close(stop)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()

	sendMDNSQuery(conn, gaddr, mdnsQueryNames)

	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // socket cerrado (ctx listo) u otro error de lectura: terminamos
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		acc.handlePacket(data, conn, gaddr)
	}
}

// sendMDNSQuery manda un único paquete con una pregunta PTR por cada nombre.
func sendMDNSQuery(conn *net.UDPConn, dest *net.UDPAddr, names []string) {
	qs := make([]dnsmessage.Question, 0, len(names))
	for _, n := range names {
		name, err := dnsmessage.NewName(n)
		if err != nil {
			continue
		}
		qs = append(qs, dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
	}
	writeDNSQuery(conn, dest, qs)
}

// writeDNSQuery empaqueta y manda un mensaje DNS de solo-preguntas (sin compresión:
// los paquetes son chicos y así evitamos cualquier sutileza del builder).
func writeDNSQuery(conn *net.UDPConn, dest *net.UDPAddr, qs []dnsmessage.Question) {
	if len(qs) == 0 {
		return
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := b.StartQuestions(); err != nil {
		return
	}
	for _, q := range qs {
		if err := b.Question(q); err != nil {
			return
		}
	}
	pkt, err := b.Finish()
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(pkt, dest)
}

// handlePacket interpreta un datagrama mDNS: recorre TODAS las secciones de
// recursos (answer + authority + additional, sin distinguir entre ellas: los
// respondedores "completos" meten SRV+TXT+A/AAAA como adicionales de una sola
// respuesta, por diseño de mDNS/RFC 6763) y acumula lo que entienda.
func (acc *mdnsAccumulator) handlePacket(data []byte, conn *net.UDPConn, dest *net.UDPAddr) {
	var p dnsmessage.Parser
	if _, err := p.Start(data); err != nil {
		return
	}
	if err := p.SkipAllQuestions(); err != nil {
		return
	}
	answers, err := p.AllAnswers()
	if err != nil {
		return
	}
	authorities, err := p.AllAuthorities()
	if err != nil {
		return
	}
	additionals, err := p.AllAdditionals()
	if err != nil {
		return
	}
	for _, res := range answers {
		acc.handleResource(res, conn, dest)
	}
	for _, res := range authorities {
		acc.handleResource(res, conn, dest)
	}
	for _, res := range additionals {
		acc.handleResource(res, conn, dest)
	}
}

func (acc *mdnsAccumulator) handleResource(res dnsmessage.Resource, conn *net.UDPConn, dest *net.UDPAddr) {
	// Solo se recorta el punto final: el ".local" se deja (se necesita intacto
	// para poder re-preguntar por ese nombre más adelante). El recorte "bonito"
	// para el resultado final pasa en result().
	name := strings.TrimSuffix(res.Header.Name.String(), ".")
	switch body := res.Body.(type) {
	case *dnsmessage.PTRResource:
		acc.addPTR(name, strings.TrimSuffix(body.PTR.String(), "."), conn, dest)
	case *dnsmessage.SRVResource:
		acc.addSRV(name, strings.TrimSuffix(body.Target.String(), "."), conn, dest)
	case *dnsmessage.TXTResource:
		acc.addTXT(name, body.TXT)
	case *dnsmessage.AResource:
		acc.addAddr(name, net.IP(body.A[:]).String())
	case *dnsmessage.AAAAResource:
		acc.addAddr(name, net.IP(body.AAAA[:]).String())
	}
}

// addPTR procesa una respuesta PTR. Si `serviceType` es el nombre especial de
// enumeración dns-sd, `target` es en realidad un tipo de servicio nuevo que no
// estaba en nuestra lista fija: lo repreguntamos una vez. Si no, `serviceType` es
// un tipo real y `target` la instancia que lo ofrece.
func (acc *mdnsAccumulator) addPTR(serviceType, target string, conn *net.UDPConn, dest *net.UDPAddr) {
	if serviceType == "" || target == "" {
		return
	}
	if serviceType == mdnsEnumNameTrimmed {
		acc.mu.Lock()
		already := acc.queriedTypes[target]
		acc.queriedTypes[target] = true
		acc.mu.Unlock()
		if !already {
			sendMDNSQuery(conn, dest, []string{ensureTrailingDot(target)})
		}
		return
	}
	acc.mu.Lock()
	acc.services[target] = appendUniqueStr(acc.services[target], serviceType)
	acc.mu.Unlock()
}

// addSRV asocia una instancia a su hostname destino. Si ese host todavía no
// tiene A/AAAA conocida, se pregunta explícitamente (muchos respondedores ya la
// mandan como adicional junto con el SRV, pero no todos lo garantizan).
func (acc *mdnsAccumulator) addSRV(instance, host string, conn *net.UDPConn, dest *net.UDPAddr) {
	if instance == "" || host == "" {
		return
	}
	acc.mu.Lock()
	acc.srv[instance] = host
	_, haveAddr := acc.addrs[host]
	alreadyQueried := acc.queriedHosts[host]
	if !haveAddr && !alreadyQueried {
		acc.queriedHosts[host] = true
	}
	acc.mu.Unlock()

	if haveAddr || alreadyQueried {
		return
	}
	name, err := dnsmessage.NewName(ensureTrailingDot(host))
	if err != nil {
		return
	}
	writeDNSQuery(conn, dest, []dnsmessage.Question{
		{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
		{Name: name, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET},
	})
}

func (acc *mdnsAccumulator) addTXT(instance string, txt []string) {
	if instance == "" {
		return
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	acc.txt[instance] = txt
}

func (acc *mdnsAccumulator) addAddr(host, ip string) {
	if host == "" || ip == "" {
		return
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	for _, existing := range acc.addrs[host] {
		if existing == ip {
			return
		}
	}
	acc.addrs[host] = append(acc.addrs[host], ip)
}

// metadataOnlyService: tipos de servicio que mDNSResponder NO resuelve con su
// propia cadena PTR→SRV→A. Verificado en la red real (iMac .48): preguntar
// "_device-info._tcp.local" directo no contesta NADA, pero cualquier otra
// respuesta sobre el mismo host (en este caso, "_rfb._tcp.local", Screen
// Sharing) trae en su sección adicional, de regalo, un TXT suelto
// "<Nombre>._device-info._tcp.local" con "model="/"osxvers=" — sin PTR ni SRV
// propios. Por eso se correlaciona por nombre base en vez de por la cadena
// normal (ver splitInstance y su uso en result()).
var metadataOnlyService = map[string]bool{
	"_device-info._tcp": true,
}

// instanceTypeRe separa un nombre de instancia DNS-SD con su ".local" en el
// nombre base (humano) y el tipo de servicio: "iMac de Estudio._device-info._tcp.local"
// → ("iMac de Estudio", "_device-info._tcp"). Nota: Name.String() (ver doc de
// dnsmessage) NO escapa los bytes decodificados, así que el nombre base puede
// traer espacios o comillas literales ("iMac de Estudio", `75" QLED`), no
// secuencias "\032"/"\"" — eso es solo cómo lo muestra `dig`.
var instanceTypeRe = regexp.MustCompile(`^(.*)\.(_[a-zA-Z0-9-]+\._(?:tcp|udp))\.local$`)

func splitInstance(name string) (base, svcType string, ok bool) {
	m := instanceTypeRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// mdnsTXTFields: los campos que nos importan de un TXT mDNS, ya sean los de
// la propia instancia o los de un servicio "solo metadata" asociado (ver
// metadataOnlyService). Claves case-insensitive, como pide RFC 6763.
type mdnsTXTFields struct {
	model        string // "model"/"md" (Chromecast/HomeKit), "am" (AirPlay/raop), "rpMd" (companion-link)
	manufacturer string // "manufacturer"/"integrator"
	osVersion    string // "osxvers" (Darwin, crudo: "24")
	friendlyName string // "fn" (Chromecast/googlecast)
}

func parseMDNSTXT(txt []string) mdnsTXTFields {
	var f mdnsTXTFields
	for _, kv := range txt {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "md", "model", "am", "rpmd":
			if f.model == "" {
				f.model = v
			}
		case "manufacturer", "integrator":
			if f.manufacturer == "" {
				f.manufacturer = v
			}
		case "osxvers":
			if f.osVersion == "" {
				f.osVersion = v
			}
		case "fn":
			if f.friendlyName == "" {
				f.friendlyName = v
			}
		}
	}
	return f
}

// fillMissing copia de `other` los campos que todavía están vacíos: la
// evidencia de la propia instancia manda, `other` (el TXT huérfano de
// _device-info, si lo hay) solo rellena huecos.
func (f *mdnsTXTFields) fillMissing(other mdnsTXTFields) {
	if f.model == "" {
		f.model = other.model
	}
	if f.manufacturer == "" {
		f.manufacturer = other.manufacturer
	}
	if f.osVersion == "" {
		f.osVersion = other.osVersion
	}
	if f.friendlyName == "" {
		f.friendlyName = other.friendlyName
	}
}

// instanceNamePriority: de qué servicios sale un buen nombre visible para el
// equipo (menor = mejor). Los de _device-info/_companion-link/_airplay son el
// nombre que el usuario le puso ("iMac de Estudio", `75" QLED`).
var instanceNamePriority = map[string]int{
	"_device-info._tcp":    0,
	"_companion-link._tcp": 1,
	"_airplay._tcp":        2,
	"_raop._tcp":           3,
	"_rfb._tcp":            4,
	"_smb._tcp":            5,
	"_afpovertcp._tcp":     5,
	"_hap._tcp":            6,
	"_homekit._tcp":        6,
	"_ipp._tcp":            6,
	"_printer._tcp":        6,
	"_octoprint._tcp":      6,
	"_sonos._tcp":          6,
	"_home-assistant._tcp": 6,
}

// fnPriority: el "fn" (friendly name) del TXT de Chromecast es tan bueno como
// el nombre AirPlay.
const fnPriority = 2

// otherInstancePriority: cualquier otro servicio que no esté vetado.
const otherInstancePriority = 8

// noNameServices: servicios cuyo nombre de instancia es un identificador
// interno y no un nombre (medido: _spotify-connect "d02281698a64…",
// "SpotifyConnect #3"; _matter "ACDC919AAFED4D50-014F…"; _teamviewer "0").
var noNameServices = map[string]bool{
	"_spotify-connect._tcp": true,
	"_teamviewer._tcp":      true,
	"_matter._tcp":          true,
	"_matterc._udp":         true,
	"_matterd._udp":         true,
	"_adb._tcp":             true,
	"_adb-tls-connect._tcp": true,
	"_adb-tls-pairing._tcp": true,
	"_nearbypresence._tcp":  true,
	"_googlecast._tcp":      true, // la instancia es "Chromecast-<hex>": el nombre va en fn
	"_googlezone._tcp":      true,
	"_googlerpc._tcp":       true,
	"_amzn-wplay._tcp":      true,
	"_meshcop._udp":         true,
	"_trel._udp":            true,
	"_sleep-proxy._udp":     true, // "70-35-60-63.1 iMac": prefijo técnico
	"_device-info._udp":     true,
	"_workstation._tcp":     true, // "nombre [aa:bb:cc:dd:ee:ff]"
}

var (
	hexTypeRe       = regexp.MustCompile(`^_[0-9A-Fa-f]{8,}\._(tcp|udp)$`)
	hexIDRe         = regexp.MustCompile(`^[0-9A-Fa-f]{8,}([-:_][0-9A-Fa-f]+)*$`)
	uuidRe          = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	digitsRe        = regexp.MustCompile(`^\d+$`)
	spotifyNameRe   = regexp.MustCompile(`(?i)^spotify ?connect( #\d+)?$`)
	raopMACPrefixRe = regexp.MustCompile(`^[0-9A-Fa-f]{12}@`)
)

// isJunkInstanceName: identificadores que no sirven como nombre visible.
func isJunkInstanceName(name string) bool {
	name = strings.TrimSpace(name)
	return name == "" || isHex12(name) || hexIDRe.MatchString(name) || uuidRe.MatchString(name) ||
		digitsRe.MatchString(name) || spotifyNameRe.MatchString(name)
}

// humanInstanceName: el nombre visible que aporta una instancia DNS-SD de tipo
// svcType, con su prioridad, u ok=false si ese servicio no da nombres o el
// nombre es basura. En _raop quita el prefijo "AABBCCDDEEFF@".
func humanInstanceName(base, svcType string) (name string, prio int, ok bool) {
	if noNameServices[svcType] || hexTypeRe.MatchString(svcType) {
		return "", 0, false
	}
	name = strings.TrimSpace(base)
	if svcType == "_raop._tcp" {
		name = raopMACPrefixRe.ReplaceAllString(name, "")
	}
	if isJunkInstanceName(name) {
		return "", 0, false
	}
	prio, known := instanceNamePriority[svcType]
	if !known {
		prio = otherInstancePriority
	}
	return name, prio, true
}

// offerName: guarda name como nombre visible del host si es mejor (menor
// prioridad) que el que ya tenía; y lo suma a Instances.
func (info *MDNSInfo) offerName(name string, prio int) {
	info.Instances = appendUniqueStr(info.Instances, name)
	if info.Name == "" || prio < info.namePrio {
		info.Name, info.namePrio = name, prio
	}
}

// result arma el mapa final IP -> MDNSInfo encadenando PTR (instancia -> tipos) ->
// SRV (instancia -> host) -> A/AAAA (host -> IPs), más lo que diga el TXT
// (modelo, osxvers, manufacturer, nombre de instancia) — fusionando también el
// TXT huérfano de un servicio "solo metadata" del mismo nombre base, si lo hay
// (ver metadataOnlyService). Acá sí se recorta el ".local" (vía trimLocal, de
// scan.go) para que Hostname y Services queden legibles ("Living-Room-TV",
// "_googlecast._tcp").
func (acc *mdnsAccumulator) result() map[string]MDNSInfo {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	// TXT de servicios "solo metadata", indexado por nombre base: nunca tienen
	// su propio SRV (no aparecen en acc.services), así que se enlazan por
	// nombre en vez de por la cadena PTR→SRV→A normal.
	metaByBase := map[string]mdnsTXTFields{}
	for instance, txt := range acc.txt {
		base, svcType, ok := splitInstance(instance)
		if !ok || !metadataOnlyService[svcType] {
			continue
		}
		metaByBase[base] = parseMDNSTXT(txt)
	}

	out := map[string]MDNSInfo{}
	for instance, types := range acc.services {
		host, ok := acc.srv[instance]
		if !ok {
			continue // sin SRV no hay a qué IP colgar este servicio
		}
		ips := acc.addrs[host]
		if len(ips) == 0 {
			continue // sin A/AAAA no hay IP (no debería pasar: addSRV ya repreguntó)
		}

		fields := parseMDNSTXT(acc.txt[instance])
		base, svcType, hasBase := splitInstance(instance)
		sawDeviceInfo := false
		if hasBase {
			if meta, ok := metaByBase[base]; ok {
				fields.fillMissing(meta)
				sawDeviceInfo = true
			}
		}

		hostname := trimLocal(host)
		for _, ip := range ips {
			info := out[ip]
			if info.Hostname == "" {
				info.Hostname = hostname
			}
			for _, t := range types {
				info.Services = appendUniqueStr(info.Services, trimLocal(t))
			}
			if sawDeviceInfo {
				// El TXT llegó de regalo, pero el servicio existe: que
				// recognize() lo vea como cualquier otro _device-info._tcp.
				info.Services = appendUniqueStr(info.Services, "_device-info._tcp")
			}
			if hasBase {
				if name, prio, ok := humanInstanceName(base, svcType); ok {
					info.offerName(name, prio)
				}
			}
			if fn := strings.TrimSpace(fields.friendlyName); fn != "" && !isJunkInstanceName(fn) {
				info.offerName(fn, fnPriority)
			}
			if fields.model != "" && info.Model == "" {
				info.Model = fields.model
			}
			if fields.osVersion != "" && info.OSVersion == "" {
				info.OSVersion = fields.osVersion
			}
			if fields.manufacturer != "" && info.Manufacturer == "" {
				info.Manufacturer = fields.manufacturer
			}
			out[ip] = info
		}
	}
	return out
}

func ensureTrailingDot(s string) string {
	if strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

func appendUniqueStr(list []string, s string) []string {
	for _, existing := range list {
		if existing == s {
			return list
		}
	}
	return append(list, s)
}

// ===========================================================================
// SSDP / UPnP (UDP 239.255.255.250:1900 + HTTP GET de la descripción XML)
// ===========================================================================

const ssdpAddr = "239.255.255.250:1900"

// ssdpSearchTargets: los dos ST que pide el contrato más "ssdp:all" de bonus
// (algunos dispositivos, sobre todo TVs, solo contestan a ese).
var ssdpSearchTargets = []string{"ssdp:rootdevice", "upnp:rootdevice", "ssdp:all"}

// runSSDPDiscovery hace el trabajo real de discoverSSDP (ver ese comentario en
// discover.go para el porqué del timeout de la descarga XML independiente de la
// ventana de escucha).
func runSSDPDiscovery(ctx context.Context, timeout time.Duration, iface string) map[string]SSDPInfo {
	out := map[string]SSDPInfo{}

	// No hace falta unirse al grupo multicast para SSDP: las respuestas a
	// M-SEARCH llegan por unicast a nuestro puerto efímero.
	lc := newListenConfig(iface, nil)
	conn, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return out
	}
	defer conn.Close()

	if ifc := interfaceByName(iface); ifc != nil {
		_ = ipv4.NewPacketConn(conn).SetMulticastInterface(ifc)
	}

	dest, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return out
	}

	stop := make(chan struct{})
	var closerWG sync.WaitGroup
	closerWG.Add(1)
	go func() {
		defer closerWG.Done()
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	defer closerWG.Wait()
	defer close(stop)

	// MX: cuánto deben esperar (como máximo) los dispositivos antes de
	// responder, para repartir las respuestas. La mitad de la ventana de
	// escucha (1-3 s): con MX = ventana entera, una TV que contesta al final
	// queda fuera y se pierde su descripción XML.
	mx := int(timeout.Seconds() / 2)
	if mx < 1 {
		mx = 1
	} else if mx > 3 {
		mx = 3
	}
	// Cada M-SEARCH va dos veces (UDP se pierde; lo recomienda UPnP).
	for round := 0; round < 2; round++ {
		if round > 0 {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return out
			}
		}
		for _, st := range ssdpSearchTargets {
			req := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"MAN: \"ssdp:discover\"\r\n"+
				"MX: %d\r\n"+
				"ST: %s\r\n\r\n", ssdpAddr, mx, st)
			_, _ = conn.WriteTo([]byte(req), dest)
		}
	}

	httpClient := ssdpClientFor(iface)

	var (
		mu      sync.Mutex
		fetched = map[string]bool{} // LOCATION ya pedida (varios ST pueden traer la misma)
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 16)
	)

	buf := make([]byte, 65535)
	for {
		n, raddr, err := conn.ReadFrom(buf)
		if err != nil {
			break // socket cerrado (ctx listo) u otro error: terminamos la escucha
		}
		ua, ok := raddr.(*net.UDPAddr)
		if !ok {
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		ip := ua.IP.String()

		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
		if err != nil {
			continue
		}
		server := resp.Header.Get("SERVER")
		location := strings.TrimSpace(resp.Header.Get("LOCATION"))
		resp.Body.Close()

		mu.Lock()
		info := out[ip]
		if info.Server == "" {
			info.Server = server
		}
		out[ip] = info
		skip := location == "" || fetched[location]
		if location != "" {
			fetched[location] = true
		}
		mu.Unlock()
		if skip {
			continue
		}

		wg.Add(1)
		go func(ip, location string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			desc, ok := fetchSSDPDescriptionWithClient(ctx, httpClient, location)
			if !ok {
				return
			}
			mu.Lock()
			info := out[ip]
			info.FriendlyName = firstNonEmpty(info.FriendlyName, desc.FriendlyName)
			info.Manufacturer = firstNonEmpty(info.Manufacturer, desc.Manufacturer)
			info.ModelName = firstNonEmpty(info.ModelName, desc.ModelName)
			info.ModelNumber = firstNonEmpty(info.ModelNumber, desc.ModelNumber)
			info.DeviceType = betterDeviceType(info.DeviceType, desc.DeviceType)
			// Una IP puede publicar varios dispositivos UPnP (la TV: 4-5); para
			// que el identificador sea el mismo en cada escaneo, se queda el
			// menor, no el primero que conteste.
			if desc.UDN != "" && (info.UDN == "" || desc.UDN < info.UDN) {
				info.UDN = desc.UDN
			}
			out[ip] = info
			mu.Unlock()
		}(ip, location)
	}

	wg.Wait() // esperar las descargas de XML en vuelo antes de devolver el mapa
	return out
}

// betterDeviceType: una IP puede publicar varios dispositivos UPnP (la TV
// Samsung: MediaRenderer, dialreceiver, ScreenMirroring...). Se queda con el
// que dice qué aparato es (ver ssdpTypeHints) y, si ninguno lo dice, con el
// primero.
func betterDeviceType(cur, cand string) string {
	rank := func(dt string) int {
		for i, re := range ssdpTypeHintRe {
			if dt != "" && re.MatchString(dt) {
				return i
			}
		}
		return len(ssdpTypeHintRe)
	}
	if cur == "" || rank(cand) < rank(cur) {
		return cand
	}
	return cur
}

// ssdpDescription: subconjunto de la descripción XML UPnP que nos interesa.
type ssdpDescription struct {
	FriendlyName string
	Manufacturer string
	ModelName    string
	ModelNumber  string
	DeviceType   string
	UDN          string
}

// fetchSSDPDescription baja y parsea el XML de descripción UPnP de `location`.
// Timeout corto y propio (ver discoverSSDP): independiente de la ventana de
// escucha.
var ssdpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:             nil, // el XML UPnP es siempre LAN; no usar HTTP_PROXY
		DisableKeepAlives: true,
	},
	Timeout: 3 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func ssdpClientFor(iface string) *http.Client {
	if strings.TrimSpace(iface) == "" {
		return ssdpClient
	}
	return &http.Client{
		Transport: newLANTransport(iface),
		Timeout:   3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func fetchSSDPDescription(parentCtx context.Context, location string) (ssdpDescription, bool) {
	return fetchSSDPDescriptionWithClient(parentCtx, ssdpClient, location)
}

func fetchSSDPDescriptionWithClient(parentCtx context.Context, client *http.Client, location string) (ssdpDescription, bool) {
	ctx, cancel := context.WithTimeout(parentCtx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return ssdpDescription{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return ssdpDescription{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ssdpDescription{}, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB de tope, por si acaso
	if err != nil {
		return ssdpDescription{}, false
	}

	var doc struct {
		Device struct {
			DeviceType   string `xml:"deviceType"`
			FriendlyName string `xml:"friendlyName"`
			Manufacturer string `xml:"manufacturer"`
			ModelName    string `xml:"modelName"`
			ModelNumber  string `xml:"modelNumber"`
			UDN          string `xml:"UDN"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return ssdpDescription{}, false
	}
	return ssdpDescription{
		FriendlyName: strings.TrimSpace(doc.Device.FriendlyName),
		Manufacturer: strings.TrimSpace(doc.Device.Manufacturer),
		ModelName:    strings.TrimSpace(doc.Device.ModelName),
		ModelNumber:  strings.TrimSpace(doc.Device.ModelNumber),
		DeviceType:   strings.TrimSpace(doc.Device.DeviceType),
		UDN:          strings.TrimSpace(doc.Device.UDN),
	}, true
}

// ===========================================================================
// NetBIOS (UDP 137, "node status" / NBSTAT — RFC 1001 §14 + RFC 1002 §4.2.1)
// ===========================================================================

const netbiosPort = "137"

// runNetBIOSQueries reparte las consultas en paralelo, acotadas por un semáforo;
// cada una tiene su propio timeout corto (no uno compartido) para que una IP
// muerta no le robe tiempo a las demás. Deja de *lanzar* consultas nuevas en
// cuanto ctx se cancela, pero espera a que las ya en vuelo terminen.
func runNetBIOSQueries(ctx context.Context, ips []string, iface string) map[string]NetBIOSInfo {
	out := map[string]NetBIOSInfo{}
	if len(ips) == 0 {
		return out
	}

	const (
		perHostTimeout = 700 * time.Millisecond // igual que DefaultPortTimeout, por consistencia
		concurrency    = 64
	)
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, concurrency)
	)
dispatch:
	for _, ip := range ips {
		select {
		case <-ctx.Done():
			break dispatch
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			if info, ok := queryNetBIOSOne(ctx, ip, iface, perHostTimeout); ok {
				mu.Lock()
				out[ip] = info
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return out
}

// queryNetBIOSOne manda una consulta NBSTAT a una IP y espera (acotado por
// timeout) su respuesta.
func queryNetBIOSOne(ctx context.Context, ip, iface string, timeout time.Duration) (NetBIOSInfo, bool) {
	d := newDialer(iface, timeout)
	conn, err := d.DialContext(ctx, "udp4", net.JoinHostPort(ip, netbiosPort))
	if err != nil {
		return NetBIOSInfo{}, false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write(buildNetBIOSStatusQuery(0x1337)); err != nil {
		return NetBIOSInfo{}, false
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return NetBIOSInfo{}, false
	}
	return parseNetBIOSStatus(buf[:n])
}

// buildNetBIOSStatusQuery arma el paquete NBSTAT (node status request): una
// cabecera al estilo DNS seguida del nombre comodín "*" codificado en
// half-ASCII.
func buildNetBIOSStatusQuery(txID uint16) []byte {
	buf := make([]byte, 0, 50)
	buf = appendU16(buf, txID)
	buf = appendU16(buf, 0x0000) // flags: query, opcode 0 (consulta estándar)
	buf = appendU16(buf, 1)      // QDCOUNT = 1
	buf = appendU16(buf, 0)      // ANCOUNT
	buf = appendU16(buf, 0)      // NSCOUNT
	buf = appendU16(buf, 0)      // ARCOUNT
	buf = append(buf, 0x20)      // longitud del nombre codificado (32 bytes)
	buf = append(buf, netbiosEncodeWildcard()...)
	buf = append(buf, 0x00)      // fin de nombre (sin dominios, como en DNS)
	buf = appendU16(buf, 0x0021) // QTYPE = NBSTAT
	buf = appendU16(buf, 0x0001) // QCLASS = IN
	return buf
}

// netbiosEncodeWildcard codifica el nombre comodín de 16 bytes ('*' + 15 ceros)
// en "half-ASCII" (RFC 1001 §14.1): cada nibble se convierte en una letra
// 'A'..'P'. Da el clásico "CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" de 32 bytes que
// usan nbtscan/nmblookup para este mismo propósito.
func netbiosEncodeWildcard() []byte {
	var raw [16]byte
	raw[0] = '*' // el resto queda en 0x00: es el nombre especial de "cualquiera"
	enc := make([]byte, 32)
	for i, b := range raw {
		enc[i*2] = 'A' + (b >> 4)
		enc[i*2+1] = 'A' + (b & 0x0F)
	}
	return enc
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// parseNetBIOSStatus interpreta la respuesta NBSTAT: el nombre de equipo (la
// primera entrada única de tipo workstation 0x00 o file-server 0x20 que
// encuentre, no-grupo) y la MAC, que va justo después de la tabla de nombres.
func parseNetBIOSStatus(data []byte) (NetBIOSInfo, bool) {
	const headerLen = 12 // igual que una cabecera DNS: txid+flags+4 contadores
	off := headerLen

	if len(data) < off+1 {
		return NetBIOSInfo{}, false
	}
	nameLen := int(data[off])
	off++
	if nameLen != 0x20 || len(data) < off+nameLen+1 {
		return NetBIOSInfo{}, false // no es el nombre comodín que esperábamos
	}
	off += nameLen
	if data[off] != 0x00 { // terminador de nombre
		return NetBIOSInfo{}, false
	}
	off++

	// RR_TYPE(2) + RR_CLASS(2) + TTL(4) + RDLENGTH(2) = 10 bytes que no usamos.
	if len(data) < off+10 {
		return NetBIOSInfo{}, false
	}
	off += 10

	if len(data) < off+1 {
		return NetBIOSInfo{}, false
	}
	numNames := int(data[off])
	off++

	var workstation, fileserver, anyUnique string
	for i := 0; i < numNames; i++ {
		if len(data) < off+18 { // 15 (nombre) + 1 (sufijo) + 2 (flags)
			return NetBIOSInfo{}, false
		}
		rawName := string(data[off : off+15])
		suffix := data[off+15]
		flags := uint16(data[off+16])<<8 | uint16(data[off+17])
		off += 18

		isGroup := flags&0x8000 != 0 // bit G: nombre de grupo, no de un equipo puntual
		name := strings.TrimRight(rawName, " ")
		if name == "" || isGroup {
			continue
		}
		switch suffix {
		case 0x00: // Workstation Service
			if workstation == "" {
				workstation = name
			}
		case 0x20: // Server Service
			if fileserver == "" {
				fileserver = name
			}
		}
		if anyUnique == "" {
			anyUnique = name
		}
	}

	info := NetBIOSInfo{Name: firstNonEmpty(workstation, fileserver, anyUnique)}
	if len(data) >= off+6 {
		mac := normalizeMAC(hex.EncodeToString(data[off : off+6]))
		if mac != "" && mac != "00:00:00:00:00:00" && mac != "ff:ff:ff:ff:ff:ff" {
			info.MAC = mac
		}
	}
	if info.Name == "" && info.MAC == "" {
		return NetBIOSInfo{}, false
	}
	return info, true
}
