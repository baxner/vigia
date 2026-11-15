package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// TestBuildEchoRequestRoundTrip comprueba que el paquete que armamos para
// mandar (buildEchoRequest) es, de verdad, un echo request ICMPv4 válido:
// que ParseMessage lo entienda de vuelta tal cual, con el mismo id/seq/datos.
// Es la única parte de icmp.go que no depende de un socket real.
func TestBuildEchoRequestRoundTrip(t *testing.T) {
	data := []byte("vigia")
	wb, err := buildEchoRequest(0x1234, 7, data)
	if err != nil {
		t.Fatalf("buildEchoRequest: %v", err)
	}

	rm, err := icmp.ParseMessage(protocolICMP, wb)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if rm.Type != ipv4.ICMPTypeEcho {
		t.Fatalf("Type = %v, want ICMPTypeEcho", rm.Type)
	}
	if rm.Code != 0 {
		t.Fatalf("Code = %d, want 0", rm.Code)
	}
	echo, ok := rm.Body.(*icmp.Echo)
	if !ok {
		t.Fatalf("Body = %T, want *icmp.Echo", rm.Body)
	}
	// El ID real que viaja en el paquete se trunca a 16 bits (es un campo de
	// 2 bytes del protocolo); 0x1234 ya entra entero.
	if echo.ID != 0x1234 {
		t.Errorf("ID = %#x, want %#x", echo.ID, 0x1234)
	}
	if echo.Seq != 7 {
		t.Errorf("Seq = %d, want 7", echo.Seq)
	}
	if !bytes.Equal(echo.Data, data) {
		t.Errorf("Data = %q, want %q", echo.Data, data)
	}
}

// TestBuildEchoRequestIDTruncation documenta el comportamiento con un ID que
// no entra en 16 bits (nunca debería pasar en este código — se construye con
// os.Getpid()&0xffff — pero conviene saber qué hace Marshal si algo cambia).
func TestBuildEchoRequestIDTruncation(t *testing.T) {
	wb, err := buildEchoRequest(0x1FFFF, 1, nil) // 17 bits
	if err != nil {
		t.Fatalf("buildEchoRequest: %v", err)
	}
	rm, err := icmp.ParseMessage(protocolICMP, wb)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	echo := rm.Body.(*icmp.Echo)
	if echo.ID != 0x1FFFF&0xffff {
		t.Errorf("ID = %#x, want %#x (truncado a 16 bits)", echo.ID, 0x1FFFF&0xffff)
	}
}

// TestICMPSweepSinHosts: sin hosts que mandar, no abre socket ni hace nada —
// devuelve nil, no un mapa vacío (contrato explícito de icmpSweep).
func TestICMPSweepSinHosts(t *testing.T) {
	ctx := context.Background()
	if got := icmpSweep(ctx, nil, 50*time.Millisecond); got != nil {
		t.Errorf("icmpSweep(nil hosts) = %v, want nil", got)
	}
	if got := icmpSweep(ctx, []string{}, 50*time.Millisecond); got != nil {
		t.Errorf("icmpSweep([]string{}) = %v, want nil", got)
	}
}

// TestICMPSweepLoopback: best-effort contra 127.0.0.1. En este entorno
// (macOS, sin root) el socket no privilegiado se abre y normalmente contesta
// nuestro propio ping; en una caja donde no se pueda (p. ej. Linux sin
// net.ipv4.ping_group_range para este usuario, un sandbox que bloquee el
// socket...) la función ya promete "nil en silencio", así que el test se
// salta en vez de fallar — lo que de verdad importa para la suite es que no
// cuelga más allá del timeout y no entra en pánico.
func TestICMPSweepLoopback(t *testing.T) {
	start := time.Now()
	got := icmpSweep(context.Background(), []string{"127.0.0.1"}, 500*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("icmpSweep tardó %s con timeout de 500ms: no está respetando el límite", elapsed)
	}
	if got == nil {
		t.Skip("socket ICMP no disponible en este entorno (ping_group_range/sandbox) — comportamiento best-effort esperado")
	}
	t.Logf("icmpSweep(127.0.0.1) = %v en %s", got, elapsed)
}

// TestPingRTT_Loopback: mismo criterio best-effort que TestICMPSweepLoopback
// — si el socket no privilegiado no se puede abrir en este entorno, se
// salta en vez de fallar.
func TestPingRTT_Loopback(t *testing.T) {
	start := time.Now()
	rtt, ok := PingRTT(context.Background(), "127.0.0.1", 500*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("PingRTT tardó %s con timeout de 500ms: no está respetando el límite", elapsed)
	}
	if !ok {
		t.Skip("socket ICMP no disponible en este entorno (ping_group_range/sandbox) — comportamiento best-effort esperado")
	}
	if rtt <= 0 {
		t.Fatalf("rtt = %v, want > 0", rtt)
	}
	if rtt > 500*time.Millisecond {
		t.Fatalf("rtt = %v, no debería superar el timeout", rtt)
	}
}

// TestPingRTT_HostInvalido: una IP mal formada no abre socket ni cuelga.
func TestPingRTT_HostInvalido(t *testing.T) {
	if _, ok := PingRTT(context.Background(), "no-es-una-ip", 100*time.Millisecond); ok {
		t.Fatal("ok = true con un host inválido, want false")
	}
}

// TestPingRTT_SinRespuesta: una IP que (casi seguro) no existe en la LAN
// local debe respetar el timeout sin colgarse más allá de él.
func TestPingRTT_SinRespuesta(t *testing.T) {
	start := time.Now()
	_, ok := PingRTT(context.Background(), "192.0.2.1", 200*time.Millisecond) // TEST-NET-1 (RFC 5737)
	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Fatalf("PingRTT tardó %s con timeout de 200ms", elapsed)
	}
	if ok {
		t.Log("192.0.2.1 contestó en esta red (inusual, pero no es un error del código)")
	}
}
