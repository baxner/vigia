package core

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// localNetwork devuelve la red IPv4 a escanear y nuestra IP en ella. Si cidr != ""
// se usa tal cual; si no, se autodetecta la de la interfaz con ruta por defecto
// (la primera IPv4 privada no-loopback que esté arriba).
func localNetwork(cidr string) (*net.IPNet, net.IP, error) {
	return localNetworkOnInterface(cidr, "")
}

func localNetworkOnInterface(cidr, iface string) (*net.IPNet, net.IP, error) {
	if cidr != "" {
		ip, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, nil, err
		}
		if v4 := ip.To4(); v4 != nil {
			ipNet.IP = v4.Mask(ipNet.Mask)
		}
		return ipNet, ip, nil
	}
	if ifc := interfaceByName(iface); ifc != nil && ifc.Flags&net.FlagUp != 0 {
		if addrs, err := ifc.Addrs(); err == nil {
			for _, a := range addrs {
				n, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				v4 := n.IP.To4()
				if v4 == nil || !v4.IsPrivate() {
					continue
				}
				return &net.IPNet{IP: v4.Mask(n.Mask), Mask: n.Mask}, v4, nil
			}
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := n.IP.To4()
			if v4 == nil || !v4.IsPrivate() {
				continue
			}
			return &net.IPNet{IP: v4.Mask(n.Mask), Mask: n.Mask}, v4, nil
		}
	}
	return nil, nil, errors.New("no se encontró una red IPv4 privada activa")
}

// localInterfaceAddrs junta las IP (v4 y v6) de todas las interfaces locales
// activas, para que isRoutedNetwork decida si una red está "detrás" de
// alguna de ellas o es de delante del router. Best-effort: nil si
// net.Interfaces() falla (se trata como "no se pudo comprobar" en
// isRoutedNetwork, no como "ninguna interfaz").
func localInterfaceAddrs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				out = append(out, n.IP)
			}
		}
	}
	return out
}

