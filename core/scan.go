package core

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scan descubre, reconoce y (opcionalmente) escanea puertos de la red local.
// Es la entrada única del motor; la usan el agente y la app por igual.
func Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	ipNet, self, err := localNetworkOnInterface(opts.CIDR, opts.BindInterface)
	if err != nil {
		return nil, err
	}
	start := time.Now()

	// routed: red de delante del router (el "anillo exterior" de la Fase 5,
	// ver ScanOptions.Routed). opts.CIDR == "" nunca es Routed: localNetwork
	// ya la sacó de una interfaz propia en ese caso, así que ni vale la pena
	// llamar a isRoutedNetwork (ahorra un net.Interfaces() en el camino
	// normal, que es justo este).
	routed := opts.Routed
	if !routed && opts.CIDR != "" {
		routed = isRoutedNetwork(ipNet)
	}

	// A partir de aquí varias etapas corren a la vez y más de una puede
	// llamar a OnProgress (antes solo lo hacía el barrido mientras el resto
	// esperaba en wg.Wait()); esta envoltura con mutex es la que hace cierta
	// la promesa del contrato de ScanOptions ("el motor serializa las
	// llamadas, el callback no necesita ser seguro para hilos") ahora que ya
	// no hay una sola etapa concurrente invocándolo.
	if opts.OnProgress != nil {
		var pmu sync.Mutex
		real := opts.OnProgress
		opts.OnProgress = func(stage string, done, total int) {
			pmu.Lock()
			defer pmu.Unlock()
			real(stage, done, total)
		}
	}
	discoverTimeout := orDur(opts.DiscoverTimeout, DefaultDiscoverTimeout)

	var (
		live    map[string][]string
		nbr     map[string]string
		mdns    map[string]MDNSInfo
		ssdp    map[string]SSDPInfo
		netbios map[string]NetBIOSInfo
		wg      sync.WaitGroup
	)
	// sweepDone avisa a la rama de NetBIOS (ver abajo) que ya puede leer
	// `live`: en redes grandes necesita esperar al barrido, y leerla desde
	// otra goroutine mientras el barrido todavía escribe sería una carrera de
	// datos. close() aquí sincroniza esa lectura con la última escritura.
	sweepDone := make(chan struct{})

	// Tuya escucha durante todo el escaneo (ver tuyaListener) — salvo en una
	// red Routed: es un broadcast UDP a 255.255.255.255, que no cruza el
	// router, así que escucharlo ahí nunca encontraría nada. tuya es
	// nil-safe (ver tuya.go): el resto del código no necesita otra rama.
	var tuya *tuyaListener
	if !routed {
		tuya = startTuyaWithIface(ctx, opts.BindInterface)
	}
	defer tuya.stop()

	// Barrido (TCP+ICMP, puebla ARP de paso) y, en paralelo, mDNS, SSDP y
	// NetBIOS. En una red Routed, mDNS/SSDP se saltan (multicast, no cruza
	// el router) y tampoco se lee la tabla ARP/NDP (ídem): solo NetBIOS
	// sigue, porque sus consultas son unicast.
	wg.Add(4)
	go func() {
		defer wg.Done()
		defer close(sweepDone)
		live = sweepLiveness(ctx, ipNet, opts)
		if !opts.SkipNeighborTable && !routed {
			nbr = neighborTable()
		}
		// Terminado el barrido, lo que falta de esta fase es esperar nombres
		// y anuncios (NetBIOS, mDNS, SSDP, Tuya).
		if opts.OnProgress != nil {
			opts.OnProgress("names", 0, 0)
		}
	}()
	go func() {
		defer wg.Done()
		if routed {
			return
		}
		dctx, cancel := withTimeout(ctx, discoverTimeout)
		defer cancel()
		mdns = discoverMDNSWithIface(dctx, discoverTimeout, opts.BindInterface)
	}()
	go func() {
		defer wg.Done()
		if routed {
			return
		}
		dctx, cancel := withTimeout(ctx, discoverTimeout)
		defer cancel()
		ssdp = discoverSSDPWithIface(dctx, discoverTimeout, opts.BindInterface)
	}()
	go func() {
		defer wg.Done()
		// NetBIOS a TODAS las IPs de la subred si no son demasiadas (254 en
		// un /24 de serie: medido, 2.5 s) para que corra junto con el resto
		// del descubrimiento sin esperar al barrido. En redes más grandes
		// (>1024 hosts) consultar cada IP sale carísimo, así que ahí nos
		// limitamos a las que el barrido ya encontró vivas (netbiosTargets);
		// por eso hace falta esperar a que termine (sweepDone) SOLO en ese
		// caso: leer `live` antes de esperarlo sería una carrera de datos —
		// y ojo, eso incluye el simple hecho de EVALUARLA para pasarla como
		// argumento, aunque netbiosTargets no la toque en la rama chica (lo
		// vio -race con una red de prueba de 2 hosts: la rama chica, que no
		// esperaba nada, sí leía `live` igual). Por eso la rama chica ni
		// siquiera llama a netbiosTargets con `live`: usa `hosts` tal cual,
		// que es exactamente lo que esa función devolvería ahí.
		hosts := hostsInNet(ipNet)
		var targets []string
		if len(hosts) > netbiosFullScanLimit {
			select {
			case <-sweepDone:
				targets = netbiosTargets(hosts, live)
			case <-ctx.Done():
				// Cancelado antes de que terminara el barrido: leer `live`
				// aquí sería la misma carrera, y ya no hay nada que consultar.
				return
			}
		} else {
			targets = hosts
		}
		netbios = queryNetBIOSWithIface(ctx, targets, discoverTimeout, opts.BindInterface)
	}()
	wg.Wait()

	// Conjunto de dispositivos, unión de todas las fuentes (una IP puede venir solo
	// de mDNS/SSDP aunque no tenga entrada ARP: así funciona sin MAC en Android).
	devs := map[string]*Device{}
	get := func(ip string) *Device {
		d := devs[ip]
		if d == nil {
			d = &Device{IP: ip, LastSeen: time.Now()}
			devs[ip] = d
		}
		return d
	}
	for ip, sources := range live {
		d := get(ip)
		for _, s := range sources {
			addSource(d, s)
		}
	}
	// El propio equipo que escanea: siempre está, aunque no se responda a sí
	// mismo por mDNS. Se marca para que el reconocimiento lo sepa — salvo en
	// una red Routed, donde no tenemos una interfaz de verdad ahí: "self"
	// solo sería lo que el llamador puso en el host del CIDR (p. ej. la IP
	// de red, ".0", si pasó "192.168.100.0/24"), no un equipo real.
	validSelf := self != nil && ipNet.Contains(self) && !isNetworkAddr(self, ipNet)
	if !routed && validSelf {
		me := get(self.String())
		addSource(me, "self")
		setExtra(me, "self", "true")
		for k, v := range selfExtra() {
			setExtra(me, k, v)
		}
	}
	for ip, mac := range nbr {
		if net.ParseIP(ip).To4() == nil {
			continue // los IPv6 se cuelgan de su dueño más abajo
		}
		if ipNet.Contains(net.ParseIP(ip)) {
			d := get(ip)
			d.MAC = mac
			addSource(d, "arp")
		}
	}
	for ip := range mdns {
		if ipNet.Contains(net.ParseIP(ip)) {
			addSource(get(ip), "mdns")
		}
	}
	for ip := range ssdp {
		if ipNet.Contains(net.ParseIP(ip)) {
			addSource(get(ip), "ssdp")
		}
	}
	for ip := range netbios {
		if ipNet.Contains(net.ParseIP(ip)) {
			addSource(get(ip), "netbios")
		}
	}
	for ip := range tuya.snapshot() {
		if ipNet.Contains(net.ParseIP(ip)) {
			addSource(get(ip), "tuya")
		}
	}

	// Último candado antes de enriquecer: cada fuente de arriba ya filtra
	// por ipNet.Contains al insertar, así que en el camino normal esto no
	// quita nada; existe para que un escaneo Routed JAMÁS pueda devolver un
	// equipo del anillo interior así alguna fuente futura olvidara filtrar
	// (ver filterToNet y su prueba unitaria).
	filterToNet(devs, ipNet)

	// Enriquecer cada dispositivo.
	ips := make([]string, 0, len(devs))
	for ip := range devs {
		ips = append(ips, ip)
	}
	enrich(ctx, devs, ips, nbr, mdns, ssdp, netbios, tuya, opts)

	// Resultado ordenado por IP.
	gw := opts.Gateway
	if gw == "" && !routed {
		// defaultGateway() lee la ruta por defecto DE ESTE EQUIPO: en una
		// red Routed sería la del anillo interior (la nuestra), no la de la
		// red que se pidió escanear — mejor vacío que un gateway equivocado.
		gw = defaultGateway()
	}
	selfForResult := ""
	if !routed && validSelf {
		selfForResult = self.String()
	}
	result := &ScanResult{
		Subnet:    ipNet.String(),
		Self:      selfForResult,
		Gateway:   gw,
		Devices:   make([]Device, 0, len(ips)),
		StartedAt: start,
		Duration:  time.Since(start),
	}
	sort.Slice(ips, func(i, j int) bool { return ipLess(ips[i], ips[j]) })
	for _, ip := range ips {
		result.Devices = append(result.Devices, *devs[ip])
	}
	return result, nil
}

