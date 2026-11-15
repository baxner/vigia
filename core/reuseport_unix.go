//go:build darwin || linux || android

package core

import (
	"runtime"
	"syscall"
)

// soReusePortLinux / soReusePortDarwin: valor crudo de SO_REUSEPORT para
// setsockopt(SOL_SOCKET, ...). El paquete syscall estándar lo expone para
// Darwin pero NO para Linux en amd64/386/arm (sí para arm64 y varias otras:
// un vacío histórico de esa parte de la stdlib de Go, no del kernel — el
// valor real es el mismo 15 en cualquier arquitectura Linux/Android), así que
// se pone a mano para no depender de para qué arquitectura se compile.
const (
	soReusePortLinux  = 0xf   // include/uapi/asm-generic/socket.h
	soReusePortDarwin = 0x200 // sys/socket.h (BSD)
)

// reusePortControl se pasa a net.ListenConfig.Control para que el socket de
// escucha Tuya (UDP 6666/6667, ver tuya.go) pueda convivir con otro proceso
// ya escuchando ahí — típicamente nosotros mismos en una corrida anterior, o
// la app oficial Tuya/Smart Life si corre en la misma máquina — en vez de que
// el bind falle. SO_REUSEADDR ya cubre el caso típico (socket en TIME_WAIT);
// SO_REUSEPORT (BSD/Linux; no existe en Windows, ver reuseport_other.go)
// además deja que dos procesos escuchen el mismo puerto a la vez.
//
// Todo best-effort: un setsockopt que falla no corta nada, el bind sigue su
// curso solo (si de verdad no se puede abrir el puerto, eso lo decide
// ListenPacket más arriba, no esta función).
func reusePortControl(_, _ string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort(), 1)
	})
}

func soReusePort() int {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return soReusePortDarwin
	}
	return soReusePortLinux // linux y android
}
