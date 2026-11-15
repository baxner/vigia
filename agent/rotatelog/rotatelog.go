// Package rotatelog es un io.Writer que escribe en un archivo y lo rota
// por tamaño (ver docs/agente-api.md, "Rotación de logs" y
// cmd/vigia-agent/run.go): sin él, ~/Library/Logs/Vigia/agent.log crece
// sin límite durante toda la vida del LaunchAgent. Sin librerías nuevas:
// solo os/sync de la biblioteca estándar.
package rotatelog

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

// DefaultMaxBytes/DefaultKeep: los valores del contrato (ver
// docs/agente-api.md): rotar a los 5 MB, conservar 3 archivos viejos
// además del que está activo.
const (
	DefaultMaxBytes int64 = 5 * 1024 * 1024
	DefaultKeep           = 3
)

// Writer escribe en un archivo (path) y lo rota cuando pasaría de
// maxBytes: lo cierra, desplaza path.1→path.2… hasta keep (el más viejo
// se borra) y abre uno nuevo en path. Seguro para llamar Write desde
// varias goroutines a la vez — slog puede hacerlo si dos escaneos
// solapan sus logs — un solo sync.Mutex protege tanto la escritura como
// la rotación, así que nunca se entrelazan dos escrituras ni una
// escritura con una rotación a medio hacer.
type Writer struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	f        *os.File
	size     int64
}

// Open abre (o crea) path para agregar al final y arma un Writer que lo
// rota pasado maxBytes, conservando keep archivos viejos. maxBytes <= 0
// usa DefaultMaxBytes; keep < 0 usa DefaultKeep (keep == 0 es válido:
// rotar sin conservar ninguno de los viejos, solo truncar).
func Open(path string, maxBytes int64, keep int) (*Writer, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if keep < 0 {
		keep = DefaultKeep
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("leer tamaño de %s: %w", path, err)
	}
	return &Writer{path: path, maxBytes: maxBytes, keep: keep, f: f, size: info.Size()}, nil
}

// Write implementa io.Writer: si agregar p haría que el archivo pasara de
// maxBytes, rota primero (nunca rota un archivo recién creado y vacío,
// para no generar un agent.log.1 vacío con la primera línea del
// proceso). Una sola línea más grande que maxBytes se escribe igual
// (entera, nunca cortada a la mitad) — la próxima escritura es la que
// dispara la rotación.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	if w.f == nil {
		return 0, os.ErrClosed
	}

	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate desplaza los backups (path.(keep-1) se pierde si ya había keep) y
// abre un archivo nuevo y vacío en path con permisos 0600. En Unix mantiene
// w.f abierto hasta tener listo el nuevo descriptor para no dejar el Writer
// inutilizado si falla un Rename.
func (w *Writer) rotate() error {
	if err := shiftBackups(w.path, w.keep); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return w.rotateWindows()
	}

	if w.keep > 0 {
		if err := os.Rename(w.path, backupPath(w.path, 1)); err != nil {
			return fmt.Errorf("rotar %s → %s: %w", w.path, backupPath(w.path, 1), err)
		}
	} else {
		// keep == 0: sin backups, solo truncar.
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("truncar %s: %w", w.path, err)
		}
	}

	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		if w.keep > 0 {
			_ = os.Rename(backupPath(w.path, 1), w.path)
		}
		return fmt.Errorf("reabrir %s tras rotar: %w", w.path, err)
	}
	old := w.f
	w.f = f
	w.size = 0
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (w *Writer) rotateWindows() error {
	if w.f != nil {
		if err := w.f.Close(); err != nil {
			return fmt.Errorf("cerrar %s antes de rotar: %w", w.path, err)
		}
		w.f = nil
	}

	reopenOnErr := func(rotErr error) error {
		if f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			w.f = f
			if info, err := f.Stat(); err == nil {
				w.size = info.Size()
			}
		}
		return rotErr
	}

	if w.keep > 0 {
		if err := os.Rename(w.path, backupPath(w.path, 1)); err != nil {
			return reopenOnErr(fmt.Errorf("rotar %s → %s: %w", w.path, backupPath(w.path, 1), err))
		}
	} else {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return reopenOnErr(fmt.Errorf("truncar %s: %w", w.path, err))
		}
	}

	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		if w.keep > 0 {
			_ = os.Rename(backupPath(w.path, 1), w.path)
		}
		return reopenOnErr(fmt.Errorf("reabrir %s tras rotar: %w", w.path, err))
	}
	w.f = f
	w.size = 0
	return nil
}

func shiftBackups(path string, keep int) error {
	if keep <= 0 {
		return nil
	}
	oldest := backupPath(path, keep)
	_ = os.Remove(oldest) // si no existía, no pasa nada: Remove ya devuelve ese error y se ignora a propósito.
	for i := keep - 1; i >= 1; i-- {
		from, to := backupPath(path, i), backupPath(path, i+1)
		if _, err := os.Stat(from); err == nil {
			if err := os.Rename(from, to); err != nil {
				return fmt.Errorf("rotar %s → %s: %w", from, to, err)
			}
		}
	}
	return nil
}

// backupPath: "path.N", el N-ésimo backup rotado (1 es el más reciente).
func backupPath(path string, n int) string {
	return fmt.Sprintf("%s.%d", path, n)
}

// Close cierra el archivo activo. No rota.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
