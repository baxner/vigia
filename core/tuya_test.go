package core

import (
	"context"
	"testing"
	"time"
)

// TestIsTuyaPacket comprueba la detección del prefijo 00 00 55 aa, medido en
// la red real (192.168.0.143, un "Tuya Smart", 172 bytes en el puerto 6667).
func TestIsTuyaPacket(t *testing.T) {
	casos := []struct {
		nombre string
		data   []byte
		want   bool
	}{
		{"prefijo exacto, sin nada más", []byte{0x00, 0x00, 0x55, 0xaa}, true},
		{"prefijo + payload", append([]byte{0x00, 0x00, 0x55, 0xaa}, []byte("resto del paquete")...), true},
		{"prefijo distinto", []byte{0x00, 0x00, 0x55, 0xab}, false},
		{"demasiado corto", []byte{0x00, 0x00, 0x55}, false},
		{"vacío", []byte{}, false},
		{"nil", nil, false},
		{"basura sin relación", []byte("GET / HTTP/1.1"), false},
	}
	for _, c := range casos {
		if got := isTuyaPacket(c.data); got != c.want {
			t.Errorf("%s: isTuyaPacket(%v) = %v, want %v", c.nombre, c.data, got, c.want)
		}
	}
}

// TestParseTuyaPlaintext comprueba la extracción best-effort de gwId/
// productKey del JSON en claro de un paquete 3.1 (puerto 6666).
func TestParseTuyaPlaintext(t *testing.T) {
	t.Run("JSON válido entre cabecera y relleno", func(t *testing.T) {
		// Forma real aproximada: magic + campos binarios + JSON + CRC/suffix.
		pkt := append([]byte{0x00, 0x00, 0x55, 0xaa, 0, 0, 0, 1, 0, 0, 0, 0},
			[]byte(`{"ip":"192.168.0.143","gwId":"abc123","active":2,"productKey":"keyXYZ","version":"3.1"}`)...)
		pkt = append(pkt, 0xaa, 0x55, 0x00, 0x00) // suffix inventado para el test

		got := parseTuyaPlaintext(pkt)
		if got.GwID != "abc123" {
			t.Errorf("GwID = %q, want %q", got.GwID, "abc123")
		}
		if got.ProductKey != "keyXYZ" {
			t.Errorf("ProductKey = %q, want %q", got.ProductKey, "keyXYZ")
		}
	})

	t.Run("sin JSON (payload cifrado, 3.3+)", func(t *testing.T) {
		pkt := append([]byte{0x00, 0x00, 0x55, 0xaa}, []byte("\x8f\x02\xaa\xcc\x00binarioqueNoEsJSON")...)
		got := parseTuyaPlaintext(pkt)
		if got.GwID != "" || got.ProductKey != "" {
			t.Errorf("got = %+v, want cero-valor sin JSON", got)
		}
	})

	t.Run("JSON incompleto (sin cierre)", func(t *testing.T) {
		pkt := []byte(`{"gwId":"abc123"`) // falta el '}'
		got := parseTuyaPlaintext(pkt)
		if got != (tuyaPlaintext{}) {
			t.Errorf("got = %+v, want cero-valor", got)
		}
	})

	t.Run("vacío", func(t *testing.T) {
		if got := parseTuyaPlaintext(nil); got != (tuyaPlaintext{}) {
			t.Errorf("got = %+v, want cero-valor", got)
		}
	})
}

// TestDiscoverTuyaTimeout: sin tráfico real, discoverTuya debe devolver un
// mapa (no necesariamente nil: distinto contrato que icmpSweep) dentro del
// timeout pedido, sin colgarse ni entrar en pánico, abra o no los sockets
// 6666/6667 en este entorno (best-effort: ver reusePortControl).
func TestDiscoverTuyaTimeout(t *testing.T) {
	start := time.Now()
	got := discoverTuya(context.Background(), 300*time.Millisecond)
	elapsed := time.Since(start)

	if got == nil {
		t.Error("discoverTuya() = nil, want mapa (aunque esté vacío)")
	}
	if len(got) != 0 {
		t.Logf("discoverTuya encontró equipos reales en la red de prueba: %v", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("discoverTuya tardó %s con timeout de 300ms: no está respetando el límite", elapsed)
	}
}
