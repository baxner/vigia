package history

import (
	"time"

	"github.com/baxner/vigia/core"
)

// graceWindow y graceMisses: regla de gracia de WENT_OFFLINE. Un escaneo
// puede no ver un teléfono dormido. Por eso un conocido que no aparece queda
// online=false en la lista desde ya (ver Missing) pero el evento recién se
// registra ("se compromete") si faltó en dos escaneos seguidos de su red o
// si pasaron más de 10 min desde su lastSeen. Mismos valores que
// DeviceHistoryEngine.kt.
const (
	graceWindow = 10 * time.Minute
	graceMisses = 2
)

// Outcome es el resultado de fusionar el estado anterior de un KnownDevice
// con lo que pasó en este escaneo (visto de nuevo, o ausente): el device ya
// actualizado, su offlineMisses (contabilidad interna de la gracia, para que
// quien persiste se la devuelva en la próxima llamada a Missing) y los
// eventos que corresponden.
type Outcome struct {
	Device        KnownDevice
	OfflineMisses int
	Events        []DeviceEvent
}

// Seen: previous se volvió a ver en este escaneo, reportado como scanned.
// Puerto de DeviceHistoryEngine.seen: hasta aquí solo pueden surgir
// CAME_ONLINE (al volver de un WENT_OFFLINE ya comprometido) e IP_CHANGED;
// WENT_OFFLINE siempre se decide en Missing.
func Seen(previous KnownDevice, scanned core.Device, at time.Time) Outcome {
	var events []DeviceEvent
	var onlineSince *time.Time
	switch {
	case previous.Online:
		// Seguía conectado desde antes: sin interrupción que reportar.
		onlineSince = previous.OnlineSince
	case previous.OnlineSince != nil:
		// Dentro de la gracia (nunca se comprometió WENT_OFFLINE): vuelve en silencio.
		onlineSince = previous.OnlineSince
	default:
		// Se había comprometido que estaba sin conexión: esto sí es un regreso real.
		t := at
		onlineSince = &t
		events = append(events, DeviceEvent{DeviceID: previous.ID, Type: CameOnline, At: at})
	}

	if previous.LastIP != scanned.IP {
		detail := previous.LastIP + " → " + scanned.IP
		events = append(events, DeviceEvent{DeviceID: previous.ID, Type: IPChanged, At: at, Detail: &detail})
	}

	merged := previous
	// "pegajosa": una MAC ya conocida no se reemplaza aunque este escaneo no la resuelva.
	if merged.MAC == nil && scanned.MAC != "" {
		mac := scanned.MAC
		merged.MAC = &mac
	}
	merged.LastIP = scanned.IP
	merged.LastSeen = at
	merged.Online = true
	merged.OnlineSince = onlineSince
	merged.Last = scanned

	return Outcome{Device: merged, OfflineMisses: 0, Events: events}
}

// Missing: no apareció previous en este escaneo de su red. Puerto de
// DeviceHistoryEngine.missing.
//
// El evento WENT_OFFLINE se fecha con el último lastSeen real del equipo, no
// con el momento en que el escaneo recién lo confirma (que, por la gracia,
// puede ser minutos después): es cuando de verdad se fue, no cuando nos
// enteramos. Solo se compromete (y se emite el evento) la primera vez que se
// cruza el umbral: si ya estaba comprometido (previous.OnlineSince == nil de
// un escaneo anterior), no se repite.
func Missing(previous KnownDevice, offlineMissesBefore int, at time.Time) Outcome {
	misses := offlineMissesBefore + 1
	graceExpired := misses >= graceMisses || at.Sub(previous.LastSeen) > graceWindow

	commitsNow := graceExpired && previous.OnlineSince != nil
	var events []DeviceEvent
	if commitsNow {
		events = append(events, DeviceEvent{DeviceID: previous.ID, Type: WentOffline, At: previous.LastSeen})
	}

	var onlineSince *time.Time
	if !graceExpired {
		onlineSince = previous.OnlineSince
	}

	merged := previous
	merged.Online = false
	merged.OnlineSince = onlineSince

	return Outcome{Device: merged, OfflineMisses: misses, Events: events}
}
