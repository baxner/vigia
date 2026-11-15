//go:build !darwin && !linux && !android

package core

import "syscall"

// reusePortControl: no-op fuera de Darwin/Linux/Android (p. ej. Windows, que
// no tiene SO_REUSEPORT). Sin él el bind puede fallar si algo más ya escucha
// ese puerto, pero eso ya lo maneja el llamador (discoverTuya sigue sin Tuya
// en silencio, ver tuya.go) — no hace falta nada más aquí.
func reusePortControl(_, _ string, _ syscall.RawConn) error {
	return nil
}
