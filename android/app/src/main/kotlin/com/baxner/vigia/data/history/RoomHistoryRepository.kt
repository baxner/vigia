package com.baxner.vigia.data.history

import androidx.room.withTransaction
import com.baxner.vigia.data.model.ScanResult
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.withContext

/**
 * Historial persistente con Room: la implementación real de [HistoryRepository]
 * (la que entrega [HistoryModule] fuera del modo demo). La reconciliación
 * ([DeviceReconciler]) y la fusión de estado/eventos ([DeviceHistoryEngine])
 * son puras; esta clase solo las conecta con las filas de la base.
 */
class RoomHistoryRepository(private val db: HistoryDatabase) : HistoryRepository {
    private val networkDao get() = db.networkDao()
    private val deviceDao get() = db.deviceDao()
    private val eventDao get() = db.eventDao()

    override suspend fun recordScan(network: NetworkKey, result: ScanResult, at: Instant): List<DeviceEvent> =
        withContext(Dispatchers.IO) {
            db.withTransaction {
                networkDao.touch(network.value, at)

                val rows = deviceDao.listByNetwork(network.value).associateBy { it.id }
                val known = rows.values.map { it.toDomain() }
                val matches = DeviceReconciler.reconcile(known, result.devices)

                val events = mutableListOf<DeviceEvent>()
                val matchedIds = mutableSetOf<Long>()

                for (match in matches) {
                    val knownDevice = match.known
                    if (knownDevice == null) {
                        val id = deviceDao.insert(DeviceEntity.firstSeen(network, match.scanned, at))
                        events += DeviceEvent(id, DeviceEventType.FIRST_SEEN, at)
                    } else {
                        matchedIds += knownDevice.id
                        val outcome = DeviceHistoryEngine.seen(knownDevice, match.scanned, at)
                        deviceDao.update(rows.getValue(knownDevice.id).mergedWith(outcome.device, outcome.offlineMisses))
                        events += outcome.events
                    }
                }

                for (k in known) {
                    if (k.id in matchedIds) continue
                    val row = rows.getValue(k.id)
                    val outcome = DeviceHistoryEngine.missing(k, row.offlineMisses, at)
                    deviceDao.update(row.mergedWith(outcome.device, outcome.offlineMisses))
                    events += outcome.events
                }

                if (events.isNotEmpty()) eventDao.insertAll(events.map { it.toEntity() })
                events
            }
        }

    override fun observeDevices(network: NetworkKey): Flow<List<KnownDevice>> =
        deviceDao.observeByNetwork(network.value)
            .map { rows -> rows.map { it.toDomain() }.sortedBy { ipSortKey(it.lastIp) } }
            .flowOn(Dispatchers.IO)

    override fun observeDevice(id: Long): Flow<KnownDevice?> =
        deviceDao.observeById(id)
            .map { it?.toDomain() }
            .flowOn(Dispatchers.IO)

    override fun observeEvents(deviceId: Long): Flow<List<DeviceEvent>> =
        eventDao.observeForDevice(deviceId)
            .map { rows -> rows.map { it.toDomain() } }
            .flowOn(Dispatchers.IO)

    override suspend fun updateCustomization(id: Long, custom: DeviceCustomization) {
        withContext(Dispatchers.IO) {
            deviceDao.updateCustomization(id, custom.name, custom.typeId, custom.notes, custom.location, custom.important)
        }
    }

    override suspend fun forget(id: Long) {
        withContext(Dispatchers.IO) {
            db.withTransaction {
                eventDao.deleteForDevice(id)
                deviceDao.delete(id)
            }
        }
    }
}

/**
 * Clave para ordenar IPv4 numéricamente ("192.168.0.9" antes que
 * "192.168.0.29"): como texto saldría al revés. Una IP rara (IPv6, vacía)
 * no revienta el orden: va al final.
 */
private fun ipSortKey(ip: String): Long =
    ip.split('.').mapNotNull { it.toIntOrNull() }.takeIf { it.size == 4 }
        ?.fold(0L) { acc, octet -> (acc shl 8) or (octet.toLong() and 0xFF) }
        ?: Long.MAX_VALUE
