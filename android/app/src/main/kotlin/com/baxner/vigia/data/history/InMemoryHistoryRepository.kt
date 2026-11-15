package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.ScanResult
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import java.time.Instant

/**
 * Historial en memoria (se pierde al cerrar la app). Sirve para el modo demo,
 * las capturas y los tests de UI; la app real usa la implementación
 * persistente que da [HistoryModule]. Reconciliación simple: misma MAC o,
 * sin MAC, misma IP.
 */
class InMemoryHistoryRepository : HistoryRepository {
    private val devices = MutableStateFlow<Map<Long, KnownDevice>>(emptyMap())
    private val events = MutableStateFlow<List<DeviceEvent>>(emptyList())
    private var nextId = 1L

    override suspend fun recordScan(network: NetworkKey, result: ScanResult, at: Instant): List<DeviceEvent> {
        val out = mutableListOf<DeviceEvent>()
        devices.update { current ->
            val all = current.toMutableMap()
            val known = all.values.filter { it.network == network }
            val matched = mutableSetOf<Long>()
            for (d in result.devices) {
                val match = known.firstOrNull { k ->
                    k.id !in matched && ((d.mac.isNotEmpty() && k.mac == d.mac) || (d.mac.isEmpty() && k.lastIp == d.ip))
                }
                if (match == null) {
                    val id = nextId++
                    all[id] = KnownDevice(id, network, d.mac.ifEmpty { null }, d.ip, at, at, true, at, d)
                    out += DeviceEvent(id, DeviceEventType.FIRST_SEEN, at)
                    matched += id
                } else {
                    matched += match.id
                    if (!match.online) out += DeviceEvent(match.id, DeviceEventType.CAME_ONLINE, at)
                    if (match.lastIp != d.ip) out += DeviceEvent(match.id, DeviceEventType.IP_CHANGED, at, "${match.lastIp} → ${d.ip}")
                    all[match.id] = match.copy(
                        mac = match.mac ?: d.mac.ifEmpty { null },
                        lastIp = d.ip,
                        lastSeen = at,
                        online = true,
                        onlineSince = if (match.online) match.onlineSince else at,
                        last = d,
                    )
                }
            }
            for (k in known) {
                if (k.id !in matched && k.online) {
                    all[k.id] = k.copy(online = false, onlineSince = null)
                    out += DeviceEvent(k.id, DeviceEventType.WENT_OFFLINE, at)
                }
            }
            all
        }
        events.update { it + out }
        return out
    }

    override fun observeDevices(network: NetworkKey): Flow<List<KnownDevice>> =
        devices.map { all -> all.values.filter { it.network == network }.sortedBy { it.lastIp } }

    override fun observeDevice(id: Long): Flow<KnownDevice?> = devices.map { it[id] }

    override fun observeEvents(deviceId: Long): Flow<List<DeviceEvent>> =
        events.map { all -> all.filter { it.deviceId == deviceId }.sortedByDescending { it.at } }

    override suspend fun updateCustomization(id: Long, custom: DeviceCustomization) {
        devices.update { all -> all[id]?.let { all + (id to it.copy(custom = custom)) } ?: all }
    }

    override suspend fun forget(id: Long) {
        devices.update { it - id }
        events.update { all -> all.filterNot { it.deviceId == id } }
    }
}
