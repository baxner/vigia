package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.ScanResult
import kotlinx.coroutines.flow.Flow
import java.time.Instant

// Contrato del historial local de la app: qué equipos se han visto en cada
// red, desde cuándo, si siguen conectados, qué les pasó (eventos) y cómo los
// personalizó el usuario. La UI solo habla con [HistoryRepository]; la
// implementación persistente (Room) vive aparte y se elige en [HistoryModule].

/**
 * Una red, para separar el historial de casa del de otra ubicación. SSID (si
 * Android lo deja leer) + router + subred: sin el SSID, dos casas con el
 * router por defecto (192.168.0.1/24) serían la misma red.
 */
data class NetworkKey(val value: String) {
    companion object {
        fun of(ssid: String?, gateway: String?, subnet: String?): NetworkKey =
            NetworkKey(listOf(ssid ?: "?", gateway ?: "?", subnet ?: "?").joinToString("|"))

        /**
         * Subred de un CIDR de interfaz como la escribe el motor:
         * "192.168.0.105/24" → "192.168.0.0/24". null si no se entiende.
         */
        fun subnetOf(cidr: String?): String? {
            val (ip, prefixText) = cidr?.split('/')?.takeIf { it.size == 2 }?.let { it[0] to it[1] } ?: return null
            val prefix = prefixText.toIntOrNull()?.takeIf { it in 0..32 } ?: return null
            val octets = ip.split('.').mapNotNull { it.toIntOrNull() }.takeIf { it.size == 4 } ?: return null
            val value = octets.fold(0L) { acc, o -> (acc shl 8) or (o.toLong() and 0xFF) }
            val mask = if (prefix == 0) 0L else (0xFFFFFFFFL shl (32 - prefix)) and 0xFFFFFFFFL
            val net = value and mask
            return "${(net shr 24) and 0xFF}.${(net shr 16) and 0xFF}.${(net shr 8) and 0xFF}.${net and 0xFF}/$prefix"
        }
    }
}

/** Lo que el usuario le cambia a mano a un equipo (pantalla Editar). null = automático. */
data class DeviceCustomization(
    val name: String? = null,
    /** id del catálogo de tipos de la UI (p. ej. "smart_bulb"); null = el que dedujo el motor. */
    val typeId: String? = null,
    val notes: String? = null,
    val location: String? = null,
    /** Equipo a vigilar: el agente avisará cuando se desconecte o vuelva. */
    val important: Boolean = false,
)

/** Un equipo conocido en una red, conectado ahora o no. */
data class KnownDevice(
    /** id interno estable: sobrevive a cambios de IP. */
    val id: Long,
    val network: NetworkKey,
    /** MAC si alguna vez se supo (NetBIOS, API del equipo, agente). */
    val mac: String?,
    val lastIp: String,
    val firstSeen: Instant,
    val lastSeen: Instant,
    /** Apareció en el último escaneo de su red. */
    val online: Boolean,
    /** Si [online]: desde cuándo está conectado sin interrupción. */
    val onlineSince: Instant?,
    /** El último [Device] del motor (identidad, puertos, servicios…). */
    val last: Device,
    val custom: DeviceCustomization = DeviceCustomization(),
)

enum class DeviceEventType {
    /** Primera vez que se ve en esta red. */
    FIRST_SEEN,
    /** Volvió tras estar sin conexión. */
    CAME_ONLINE,
    /** Dejó de verse (ver la regla de gracia en el repositorio). */
    WENT_OFFLINE,
    /** Mismo equipo, IP distinta; [DeviceEvent.detail] = "ip anterior → ip nueva". */
    IP_CHANGED,
}

data class DeviceEvent(
    val deviceId: Long,
    val type: DeviceEventType,
    val at: Instant,
    val detail: String? = null,
)

interface HistoryRepository {
    /**
     * Registra un escaneo completo de [network]: reconcilia cada equipo con
     * los conocidos (mismo equipo aunque cambie de IP), actualiza estados y
     * devuelve los eventos que generó, en orden.
     */
    suspend fun recordScan(network: NetworkKey, result: ScanResult, at: Instant): List<DeviceEvent>

    /** Todos los equipos conocidos de [network], conectados y sin conexión. */
    fun observeDevices(network: NetworkKey): Flow<List<KnownDevice>>

    fun observeDevice(id: Long): Flow<KnownDevice?>

    /** Cronología de un equipo, del más reciente al más antiguo. */
    fun observeEvents(deviceId: Long): Flow<List<DeviceEvent>>

    suspend fun updateCustomization(id: Long, custom: DeviceCustomization)

    /** "Borrar": olvida el equipo y sus eventos (si vuelve, será nuevo). */
    suspend fun forget(id: Long)
}