// networkHasLocalAddr dice si alguna de addrs cae dentro de ipNet. Función
// pura (sin llamar a net.Interfaces() ella misma) para poder probar la
// regla de isRoutedNetwork con una lista armada a mano, sin depender de las
// interfaces reales de la máquina donde corra el test.
func networkHasLocalAddr(ipNet *net.IPNet, addrs []net.IP) bool {
	for _, ip := range addrs {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// isRoutedNetwork dice si ipNet es una red "de delante del router" (el
// anillo exterior de la Fase 5): ninguna interfaz local tiene una dirección
// ahí. Si no se pudo ni listar las interfaces, se asume que NO es Routed
// (false): es el comportamiento de siempre, para no cambiar nada en el
// camino normal ante un error al leerlas.
func isRoutedNetwork(ipNet *net.IPNet) bool {
	addrs := localInterfaceAddrs()
	if addrs == nil {
		return false
	}
	return !networkHasLocalAddr(ipNet, addrs)
}

// hostsInNet enumera las IPs de host de una red (sin red ni broadcast). Se acota a
// /16 como máximo para no generar rangos enormes por accidente.
func hostsInNet(ipNet *net.IPNet) []string {
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil
	}
	if ones < 16 {
		ones = 16 // acota a 65 534 hosts máx.
	}
	base := ipNet.IP.Mask(net.CIDRMask(ones, 32)).To4()
	if base == nil {
		return nil
	}
	count := 1 << (32 - ones)
	out := make([]string, 0, count)
	start := ipToU32(base)
	for i := 1; i < count-1; i++ { // salta .0 (red) y la última (broadcast)
		out = append(out, u32ToIP(start+uint32(i)))
	}
	return out
}

// sweepLiveness prueba cada host con TCP connect en unos pocos puertos Y, a la
// vez, con un barrido ICMP (icmpSweep, core/icmp.go) por un único socket sin
// privilegios. Un host que acepta la conexión TCP, la rechaza (RST), o
// contesta al ping está vivo por cualquiera de los dos caminos; el intento TCP
// además hace que el SO resuelva su MAC, poblando la tabla ARP para la fase
// siguiente. Ninguno de los dos necesita root y funcionan igual en macOS,
// Linux y Android — ICMP es precisamente el que rescata lo que TCP solo no ve
// (medido: un teléfono con MAC aleatoria que descarta el SYN sin responder).
//
// Devuelve ip → fuentes que lo vieron vivo ("ping" y/o "icmp"), para que
// quien arma el Device sepa marcar el origen (Sources) de cada uno en vez de
// asumir que todos vinieron de TCP.
func sweepLiveness(ctx context.Context, ipNet *net.IPNet, opts ScanOptions) map[string][]string {
	hosts := hostsInNet(ipNet)
	timeout := orDur(opts.SweepTimeout, DefaultSweepTimeout)
	conc := orInt(opts.Concurrency, DefaultConcurrency)
	probePorts := []int{80, 443, 22, 445, 139, 53, 7, 9, 548, 5000, 3689, 8009, 62078}

	var (
		mu    sync.Mutex
		live  = map[string][]string{}
		done  int
		sem   = make(chan struct{}, conc)
		wg    sync.WaitGroup
		total = len(hosts)
	)

	// ICMP corre en paralelo al barrido TCP (no antes ni después): así un
	// /24 completo sigue saliendo en el mismo ~1-3 s de siempre en vez de la
	// suma de los dos. Con un piso propio un poco más largo que el timeout
	// por host de TCP: medido en la red real, bajo la carga de las ~254
	// conexiones TCP + NetBIOS + mDNS/SSDP/Tuya a la vez, algún eco que SÍ
	// vuelve dentro de los 1.2 s por defecto en una corrida de icmpSweep
	// aislada se puede perder por los pocos ms de más que tarda en
	// programarse esta goroutine con todo lo demás compitiendo por CPU. 2 s
	// (el propio piso que usó la medición original) da margen sin arriesgar
	// el "≤ ~3 s" del barrido completo: wg.Wait()+icmpWG.Wait() esperan al
	// más lento de los dos, y el peor caso de TCP solo (dos tandas a 128 de
	// concurrencia para 254 hosts) ya anda cerca de 2.4 s.
	icmpTimeout := timeout
	if icmpTimeout < 2*time.Second {
		icmpTimeout = 2 * time.Second
	}
	var icmpWG sync.WaitGroup
	icmpWG.Add(1)
	go func() {
		defer icmpWG.Done()
		alive := icmpSweepWithIface(ctx, hosts, icmpTimeout, opts.BindInterface)
		if len(alive) == 0 {
			return
		}
		mu.Lock()
		for ip := range alive {
			live[ip] = append(live[ip], "icmp")
		}
		mu.Unlock()
	}()

	for _, h := range hosts {
		select {
		case <-ctx.Done():
			wg.Wait()
			icmpWG.Wait()
			return live
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			alive := hostAliveWithIface(ctx, ip, probePorts, timeout, opts.BindInterface)
			// Un solo tramo bajo lock: así `live`, el contador y la llamada de
			// progreso quedan serializados (el motor garantiza que OnProgress no
			// se invoca en paralelo; el callback no necesita ser seguro por sí).
			mu.Lock()
			if alive {
				live[ip] = append(live[ip], "ping")
			}
			done++
			if opts.OnProgress != nil {
				opts.OnProgress("sweep", done, total)
			}
			mu.Unlock()
		}(h)
	}
	wg.Wait()
	icmpWG.Wait()
	return live
}

// hostAlive: vivo si algún puerto acepta o rechaza (RST) la conexión dentro de timeout.
// hostAlive prueba todas las sondas a la vez y para en la primera respuesta:
// una conexión aceptada o un RST (puerto cerrado) prueban que el host existe.
// En paralelo y con timeout holgado porque en Android no hay tabla ARP que
// rescate a un equipo que tardó en contestar: si el barrido no lo ve, no
// aparece.
func hostAlive(ctx context.Context, ip string, ports []int, timeout time.Duration) bool {
	return hostAliveWithIface(ctx, ip, ports, timeout, "")
}

func hostAliveWithIface(ctx context.Context, ip string, ports []int, timeout time.Duration, iface string) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var (
		alive atomic.Bool
		wg    sync.WaitGroup
	)
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			d := newDialer(iface, 0)
			conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, itoa(port)))
			if err == nil {
				conn.Close()
			}
			if err == nil || errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused") {
				alive.Store(true) // el host respondió (SYN-ACK o RST): está vivo
				cancel()
			}
		}(p)
	}
	wg.Wait()
	return alive.Load()
}

