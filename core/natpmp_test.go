package core

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeNATPMPServer levanta un servidor UDP en 127.0.0.1 que, por cada
// datagrama recibido, contesta con lo que devuelva respond(request) (nil =
// no contestar, como un router con NAT-PMP apagado). Devuelve el puerto, a
// usar con withNATPMPPort.
func fakeNATPMPServer(t *testing.T, respond func(request []byte) []byte) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("no se pudo abrir el NAT-PMP falso: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 16)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // socket cerrado al terminar el test
			}
			req := append([]byte(nil), buf[:n]...)
			if resp := respond(req); resp != nil {
				_, _ = conn.WriteToUDP(resp, raddr)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

// natpmpSuccessResponse arma una respuesta válida de "Public Address
// Request" (RFC 6886 §3.2): versión 0, opcode 128, resultCode 0, IP externa
// en los últimos 4 bytes.
func natpmpSuccessResponse(externalIP string) []byte {
	resp := make([]byte, natpmpRespSize)
	resp[0] = natpmpVersion
	resp[1] = natpmpOpExternalAddrResp
	copy(resp[8:12], net.ParseIP(externalIP).To4())
	return resp
}

func withNATPMPPort(t *testing.T, port int) {
	t.Helper()
	old := natpmpPort
	natpmpPort = port
	t.Cleanup(func() { natpmpPort = old })
}

func TestQueryNATPMP_CasoReal(t *testing.T) {
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		return natpmpSuccessResponse("192.168.100.2")
	})
	withNATPMPPort(t, port)

	ip, ok := queryNATPMP(context.Background(), "127.0.0.1")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if ip != "192.168.100.2" {
		t.Errorf("externalIP = %q, want %q", ip, "192.168.100.2")
	}
}

// TestQueryNATPMP_PeticionCorrecta: el pedido es exactamente [version=0, op=0]
// ("Public Address Request", sin más cuerpo) — RFC 6886 §3.2.
func TestQueryNATPMP_PeticionCorrecta(t *testing.T) {
	reqCh := make(chan []byte, 1)
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		reqCh <- req
		return natpmpSuccessResponse("1.2.3.4")
	})
	withNATPMPPort(t, port)

	if _, ok := queryNATPMP(context.Background(), "127.0.0.1"); !ok {
		t.Fatal("ok = false")
	}
	select {
	case got := <-reqCh:
		if len(got) != 2 || got[0] != 0 || got[1] != 0 {
			t.Errorf("petición = %v, want [0 0] (versión 0, opcode 0)", got)
		}
	case <-time.After(time.Second):
		t.Fatal("el servidor falso nunca recibió la petición")
	}
}

func TestQueryNATPMP_ResultCodeDeError(t *testing.T) {
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		resp := natpmpSuccessResponse("192.168.100.2")
		resp[2], resp[3] = 0, 3 // resultCode=3 ("Network Failure", RFC 6886 §3.5)
		return resp
	})
	withNATPMPPort(t, port)

	if _, ok := queryNATPMP(context.Background(), "127.0.0.1"); ok {
		t.Error("ok = true, want false (resultCode distinto de 0)")
	}
}

func TestQueryNATPMP_VersionDeRespuestaIncorrecta(t *testing.T) {
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		resp := natpmpSuccessResponse("192.168.100.2")
		resp[0] = 1 // un router NAT-PMP nunca contesta con otra versión a esta petición
		return resp
	})
	withNATPMPPort(t, port)

	if _, ok := queryNATPMP(context.Background(), "127.0.0.1"); ok {
		t.Error("ok = true, want false (versión de respuesta != 0)")
	}
}

func TestQueryNATPMP_OpcodeDeRespuestaIncorrecto(t *testing.T) {
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		resp := natpmpSuccessResponse("192.168.100.2")
		resp[1] = 129 // opcode de respuesta de OTRA operación (p. ej. mapear un puerto)
		return resp
	})
	withNATPMPPort(t, port)

	if _, ok := queryNATPMP(context.Background(), "127.0.0.1"); ok {
		t.Error("ok = true, want false (opcode de respuesta != 128)")
	}
}

func TestQueryNATPMP_RespuestaCorta(t *testing.T) {
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		return []byte{0, 128, 0, 0} // menos de los 12 bytes del RFC
	})
	withNATPMPPort(t, port)

	if _, ok := queryNATPMP(context.Background(), "127.0.0.1"); ok {
		t.Error("ok = true, want false (respuesta más corta que natpmpRespSize)")
	}
}

// TestQueryNATPMP_SinRespuesta: un router con NAT-PMP apagado no contesta
// nada — ok=false, y sin quedarse esperando más que upnpStepTimeout (el
// paso, no los 6s totales de QueryRouter: queryNATPMP no conoce ese
// presupuesto, solo el suyo).
func TestQueryNATPMP_SinRespuesta(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close() // nadie escucha ahí
	withNATPMPPort(t, port)

	start := time.Now()
	_, ok := queryNATPMP(context.Background(), "127.0.0.1")
	elapsed := time.Since(start)

	if ok {
		t.Error("ok = true, want false (nadie contestó)")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("queryNATPMP tardó %s, debía acotarse a upnpStepTimeout (2s)", elapsed)
	}
}

func TestQueryNATPMP_GatewayInvalida(t *testing.T) {
	if _, ok := queryNATPMP(context.Background(), "no-es-una-ip"); ok {
		t.Error("ok = true, want false (gateway inválida)")
	}
}

func TestQueryNATPMP_ReintentaSiSePierdeLaPrimera(t *testing.T) {
	var mu sync.Mutex
	seen := 0
	port := fakeNATPMPServer(t, func(req []byte) []byte {
		mu.Lock()
		defer mu.Unlock()
		seen++
		if seen == 1 {
			return nil // la primera "se pierde"
		}
		return natpmpSuccessResponse("192.168.100.2")
	})
	withNATPMPPort(t, port)

	ip, ok := queryNATPMP(context.Background(), "127.0.0.1")
	if !ok || ip != "192.168.100.2" {
		t.Fatalf("queryNATPMP = (%q, %v), quería (192.168.100.2, true) al segundo intento", ip, ok)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen < 2 {
		t.Fatalf("el router falso vio %d intentos, quería al menos 2", seen)
	}
}
