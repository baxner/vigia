package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.Device
import java.time.Duration
import java.time.Instant

/**
 * Qué le pasa a un [KnownDevice] ya reconciliado (ver [DeviceReconciler])
 * cuando se registra un escaneo: fusiona el estado anterior con lo nuevo (o
 * con su ausencia) y decide qué [DeviceEvent] corresponden. Pura — nada de
 * Room — para poder probarla con JUnit; [RoomHistoryRepository] es quien le
 * da de comer el `offlineMisses` persistido y guarda lo que devuelve.
 *
 * Regla de gracia de WENT_OFFLINE: un escaneo puede no ver un teléfono
 * dormido. Por eso un conocido que no aparece queda `online = false` en la
 * lista de una (ver [missing]) pero el evento recién se registra ("se
 * compromete") si faltó en dos escaneos seguidos de su red o si pasaron más
 * de 10 minutos desde su `lastSeen`. Hasta que eso pase, [KnownDevice.onlineSince]
 * se conserva tal cual; si reaparece antes, no hay ni WENT_OFFLINE ni
 * CAME_ONLINE. Una vez comprometido (onlineSince queda en null), reaparecer
 * sí dispara CAME_ONLINE con onlineSince = ahora.
 *
 * El evento WENT_OFFLINE se fecha con el último `lastSeen` real del equipo,
 * no con el momento en que el escaneo recién lo confirma (que, por la
 * gracia, puede ser minutos después): es cuando de verdad se fue, no cuando
 * nos enteramos. "Sin conexión desde" en la UI es, por eso, el `lastSeen`
 * del [KnownDevice], no la fecha del evento.
 */
object DeviceHistoryEngine {
    private val GraceWindow: Duration = Duration.ofMinutes(10)
    private const val GraceMisses = 2

    /** [device] con [offlineMisses] ya actualizado, y los eventos que resultaron de este escaneo. */
    data class Outcome(val device: KnownDevice, val offlineMisses: Int, val events: List<DeviceEvent>)

    /** Se volvió a ver a [previous] en este escaneo, ahora reportado como [scanned]. */
    fun seen(previous: KnownDevice, scanned: Device, at: Instant): Outcome {
        val events = mutableListOf<DeviceEvent>()
        val onlineSince = when {
            // Seguía conectado desde antes: sin interrupción que reportar.
            previous.online -> previous.onlineSince
            // Dentro de la gracia (nunca se comprometió WENT_OFFLINE): vuelve en silencio.
            previous.onlineSince != null -> previous.onlineSince
            // Se había comprometido que estaba sin conexión: esto sí es un regreso real.
            else -> at.also { events += DeviceEvent(previous.id, DeviceEventType.CAME_ONLINE, at) }
        }
        if (previous.lastIp != scanned.ip) {
            events += DeviceEvent(previous.id, DeviceEventType.IP_CHANGED, at, "${previous.lastIp} → ${scanned.ip}")
        }
        val merged = previous.copy(
            // "pegajoso": una MAC ya conocida no se reemplaza aunque este escaneo no la resuelva.
            mac = previous.mac ?: scanned.mac.ifEmpty { null },
            lastIp = scanned.ip,
            lastSeen = at,
            online = true,
            onlineSince = onlineSince,
            last = scanned,
        )
        return Outcome(merged, offlineMisses = 0, events = events)
    }

    /** No apareció [previous] en este escaneo de su red. */
    fun missing(previous: KnownDevice, offlineMissesBefore: Int, at: Instant): Outcome {
        val misses = offlineMissesBefore + 1
        val graceExpired = misses >= GraceMisses || Duration.between(previous.lastSeen, at) > GraceWindow

        // Solo se compromete (y se emite el evento) la primera vez que se cruza el umbral:
        // si ya estaba comprometido (onlineSince == null de un escaneo anterior), no se repite.
        // Se fecha con `previous.lastSeen` (la última vez que de verdad se vio), no con `at`
        // (el momento en que recién nos dimos cuenta, que puede ser minutos u horas después).
        val commitsNow = graceExpired && previous.onlineSince != null
        val events = if (commitsNow) {
            listOf(DeviceEvent(previous.id, DeviceEventType.WENT_OFFLINE, previous.lastSeen))
        } else {
            emptyList()
        }
        val onlineSince = if (graceExpired) null else previous.onlineSince

        val merged = previous.copy(online = false, onlineSince = onlineSince)
        return Outcome(merged, offlineMisses = misses, events = events)
    }
}
