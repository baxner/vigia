package core

import (
	"context"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// protocolICMP: número de protocolo IPv4 para ICMP (RFC 792) — el que espera
// icmp.ParseMessage. golang.org/x/net/internal/iana no se puede importar
// desde fuera de ese módulo; 1 es ese mismo valor (ProtocolICMP).
const protocolICMP = 1

// icmpPace: espera entre un echo y el siguiente al barrer una lista de hosts,
// para no mandar una ráfaga de cientos de paquetes de golpe por el mismo
// socket. Medido: un /24 entero (254 hosts) sale en ~0.5 s de envío a este
// ritmo, dejando el resto del timeout libre para que lleguen las respuestas
// (254 echos → 17 respuestas en 2 s, en la red real).
const icmpPace = 2 * time.Millisecond

// icmpSweep manda un ICMP echo request (ping) a cada IP de `hosts` por un
// único socket SIN privilegios (golang.org/x/net/icmp, sin CAP_NET_RAW ni
// root) y devuelve qué IPs contestaron con un echo reply antes de `timeout`.
//
// Pensado para correr en paralelo con el barrido TCP de sweepLiveness (ver
// netutil.go): un host vivo por cualquiera de los dos entra al resultado.
// Hace falta sobre todo para equipos que descartan el SYN sin responder ni
// con RST — medido en la red real: un teléfono con MAC aleatoria
// (192.168.0.53) invisible por TCP pero que sí responde al ping — y es
// exactamente lo que le falta a Android 11+, que tampoco puede leer la tabla
// ARP: este socket SÍ funciona ahí (net.ipv4.ping_group_range de fábrica
// incluye el grupo de las apps).
//
// Best-effort: si el socket no se puede abrir (Linux "de escritorio" sin
// net.ipv4.ping_group_range para este usuario/grupo, Windows sin el
// privilegio que haga falta ahí...) devuelve nil en silencio — sweepLiveness
// sigue con el barrido TCP solo, igual que si esta función no existiera.
//
// OJO Linux/Android: estos sockets "udp4" son el truco sin privilegios del
// kernel (ping sockets) — a TODOS los paquetes que mandemos por este socket
// el kernel les reescribe el ID del echo con el puerto local, el mismo para
// cualquier destino. Por eso NO se puede usar el ID (ni fiarse del todo del
// Seq) para saber a qué host pertenece una respuesta: lo único fiable, y lo
// único que necesitamos para un barrido de liveness, es la IP de origen del
// paquete de vuelta.
func icmpSweep(ctx context.Context, hosts []string, timeout time.Duration) map[string]bool {
	return icmpSweepWithIface(ctx, hosts, timeout, "")
}

func icmpSweepWithIface(ctx context.Context, hosts []string, timeout time.Duration, iface string) map[string]bool {
	if len(hosts) == 0 {
		return nil
	}
	conn, err := openICMPConn(iface)
	if err != nil {
		return nil // sin privilegios/soporte en este SO: en silencio
	}
	// Orden LIFO de defers: primero cancel/close(stop) desbloquean las goroutines,
	// luego wg.Wait() espera que salgan de conn.WriteTo/Close y al final conn.Close().
	defer conn.Close()
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Mismo patrón que discoverMDNS/discoverSSDP (discover_extra.go):
	// ReadFrom no toma ctx, así que lo desbloqueamos cerrando el socket a
	// mano cuando el contexto termine.
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		sendICMPEchoes(ctx, conn, hosts)
	}()

	// Solo cuentan respuestas de los hosts barridos: el socket puede recibir
	// otros echo reply (otro ping de la máquina, una IP fuera de la subred).
	want := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		want[h] = true
	}

	// Un solo lector (esta goroutine): nada más escribe en `out`, así que no
	// hace falta mutex.
	out := map[string]bool{}
	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return out // socket cerrado (timeout/cancelación) u otro error: terminamos
		}
		rm, err := icmp.ParseMessage(protocolICMP, buf[:n])
		if err != nil || rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if ip := addrIP(peer); want[ip] {
			out[ip] = true
		}
	}
}

// sendICMPEchoes manda un echo request a cada host, con una pausa breve
// (icmpPace) entre uno y el siguiente. Corre en su propia goroutine: mandar y
// leer son independientes, así que no hace falta terminar de mandar para
// empezar a recibir respuestas.
func sendICMPEchoes(ctx context.Context, conn icmpPacketConn, hosts []string) {
	id := os.Getpid() & 0xffff
	for i, h := range hosts {
		select {
		case <-ctx.Done():
			return
		default:
		}
		dst := net.ParseIP(h)
		if dst == nil {
			continue
		}
		wb, err := buildEchoRequest(id, i+1, []byte("vigia"))
		if err != nil {
			continue
		}
		_, _ = conn.WriteTo(wb, &net.UDPAddr{IP: dst})
		if i < len(hosts)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(icmpPace):
			}
		}
	}
}

// PingRTT manda UN ICMP echo request a host y mide cuánto tarda la
// respuesta, por el mismo socket sin privilegios que icmpSweep (ver el
// comentario de esa función: "udp4", funciona igual en macOS/Linux sin
// CAP_NET_RAW ni root). Pensado para medir latencia de un tramo puntual
// (ver agent/internet, Fase 7: "router y módem"), no para barrer una red
// entera — por eso toma un solo host en vez de una lista.
//
// ok=false si no hubo respuesta antes de timeout, o si el socket no se
// pudo abrir (sin privilegios/soporte en este SO: en silencio, igual que
// icmpSweep) — en ambos casos rtt vale 0 y no debe usarse.
func PingRTT(ctx context.Context, host string, timeout time.Duration) (rtt time.Duration, ok bool) {
	dst := net.ParseIP(host)
	if dst == nil {
		return 0, false
	}
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return 0, false
	}
	defer conn.Close()
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
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

	id := os.Getpid() & 0xffff
	wb, err := buildEchoRequest(id, 1, []byte("vigia"))
	if err != nil {
		return 0, false
	}
	sentAt := time.Now()
	if _, err := conn.WriteTo(wb, &net.UDPAddr{IP: dst}); err != nil {
		return 0, false
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return 0, false // timeout/cancelación: sin respuesta a tiempo.
		}
		rm, err := icmp.ParseMessage(protocolICMP, buf[:n])
		if err != nil || rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if addrIP(peer) != host {
			continue // eco de otro ping concurrente por el mismo socket.
		}
		return time.Since(sentAt), true
	}
}

// buildEchoRequest arma (marshaling incluido) un paquete ICMP echo request
// (type 8) con el id/seq/datos dados, listo para WriteTo. Separado de
// sendICMPEchoes para poder probarlo sin un socket real: el round-trip
// marshal→ParseMessage es lo único de este archivo que no depende de la red.
func buildEchoRequest(id, seq int, data []byte) ([]byte, error) {
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: data},
	}
	return wm.Marshal(nil)
}
