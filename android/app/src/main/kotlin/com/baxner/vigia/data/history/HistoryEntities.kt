package com.baxner.vigia.data.history

import androidx.room.Embedded
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.repository.VigiaJson
import java.time.Instant
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString

/**
 * Una red vista alguna vez (ver [NetworkKey]). Sirve de ancla para los
 * equipos de [DeviceEntity] (clave foránea) y para saber desde/hasta cuándo
 * se conoce esa red, aunque hoy el contrato [HistoryRepository] no la
 * necesite directamente — [NetworkKey] le basta a la UI.
 */
@Entity(tableName = "networks")
data class NetworkEntity(
    @PrimaryKey val key: String,
    val firstSeen: Instant,
    val lastSeen: Instant,
)

/**
 * Un equipo conocido en una red. Corresponde a [KnownDevice], pero con dos
 * diferencias a propósito:
 *  - [ssdpUdn] y [strongName] son una copia indexable de los identificadores
 *    fuertes del ÚLTIMO escaneo (se recalculan cada vez desde [lastDeviceJson],
 *    igual que hace [DeviceReconciler] con `last`). A diferencia de [mac],
 *    NO son "pegajosos": si un escaneo no los vuelve a detectar, quedan en
 *    null hasta el próximo que sí. Están aparte del JSON solo para poder
 *    indexarlos/consultarlos sin deserializar.
 *  - [offlineMisses] no existe en el contrato: es contabilidad interna de
 *    [DeviceHistoryEngine] para la regla de gracia de WENT_OFFLINE.
 */
@Entity(
    tableName = "devices",
    foreignKeys = [
        ForeignKey(
            entity = NetworkEntity::class,
            parentColumns = ["key"],
            childColumns = ["networkKey"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
    indices = [
        Index("networkKey"),
        Index("networkKey", "mac"),
        Index("networkKey", "ssdpUdn"),
        Index("networkKey", "strongName"),
    ],
)
data class DeviceEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val networkKey: String,
    val mac: String?,
    val lastIp: String,
    val ssdpUdn: String?,
    val strongName: String?,
    val firstSeen: Instant,
    val lastSeen: Instant,
    val online: Boolean,
    val onlineSince: Instant?,
    val offlineMisses: Int = 0,
    /** El último [Device] del motor, serializado con [VigiaJson]. */
    val lastDeviceJson: String,
    @Embedded(prefix = "custom_") val custom: DeviceCustomization = DeviceCustomization(),
) {
    companion object {
        /** Fila para un equipo que se ve por primera vez en [network] (id = 0: Room lo asigna al insertar). */
        fun firstSeen(network: NetworkKey, scanned: Device, at: Instant): DeviceEntity = DeviceEntity(
            id = 0,
            networkKey = network.value,
            mac = scanned.mac.ifEmpty { null },
            lastIp = scanned.ip,
            ssdpUdn = scanned.extra[ExtraSsdpUdn]?.takeIf { it.isNotBlank() },
            strongName = DeviceReconciler.strongNameOf(scanned),
            firstSeen = at,
            lastSeen = at,
            online = true,
            onlineSince = at,
            offlineMisses = 0,
            lastDeviceJson = VigiaJson.encodeToString(scanned),
        )
    }
}

/** Reconstruye el [KnownDevice] del contrato a partir de la fila persistida. */
fun DeviceEntity.toDomain(): KnownDevice = KnownDevice(
    id = id,
    network = NetworkKey(networkKey),
    mac = mac,
    lastIp = lastIp,
    firstSeen = firstSeen,
    lastSeen = lastSeen,
    online = online,
    onlineSince = onlineSince,
    last = VigiaJson.decodeFromString(lastDeviceJson),
    custom = custom,
)

/**
 * Aplica sobre esta fila el resultado de fusionar su estado anterior con un
 * escaneo (ver [DeviceHistoryEngine]), conservando [DeviceEntity.id],
 * [DeviceEntity.networkKey] y [DeviceEntity.firstSeen] (no cambian) y
 * recalculando los identificadores indexables desde el nuevo `last`.
 */
fun DeviceEntity.mergedWith(device: KnownDevice, offlineMisses: Int): DeviceEntity = copy(
    mac = device.mac,
    lastIp = device.lastIp,
    ssdpUdn = device.last.extra[ExtraSsdpUdn]?.takeIf { it.isNotBlank() },
    strongName = DeviceReconciler.strongNameOf(device.last),
    lastSeen = device.lastSeen,
    online = device.online,
    onlineSince = device.onlineSince,
    offlineMisses = offlineMisses,
    lastDeviceJson = VigiaJson.encodeToString(device.last),
    custom = device.custom,
)

/** Un evento de la cronología de un equipo (ver [DeviceEvent]). */
@Entity(
    tableName = "device_events",
    foreignKeys = [
        ForeignKey(
            entity = DeviceEntity::class,
            parentColumns = ["id"],
            childColumns = ["deviceId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
    indices = [Index("deviceId")],
)
data class DeviceEventEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val deviceId: Long,
    val type: DeviceEventType,
    val at: Instant,
    val detail: String? = null,
)

fun DeviceEvent.toEntity(): DeviceEventEntity = DeviceEventEntity(
    deviceId = deviceId,
    type = type,
    at = at,
    detail = detail,
)

fun DeviceEventEntity.toDomain(): DeviceEvent = DeviceEvent(
    deviceId = deviceId,
    type = type,
    at = at,
    detail = detail,
)