// neighborTable combina ARP (IPv4) y NDP/ip-neigh (IPv6) en ip → MAC normalizada.
// Best-effort: en Android 11+ puede volver vacío; el descubrimiento sigue por mDNS/SSDP.
func neighborTable() map[string]string {
	out := map[string]string{}
	switch runtime.GOOS {
	case "linux", "android":
		mergeProcNetArp(out)
		mergeIPNeigh(out)
	case "ios":
		// El sandbox de iOS no expone tabla ARP/NDP ni permite exec.
	default: // darwin y otros BSD
		mergeCmd(out, "arp", "-an")
		mergeCmd(out, "ndp", "-an")
	}
	return out
}

// mergeCmd corre un comando tipo `arp -an` / `ndp -an` y extrae pares (IP, MAC) de
// cada línea, con un parser tolerante: busca algo "(1.2.3.4)" o un IPv6 y una MAC.
func mergeCmd(out map[string]string, name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		ip, mac := ipAndMACFromLine(sc.Text())
		if ip != "" && mac != "" {
			out[ip] = mac
		}
	}
}

func mergeProcNetArp(out map[string]string) {
	data, err := readFile("/proc/net/arp")
	if err != nil {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Scan() // cabecera
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 4 {
			if mac := normalizeMAC(f[3]); mac != "" && mac != "00:00:00:00:00:00" {
				out[f[0]] = mac
			}
		}
	}
}

func mergeIPNeigh(out map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "ip", "neigh").Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		// "192.168.0.5 dev en0 lladdr aa:bb:cc:dd:ee:ff REACHABLE"
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		ip := f[0]
		for i := 1; i < len(f)-1; i++ {
			if f[i] == "lladdr" {
				if mac := normalizeMAC(f[i+1]); mac != "" {
					out[ip] = mac
				}
			}
		}
	}
}

// ipAndMACFromLine extrae la primera IP (v4 entre paréntesis o suelta, o v6) y la
// primera MAC de una línea arbitraria de `arp`/`ndp`.
func ipAndMACFromLine(line string) (ip, mac string) {
	for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' }) {
		t := strings.Trim(tok, "()")
		if ip == "" {
			if p := net.ParseIP(t); p != nil && !p.IsUnspecified() {
				ip = t
				continue
			}
		}
		if mac == "" {
			if m := normalizeMAC(t); m != "" && (strings.Count(t, ":") == 5 || strings.Count(t, "-") == 5) {
				mac = m
			}
		}
	}
	if mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
		mac = ""
	}
	return ip, mac
}

// defaultGateway: IP del router por defecto (best-effort, por el comando del SO).
func defaultGateway() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "linux", "android":
		if data, err := exec.CommandContext(ctx, "ip", "route").Output(); err == nil {
			for _, ln := range strings.Split(string(data), "\n") {
				f := strings.Fields(ln)
				if len(f) >= 3 && f[0] == "default" && f[1] == "via" {
					return f[2]
				}
			}
		}
		if runtime.GOOS == "linux" {
			return defaultGatewayFromProcNetRoute()
		}
	case "ios":
		return ""
	default:
		if data, err := exec.CommandContext(ctx, "route", "-n", "get", "default").Output(); err == nil {
			for _, ln := range strings.Split(string(data), "\n") {
				ln = strings.TrimSpace(ln)
				if strings.HasPrefix(ln, "gateway:") {
					return strings.TrimSpace(strings.TrimPrefix(ln, "gateway:"))
				}
			}
		}
	}
	return ""
}

// addrIP saca la IP de un net.Addr devuelto por ReadFrom en un socket UDP/ICMP
// sin privilegios (siempre *net.UDPAddr para "udp4"); se acepta también
// *net.IPAddr por si alguna vez se usa con un socket crudo. "" si no es
// ninguno de los dos. La usan icmp.go y tuya.go.
func addrIP(addr net.Addr) string {
	switch a := addr.(type) {
	case *net.UDPAddr:
		return a.IP.String()
	case *net.IPAddr:
		return a.IP.String()
	default:
		return ""
	}
}

// --- helpers numéricos (sin dependencias) ---

func ipToU32(ip net.IP) uint32 {
	v4 := ip.To4()
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
}

func u32ToIP(v uint32) string {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
}

func orDur(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

func orInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
