package com.baxner.vigia.data.agent

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceEvent
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.ScanResult
import java.time.Instant
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * [HistoryRepository] contra el agente 24/7 (ver `docs/agente-api.md`), la
 * implementación que usa [com.baxner.vigia.ui.scan.ScanViewModel] cuando el
 * teléfono está en "Red monitoreada". `GET` inicial por red (una sola vez
 * por red mientras viva esta instancia, ver [loadedNetworkKeys]) + eventos en
 * vivo por SSE, todo cacheado en memoria para que la UI no parpadee entre el
 * `GET` y el primer evento (ver el encargo, punto 2).
 *
 * [recordScan] no hace nada: el agente escanea solo (la misma razón por la
 * que la gracia de WENT_OFFLINE no se reimplementa aquí — ya la aplicó el
 * agente antes de mandar el evento). "Escanear" desde la UI es [requestScan],
 * que [com.baxner.vigia.ui.scan.ScanViewModel] usa en vez de [recordScan].
 *
 * Vive mientras el teléfono esté en esa red y el agente conteste: la crea y
 * la destruye [AgentModeController], que es quien cancela [scope] (y con él,
 * el lector SSE) al salir de "Red monitoreada".
 */
class AgentHistoryRepository(
    private val api: AgentApiClient,
    stream: AgentEventStream,
    private val scope: CoroutineScope,
) : HistoryRepository {
    private val devicesById = MutableStateFlow<Map<Long, KnownDevice>>(emptyMap())
    private val deviceIdsByNetwork = MutableStateFlow<Map<String, List<Long>>>(emptyMap())
    private val eventsById = MutableStateFlow<Map<Long, List<DeviceEvent>>>(emptyMap())
    private val loadedNetworkKeys: MutableSet<String> = java.util.concurrent.ConcurrentHashMap.newKeySet()
    private val loadedEventsForDeviceIds: MutableSet<Long> = java.util.concurrent.ConcurrentHashMap.newKeySet()

    private val _lastScanEventAt = MutableStateFlow<Instant?>(null)

    /** Cuándo llegó el último `event: scan` del stream (de cualquier red). Lo usa [com.baxner.vigia.ui.scan.ScanViewModel] para saber cuándo un escaneo pedido con [requestScan] terminó. */
    val lastScanEventAt: StateFlow<Instant?> = _lastScanEventAt.asStateFlow()

    // Sin suscriptores, un `event: alert` se descarta (no hay replay): por eso
    // AlertsRepository se suscribe primero y se pone al día por REST después
    // (onSubscription). El búfer solo cubre ráfagas mientras procesa.
    private val _newAlerts = MutableSharedFlow<AlertDto>(extraBufferCapacity = 16)

    /** Alertas nuevas del stream en vivo (ver el contrato § "Alertas v1.1" → "Stream", y el encargo, punto 3). */
    val newAlerts: SharedFlow<AlertDto> = _newAlerts.asSharedFlow()

    init {
        scope.launch {
            stream.events().collect { event ->
                when (event) {
                    is AgentStreamEvent.DeviceChanged -> applyDeviceChanged(event)
                    is AgentStreamEvent.ScanFinished -> {
                        _lastScanEventAt.value = Instant.now()
                        refreshNetwork(event.networkKey)
                    }
                    is AgentStreamEvent.AlertReceived -> _newAlerts.tryEmit(event.alert)
                }
            }
        }
    }

    override suspend fun recordScan(network: NetworkKey, result: ScanResult, at: Instant): List<DeviceEvent> = emptyList()

    override fun observeDevices(network: NetworkKey): Flow<List<KnownDevice>> {
        val key = network.value
        if (loadedNetworkKeys.add(key)) scope.launch { refreshNetwork(key) }
        return combine(devicesById, deviceIdsByNetwork) { byId, idx ->
            idx[key].orEmpty().mapNotNull(byId::get).sortedBy { ipSortKey(it.lastIp) }
        }
    }

    override fun observeDevice(id: Long): Flow<KnownDevice?> = devicesById.map { it[id] }

    override fun observeEvents(deviceId: Long): Flow<List<DeviceEvent>> {
        if (loadedEventsForDeviceIds.add(deviceId)) {
            scope.launch {
                api.events(deviceId).onSuccess { dtos ->
                    eventsById.update { it + (deviceId to dtos.map(DeviceEventDto::toDomain)) }
                }
            }
        }
        return eventsById.map { it[deviceId].orEmpty() }
    }

    override suspend fun updateCustomization(id: Long, custom: DeviceCustomization) {
        api.patchCustomization(id, DeviceCustomizationPatch.replaceAll(custom)).onSuccess { dto ->
            applyKnownDevice(dto.toDomain())
        }
    }

    override suspend fun forget(id: Long) {
        api.forget(id).onSuccess {
            devicesById.update { it - id }
            deviceIdsByNetwork.update { current -> current.mapValues { (_, ids) -> ids - id } }
            eventsById.update { it - id }
            loadedEventsForDeviceIds -= id
        }
    }

    /** "Escanear" en Red monitoreada (ver el encargo, punto 3): le pide al agente un escaneo ya; el progreso lo marca [lastScanEventAt] cuando el stream avisa que terminó. */
    suspend fun requestScan(): Result<Boolean> = api.startScan()

    /** `GET /api/v1/security` (ver el contrato § "Seguridad" y `data/security/SecurityRepository.kt`): delega en [api], igual que [requestScan]. */
    suspend fun security(): Result<SecurityReport> = api.security()

    /** "Ya lo sé, es a propósito" (v1.3, ver el contrato § "Hallazgos reconocidos"): delega en [api], igual que [security]. */
    suspend fun acknowledgeFinding(findingId: String): Result<Unit> = api.acknowledgeFinding(findingId)

    /** "Deshacer" (v1.3): delega en [api]. */
    suspend fun unacknowledgeFinding(findingId: String): Result<Unit> = api.unacknowledgeFinding(findingId)

    // ---- Internet (v1.4, ver el contrato § "Internet" y `ui/internet/InternetViewModel.kt`) ----

    suspend fun internet(): Result<InternetStatusDto> = api.internet()

    suspend fun internetSeries(range: InternetSeriesRange): Result<List<InternetSeriesPointDto>> = api.internetSeries(range)

    suspend fun startSpeedTest(): Result<Boolean> = api.startSpeedTest()

    suspend fun internetSettings(): Result<InternetSettingsDto> = api.internetSettings()

    suspend fun putInternetSettings(settings: InternetSettingsDto): Result<InternetSettingsDto> = api.putInternetSettings(settings)

    private fun applyDeviceChanged(event: AgentStreamEvent.DeviceChanged) {
        applyKnownDevice(event.device.toDomain())
        eventsById.update { current ->
            val existing = current[event.event.deviceId].orEmpty()
            current + (event.event.deviceId to (listOf(event.event.toDomain()) + existing))
        }
    }

    private fun applyKnownDevice(device: KnownDevice) {
        devicesById.update { it + (device.id to device) }
        deviceIdsByNetwork.update { current ->
            val key = device.network.value
            val ids = current[key].orEmpty()
            if (device.id in ids) current else current + (key to ids + device.id)
        }
    }

    private suspend fun refreshNetwork(key: String) {
        api.devicesForNetwork(key).onSuccess { dtos ->
            val domain = dtos.map(KnownDeviceDto::toDomain)
            devicesById.update { it + domain.associateBy(KnownDevice::id) }
            deviceIdsByNetwork.update { it + (key to domain.map(KnownDevice::id)) }
        }.onFailure {
            android.util.Log.w("VigiaAgent", "devicesForNetwork key=$key falló: $it")
            // Que el próximo observeDevices lo vuelva a intentar en vez de
            // dejar la lista vacía para siempre.
            loadedNetworkKeys.remove(key)
        }
    }
}

/**
 * Clave para ordenar IPv4 numéricamente, igual que
 * [com.baxner.vigia.data.history.RoomHistoryRepository] (duplicado a propósito:
 * es privado ahí y no vale la pena acoplar los dos repositorios por 5 líneas).
 */
private fun ipSortKey(ip: String): Long =
    ip.split('.').mapNotNull { it.toIntOrNull() }.takeIf { it.size == 4 }
        ?.fold(0L) { acc, octet -> (acc shl 8) or (octet.toLong() and 0xFF) }
        ?: Long.MAX_VALUE
