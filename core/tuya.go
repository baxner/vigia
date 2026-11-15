package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ===========================================================================
// Tuya (broadcast UDP 6666/6667, protocolo de descubrimiento local)
// ===========================================================================
//
// Los equipos Tuya (enchufes, bombillas, etc. — también los que se venden de
// marca blanca o "Smart Life") anuncian su presencia cada pocos segundos con
// un broadcast UDP a 255.255.255.255, sin que nadie les pregunte nada. Dos
// puertos según la versión del protocolo:
//   - 6666: protocolo 3.1, el JSON va en claro.
//   - 6667: protocolo 3.3+, el payload va cifrado (AES, clave por
//     dispositivo que no tenemos) — ahí solo podemos confirmar que ES un
//     equipo Tuya, sin sacar más detalle.
// Todo paquete, en cualquiera de los dos puertos, empieza con el mismo
// prefijo fijo: 00 00 55 aa (medido: 192.168.0.143, un "Tuya Smart", emite en
// 6667, paquetes de 172 bytes).
//
// No hay que preguntar nada: basta con escuchar esos dos puertos durante la
// ventana de descubrimiento.

// tuyaMagic: prefijo fijo de todo paquete del protocolo de broadcast de Tuya.
var tuyaMagic = []byte{0x00, 0x00, 0x55, 0xaa}

// tuyaPortSpec asocia un puerto de escucha con la versión de protocolo que
// implica.
type tuyaPortSpec struct {
	port    int
	version string
}

var tuyaPorts = []tuyaPortSpec{
	{6666, "3.1"},  // sin cifrar: JSON en claro tras la cabecera
	{6667, "3.3+"}, // cifrado (AES): solo confirmamos que es Tuya
}

// tuyaListener escucha los anuncios Tuya durante TODO el escaneo, no solo en
// la ventana de descubrimiento: un equipo Tuya anuncia cada ~5 s y con una
// ventana de 4 s se perdía en la mitad de los escaneos (medido con el foco
// .143). Así el anuncio puede llegar hasta justo antes de reconocer al equipo
// (ver enrich) sin alargar el escaneo.
type tuyaListener struct {
	mu     sync.Mutex
	out    map[string]TuyaInfo
	cancel context.CancelFunc
	done   chan struct{}
}

// startTuya empieza a escuchar 6666/6667 hasta stop() o hasta que ctx termine.
func startTuya(ctx context.Context) *tuyaListener {
	return startTuyaWithIface(ctx, "")
}

func startTuyaWithIface(ctx context.Context, iface string) *tuyaListener {
	ctx, cancel := context.WithCancel(ctx)
	l := &tuyaListener{out: map[string]TuyaInfo{}, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(l.done)
		var wg sync.WaitGroup
		for _, tp := range tuyaPorts {
			wg.Add(1)
			go func(tp tuyaPortSpec) {
				defer wg.Done()
				listenTuyaPort(ctx, tp, iface, l.out, &l.mu)
			}(tp)
		}
		wg.Wait()
	}()
	return l
}

// get: lo que se haya oído de ip hasta ahora. nil-safe: un *tuyaListener nil
// (una red Routed, donde Scan nunca llega a crear uno — ver tuya.go) se
// comporta como si no hubiera oído nada, sin que el resto del código
// necesite otra rama para ese caso.
func (l *tuyaListener) get(ip string) (TuyaInfo, bool) {
	if l == nil {
		return TuyaInfo{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	t, ok := l.out[ip]
	return t, ok
}

// snapshot: copia de todo lo oído hasta ahora. nil-safe (ver get).
func (l *tuyaListener) snapshot() map[string]TuyaInfo {
	if l == nil {
		return map[string]TuyaInfo{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]TuyaInfo, len(l.out))
	for ip, t := range l.out {
		out[ip] = t
	}
	return out
}

// stop deja de escuchar y espera a que se cierren los sockets. Se puede
// llamar más de una vez. nil-safe (ver get).
func (l *tuyaListener) stop() {
	if l == nil {
		return
	}
	l.cancel()
	<-l.done
}

// discoverTuya escucha durante `timeout` y devuelve IP → TuyaInfo de los
// equipos que anunciaron algo con la firma 000055aa.
func discoverTuya(ctx context.Context, timeout time.Duration) map[string]TuyaInfo {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	l := startTuya(ctx)
	<-ctx.Done()
	l.stop()
	return l.snapshot()
}

// listenTuyaPort escucha un puerto Tuya hasta que ctx termine, con
// SO_REUSEADDR+SO_REUSEPORT (reusePortControl, en reuseport_unix.go/
// reuseport_other.go según el SO) para poder convivir con otro proceso que ya
// esté escuchando ahí. Si el bind falla, vuelve sin hacer nada: sin Tuya en
// ese puerto, en silencio.
func listenTuyaPort(ctx context.Context, tp tuyaPortSpec, iface string, out map[string]TuyaInfo, mu *sync.Mutex) {
	lc := newListenConfig(iface, reusePortControl)
	conn, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", tp.port))
	if err != nil {
		return
	}
	defer conn.Close()

	// Mismo patrón que discoverMDNS/discoverSSDP/icmpSweep: ReadFrom no toma
	// ctx, así que lo desbloqueamos cerrando el socket a mano al terminar.
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

	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		data := buf[:n]
		if !isTuyaPacket(data) {
			continue
		}
		ip := addrIP(addr)
		if ip == "" {
			continue
		}
		info := TuyaInfo{Version: tp.version}
		if tp.port == 6666 {
			p := parseTuyaPlaintext(data)
			info.GwID = p.GwID
			info.ProductKey = p.ProductKey
		}
		mu.Lock()
		out[ip] = info
		mu.Unlock()
	}
}

// isTuyaPacket dice si `data` es un paquete de broadcast Tuya: todos empiezan
// con el prefijo 00 00 55 aa, visto igual en 6666 y 6667.
func isTuyaPacket(data []byte) bool {
	return len(data) >= len(tuyaMagic) && bytes.Equal(data[:len(tuyaMagic)], tuyaMagic)
}

// tuyaPlaintext: lo que nos interesa del JSON en claro de un paquete 3.1
// (puerto 6666).
type tuyaPlaintext struct {
	GwID       string `json:"gwId"`
	ProductKey string `json:"productKey"`
}

// parseTuyaPlaintext intenta sacar gwId/productKey del JSON en claro de un
// paquete 3.1. Busca el primer '{' y el último '}' del paquete en vez de
// calcular offsets exactos de cabecera/longitud/CRC: más tolerante a
// variaciones de formato que no verificamos a mano en la red real (a
// diferencia de 6667, solo medimos que 6666 existe en el protocolo, no un
// paquete real de un equipo en ese puerto). Best-effort: si no hay JSON
// válido ahí, devuelve el cero-valor.
func parseTuyaPlaintext(data []byte) tuyaPlaintext {
	start := bytes.IndexByte(data, '{')
	end := bytes.LastIndexByte(data, '}')
	if start < 0 || end <= start {
		return tuyaPlaintext{}
	}
	var p tuyaPlaintext
	_ = json.Unmarshal(data[start:end+1], &p)
	return p
}
