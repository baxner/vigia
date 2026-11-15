package core

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// NAT-PMP (RFC 6886), UDP 5351: alternativa a UPnP que algunos routers traen
// encendida además o en vez de IGD. Solo pedimos la IP externa (opcode 0,
// "Public Address Request"); nada de mapear puertos — SOLO LECTURA, igual
// que upnp.go.
//
// Pedido (2 bytes): Version=0, OP=0.
// Respuesta (12 bytes), RFC 6886 §3.2:
//
//	byte 0:    Version (0)
//	byte 1:    OP (128 + 0 = 128, "respuesta a la petición 0")
//	bytes 2-3: Result Code (uint16 big-endian; 0 = sin error)
//	bytes 4-7: Seconds Since Start of Epoch (uint32 big-endian, no se usa aquí)
//	bytes 8-11: External IP Address (4 bytes)
const (
	natpmpVersion            = 0
	natpmpOpExternalAddrReq  = 0
	natpmpOpExternalAddrResp = 128 + natpmpOpExternalAddrReq
	natpmpRespSize           = 12
)

// natpmpPort: puerto UDP de NAT-PMP (5351). var en vez de const para poder
// apuntarlo a un servidor UDP falso en las pruebas (ver natpmp_test.go); en
// producción siempre es 5351.
var natpmpPort = 5351

// natpmpRetryWaits: cuánto esperar respuesta tras cada intento (RFC 6886
// empieza en 250 ms y duplica). Suman 1.75 s, dentro del paso de 2 s.
var natpmpRetryWaits = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

// queryNATPMP pide la IP externa por NAT-PMP. ok=false si el router no
// contesta, contesta con un resultCode de error, o la respuesta no tiene la
// forma esperada — nunca es un error de Go: "no contesta NAT-PMP" es el
// caso normal de casi todos los routers.
//
// net.Dial conecta el socket UDP a gateway:natpmpPort: el kernel descarta
// cualquier datagrama que no venga de esa IP:puerto exactos, que es la forma
// más simple de cumplir "quedarse solo con respuestas del gateway" para un
// protocolo que ya es unicast puro (sin el multicast de SSDP).
func queryNATPMP(ctx context.Context, gateway string) (externalIP string, ok bool) {
	return queryNATPMPWithIface(ctx, gateway, "")
}

func queryNATPMPWithIface(ctx context.Context, gateway, iface string) (externalIP string, ok bool) {
	if net.ParseIP(gateway) == nil {
		return "", false
	}

	ctx, cancel := context.WithTimeout(ctx, upnpStepTimeout)
	defer cancel()

	d := newDialer(iface, upnpStepTimeout)
	conn, err := d.DialContext(ctx, "udp4", net.JoinHostPort(gateway, strconv.Itoa(natpmpPort)))
	if err != nil {
		return "", false
	}
	defer conn.Close()

	var wg sync.WaitGroup
	defer wg.Wait()
	stop := make(chan struct{})
	defer close(stop)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			conn.Close() // desbloquea Read, que no toma ctx
		case <-stop:
		}
	}()

	// RFC 6886 §3.1: UDP se pierde, así que se reintenta con espera creciente
	// (todo dentro del paso de upnpStepTimeout). Con un solo intento, un
	// paquete perdido bastaba para decir "el router no contesta".
	req := []byte{natpmpVersion, natpmpOpExternalAddrReq}
	buf := make([]byte, 16)
	var n int
	for i, wait := range natpmpRetryWaits {
		if _, err := conn.Write(req); err != nil {
			return "", false
		}
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		var err error
		n, err = conn.Read(buf)
		if err == nil {
			break
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() && ctx.Err() == nil && i < len(natpmpRetryWaits)-1 {
			continue
		}
		return "", false
	}
	if n < natpmpRespSize || buf[0] != natpmpVersion || buf[1] != natpmpOpExternalAddrResp {
		return "", false
	}
	resultCode := binary.BigEndian.Uint16(buf[2:4])
	if resultCode != 0 {
		return "", false
	}
	ip := net.IPv4(buf[8], buf[9], buf[10], buf[11])
	return ip.String(), true
}
