package mdns

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCommandFor(t *testing.T) {
	const fingerprint = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		goos    string
		wantBin string
	}{
		{"darwin", "dns-sd"},
		{"linux", "avahi-publish"},
		{"windows", ""},
		{"plan9", ""},
	}
	for _, c := range cases {
		bin, args := commandFor(c.goos, 7341, "abc-123", "Nodo-Estudio", fingerprint)
		if bin != c.wantBin {
			t.Errorf("commandFor(%q): bin = %q, want %q", c.goos, bin, c.wantBin)
		}
		if bin == "" {
			continue
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"7341", "v=1", "id=abc-123", "name=Nodo-Estudio", "_vigia._tcp", "tls=1", "cert=abcdef0123456789"} {
			if !strings.Contains(joined, want) {
				t.Errorf("commandFor(%q): args = %v, falta %q", c.goos, args, want)
			}
		}
	}
}

func TestCertPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abcd", "abcd"},
		{"abcdef0123456789", "abcdef0123456789"},
		{"abcdef0123456789abcdef0123456789", "abcdef0123456789"},
	}
	for _, c := range cases {
		if got := certPrefix(c.in); got != c.want {
			t.Errorf("certPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStart_SinRespondedorConocido(t *testing.T) {
	a := start(context.Background(), "", nil, discardLogger())
	if a.Running() {
		t.Fatal("no debería haber proceso: sin respondedor conocido para este SO")
	}
	a.Stop() // no debe entrar en pánico aunque no haya nada que matar
}

func TestStart_BinarioInexistente(t *testing.T) {
	a := start(context.Background(), "este-binario-no-existe-en-ningun-lado", nil, discardLogger())
	if a.Running() {
		t.Fatal("no debería haber proceso: el binario no existe")
	}
	a.Stop()
}

func TestStart_LanzaYStopLoMata(t *testing.T) {
	// "sleep" en vez de dns-sd/avahi-publish: lo que se prueba aquí es el
	// ciclo de vida del proceso hijo (lanzar, matar, Stop idempotente), no
	// el anuncio mDNS real — eso lo cubre la prueba manual en el host.
	a := start(context.Background(), "sleep", []string{"30"}, discardLogger())
	if !a.Running() {
		t.Fatal("esperaba un proceso corriendo")
	}

	a.mu.Lock()
	pid := a.cmd.Process.Pid
	a.mu.Unlock()

	a.Stop()
	if a.Running() {
		t.Fatal("Running() debería ser false tras Stop")
	}
	if !processExitedEventually(pid, 2*time.Second) {
		t.Fatalf("el proceso pid=%d debería haber terminado tras Stop", pid)
	}

	a.Stop() // idempotente
}

func TestStart_ElContextoCanceladoMataElProceso(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := start(ctx, "sleep", []string{"30"}, discardLogger())
	if !a.Running() {
		t.Fatal("esperaba un proceso corriendo")
	}
	a.mu.Lock()
	pid := a.cmd.Process.Pid
	a.mu.Unlock()

	cancel()

	if !processExitedEventually(pid, 2*time.Second) {
		t.Fatalf("el proceso pid=%d debería haber terminado al cancelar el contexto", pid)
	}
}

func processExitedEventually(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !pidAlive(pid)
}

func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
