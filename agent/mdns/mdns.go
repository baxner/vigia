// Package mdns anuncia el agente por mDNS/Bonjour (_vigia._tcp, TXT v=1,
// id=<id>, name=<name> — ver docs/agente-api.md, sección "Descubrimiento")
// delegando siempre en el respondedor del sistema, nunca con una librería
// mDNS propia: en macOS `dns-sd -R` como proceso hijo; en Linux
// `avahi-publish` si está instalado. Si ninguno está disponible, el agente
// sigue funcionando sin anunciarse (lo dice el log, no es un error fatal) —
// quien ya está vinculado, o entra por IP:puerto a mano, no lo necesita.
package mdns

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
)

// Announcer mantiene (o no, si no había respondedor disponible) un anuncio
// mDNS vivo como proceso hijo.
type Announcer struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	log *slog.Logger
}

// Start intenta anunciar _vigia._tcp en port con TXT v=1/id/name/tls/cert
// (ver TLS v1.5, docs/agente-api.md: "tls=1", "cert=<primeros 16 hex>").
// certFingerprintHex es la huella SHA-256 completa del certificado (ver
// agent.Cert.FingerprintSHA256); certPrefix se queda con los primeros 16
// caracteres para el TXT. Nunca devuelve error: a falta de un respondedor
// para este sistema (o si no está instalado), deja constancia en el log con
// Warn y el Announcer queda sin proceso — Stop sobre él no hace nada.
func Start(ctx context.Context, port int, id, name, certFingerprintHex string, log *slog.Logger) *Announcer {
	bin, args := commandFor(runtime.GOOS, port, id, name, certFingerprintHex)
	return start(ctx, bin, args, log)
}

func start(ctx context.Context, bin string, args []string, log *slog.Logger) *Announcer {
	if log == nil {
		log = slog.Default()
	}
	a := &Announcer{log: log}

	if bin == "" {
		log.Warn("mdns: sin respondedor conocido para este sistema, no se anuncia", "os", runtime.GOOS)
		return a
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		log.Warn("mdns: respondedor no instalado, no se anuncia", "bin", bin, "error", err)
		return a
	}

	cmd := exec.CommandContext(ctx, path, args...)
	if err := cmd.Start(); err != nil {
		log.Warn("mdns: no se pudo lanzar el respondedor, no se anuncia", "bin", bin, "error", err)
		return a
	}
	log.Info("mdns: anunciando _vigia._tcp", "bin", bin, "pid", cmd.Process.Pid)

	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()

	// Recoge el proceso cuando Stop lo mate (o si el contexto se cancela: ver
	// CommandContext) o si muere solo — sin esto quedaría zombie.
	go func() { _ = cmd.Wait() }()

	return a
}

// Stop mata el proceso hijo si lo hay (p. ej. dns-sd). Idempotente: se
// puede llamar aunque Start nunca haya lanzado nada, o más de una vez.
func (a *Announcer) Stop() {
	a.mu.Lock()
	cmd := a.cmd
	a.cmd = nil
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

// Running: hay un proceso anunciando en este momento.
func (a *Announcer) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cmd != nil
}

// commandFor decide el binario y los argumentos del respondedor del
// sistema para goos. bin == "" significa "ninguno conocido para este
// sistema" (no es un error, Start lo registra como Warn y sigue).
func commandFor(goos string, port int, id, name, certFingerprintHex string) (bin string, args []string) {
	txt := []string{"v=1", "id=" + id, "name=" + name, "tls=1", "cert=" + certPrefix(certFingerprintHex)}
	switch goos {
	case "darwin":
		// dns-sd -R <name> <type> <domain> <port> [key=value ...]
		// (ver `man dns-sd`; "." es el dominio por defecto, resuelve a "local.")
		return "dns-sd", append([]string{"-R", name, "_vigia._tcp", ".", fmt.Sprint(port)}, txt...)
	case "linux":
		// avahi-publish -s <name> <type> <port> [<txt> ...]
		return "avahi-publish", append([]string{"-s", name, "_vigia._tcp", fmt.Sprint(port)}, txt...)
	default:
		return "", nil
	}
}

// certPrefix: primeros 16 caracteres hex de la huella (ver contrato: TXT
// "cert=<primeros 16 hex>"). Un TXT record entero tiene un límite práctico
// de 255 bytes y van varios en el mismo anuncio — 16 hex alcanza para que
// la app distinga de un vistazo si el certificado cambió sin acercarse a
// ese límite; la huella completa la da /api/v1/info y /api/v1/pair.
func certPrefix(fingerprintHex string) string {
	if len(fingerprintHex) <= 16 {
		return fingerprintHex
	}
	return fingerprintHex[:16]
}