func isNetworkAddr(ip net.IP, ipNet *net.IPNet) bool {
	if ip == nil || ipNet == nil {
		return false
	}
	ip4 := ip.To4()
	net4 := ipNet.IP.To4()
	if ip4 == nil || net4 == nil {
		return false
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 || ones >= 31 {
		return false
	}
	return ip4.Mask(ipNet.Mask).Equal(ip4)
}

// enrich rellena MAC/vendor, hostname, servicios, puertos e identidad, en paralelo.
func enrich(ctx context.Context, devs map[string]*Device, ips []string, nbr map[string]string,
	mdns map[string]MDNSInfo, ssdp map[string]SSDPInfo, netbios map[string]NetBIOSInfo,
	tuya *tuyaListener, opts ScanOptions) {

	ports := opts.Ports
	if ports == nil {
		ports = DefaultPorts
	}
	conc := orInt(opts.Concurrency, DefaultConcurrency)
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	httpClient := lanClientFor(opts.BindInterface)

	var pmu sync.Mutex
	done := 0
	progress := func(finished bool) {
		if opts.OnProgress == nil {
			return
		}
		pmu.Lock()
		defer pmu.Unlock()
		if finished {
			done++
		}
		opts.OnProgress("identify", done, len(ips))
	}
	progress(false) // 0 de N

	for _, ip := range ips {
		d := devs[ip]
		// MAC/vendor
		if d.MAC == "" {
			if m := nbr[ip]; m != "" {
				d.MAC = m
			} else if nb, ok := netbios[ip]; ok && nb.MAC != "" {
				d.MAC = nb.MAC
			}
		}
		if d.MAC != "" {
			d.Vendor = vendorForMAC(d.MAC)
		}
		// Nombre NetBIOS, si hay: se usa más abajo (prioridad de hostname) y
		// también para limpiar el sufijo de conflicto de mDNS ("-N", ver el
		// bloque de mDNS a continuación).
		netbiosName := ""
		if nb, ok := netbios[ip]; ok {
			netbiosName = nb.Name
		}
		// hostname + servicios (mDNS > NetBIOS > PTR). Un hostname basura
		// (none-2, android-7, una MAC de 12 hex sin separadores de "nombre")
		// se descarta en vez de mostrarse. Si el de mDNS trae el sufijo de
		// conflicto "-N" ("DESKTOP-ABC1234-2") y la base calza con el nombre
		// NetBIOS, se prefiere la base limpia.
		if info, ok := mdns[ip]; ok {
			name := cleanHostname(trimLocal(info.Hostname))
			if name != "" {
				if base := trimConflictSuffix(name); netbiosName != "" && strings.EqualFold(base, netbiosName) {
					name = base
				}
				d.Hostname = name
			}
			for _, s := range info.Services {
				d.Services = append(d.Services, Service{Proto: "mdns", Name: s})
			}
			if info.Model != "" {
				setExtra(d, "mdns:model", info.Model)
			}
			if info.OSVersion != "" {
				setExtra(d, "mdns:os", info.OSVersion)
			}
			if info.Manufacturer != "" {
				setExtra(d, "mdns:manufacturer", info.Manufacturer)
			}
			if info.Name != "" && !strings.EqualFold(info.Name, d.Hostname) {
				setExtra(d, "mdns:name", info.Name)
			}
		}
		if s, ok := ssdp[ip]; ok {
			// Cada campo SSDP por separado en Extra, para que el reconocimiento
			// no confunda un deviceType o una cabecera SERVER con un modelo.
			for k, v := range map[string]string{
				"ssdp:friendlyName": s.FriendlyName,
				"ssdp:modelName":    s.ModelName,
				"ssdp:modelNumber":  s.ModelNumber,
				"ssdp:manufacturer": s.Manufacturer,
				"ssdp:deviceType":   s.DeviceType,
				"ssdp:server":       s.Server,
				"ssdp:udn":          s.UDN,
			} {
				if v != "" {
					setExtra(d, k, v)
				}
			}
			d.Services = append(d.Services, Service{Proto: "ssdp", Name: firstNonEmpty(s.ModelName, s.FriendlyName, s.DeviceType, s.Server), Info: s.Manufacturer})
		}
		if netbiosName != "" {
			if d.Hostname == "" {
				d.Hostname = netbiosName
			}
			d.Services = append(d.Services, Service{Proto: "netbios", Name: netbiosName})
		}
		// puertos + fingerprint HTTP + PTR + reconocimiento (concurrente)
		select {
		case <-ctx.Done():
			progress(true)
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(d *Device) {
			defer wg.Done()
			defer func() { <-sem }()
			if len(ports) > 0 {
				pctx, cancel := withTimeout(ctx, 20*time.Second)
				d.OpenPorts = scanPortsWithIface(pctx, d.IP, ports, orDur(opts.PortTimeout, DefaultPortTimeout), opts.BindInterface)
				cancel()
			}
			// APIs propias del aparato (Samsung Tizen, Roku, Chromecast) y
			// título/Server HTTP genérico: ver core/httpfp.go. Necesita
			// OpenPorts ya lleno; ≤2s propios, no le resta tiempo al resto.
			fingerprintHTTPWithClient(ctx, d, httpClient)
			if d.Hostname == "" {
				if h := reverseDNS(ctx, d.IP); h != "" {
					d.Hostname = cleanHostname(trimLocal(h))
				}
			}
			// Correos que algunos equipos meten en su nombre (el friendlyName
			// UPnP de Windows Media Player trae la cuenta del usuario): todo
			// esto llega entero a la app, así que se tapan en la fuente.
			for k, v := range d.Extra {
				d.Extra[k] = redactEmails(v)
			}
			for i := range d.Services {
				d.Services[i].Name = redactEmails(d.Services[i].Name)
				d.Services[i].Info = redactEmails(d.Services[i].Info)
			}
			// El anuncio Tuya pudo llegar en cualquier momento del escaneo:
			// se mira justo antes de reconocer.
			if t, ok := tuya.get(d.IP); ok {
				addSource(d, "tuya")
				d.Services = append(d.Services, Service{Proto: "tuya", Name: "tuya", Info: t.Version})
				if t.GwID != "" {
					setExtra(d, "tuya:gwId", t.GwID)
				}
				if t.ProductKey != "" {
					setExtra(d, "tuya:productKey", t.ProductKey)
				}
			}
			d.Identity = recognize(d)
			progress(true)
		}(d)
	}
	wg.Wait()
}

// filterToNet descarta de devs cualquier entrada cuya IP no esté dentro de
// ipNet. Cada fuente ya filtra por ipNet.Contains al insertar (ver Scan),
// así que en el camino normal no quita nada; existe como candado explícito
// y probable por sí solo para un escaneo Routed, donde un equipo del
// anillo interior NUNCA debe colarse en el resultado.
func filterToNet(devs map[string]*Device, ipNet *net.IPNet) {
	for ip := range devs {
		parsed := net.ParseIP(ip)
		if parsed == nil || !ipNet.Contains(parsed) {
			delete(devs, ip)
		}
	}
}

func addSource(d *Device, s string) {
	for _, x := range d.Sources {
		if x == s {
			return
		}
	}
	d.Sources = append(d.Sources, s)
}

func setExtra(d *Device, k, v string) {
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	d.Extra[k] = v
}

func trimLocal(h string) string {
	h = trimSuffixFold(h, ".local.")
	h = trimSuffixFold(h, ".local")
	h = trimSuffixFold(h, ".")
	return h
}

// liveIPs devuelve las claves de un mapa ip → fuentes como las arma
// sweepLiveness. Solo se llama una vez que esa goroutine ya terminó de
// escribirlo (ver sweepDone en Scan): antes sería una carrera de datos.
func liveIPs(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for ip := range m {
		out = append(out, ip)
	}
	return out
}

// netbiosFullScanLimit: por encima de esta cantidad de hosts, consultar
// NetBIOS a la subred entera (una petición UDP + un timeout corto por IP,
// aunque esté acotado a 64 en paralelo) sale demasiado caro; ver
// netbiosTargets.
const netbiosFullScanLimit = 1024

// netbiosTargets decide a qué IPs consultar NetBIOS: a TODA la subred si no
// son demasiadas (≤1024 hosts — el caso normal, un /24 son 254), o solo a
// las que `live` ya marca como vivas si son muchas. Función pura de sus dos
// insumos (sin tocar la red) para poder probar la regla del umbral sola.
func netbiosTargets(hosts []string, live map[string][]string) []string {
	if len(hosts) <= netbiosFullScanLimit {
		return hosts
	}
	return liveIPs(live)
}
