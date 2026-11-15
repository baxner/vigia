package com.baxner.vigia.ui.scan

import com.baxner.vigia.data.agent.AgentApiException
import com.baxner.vigia.data.agent.AgentHistoryRepository
import com.baxner.vigia.data.agent.AgentModeController
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceEvent
import com.baxner.vigia.data.history.HistoryModule
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.InMemoryHistoryRepository
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.Job
import kotlinx.coroutines.withTimeoutOrNull
import java.time.Instant
import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.R
import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.NetworkInfoProvider
import com.baxner.vigia.data.network.WifiScanner
import com.baxner.vigia.data.repository.PortScanWork
import com.baxner.vigia.data.repository.PortScanWorker
import com.baxner.vigia.data.widget.WidgetSummaryUpdater
import com.baxner.vigia.data.repository.ScanRepository
import androidx.work.ExistingWorkPolicy
import androidx.work.WorkInfo
import androidx.work.WorkManager
import com.baxner.vigia.data.repository.isUserCancelled
import com.baxner.vigia.demo.DemoMode
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Estado del escaneo de red en curso (o no). */
sealed interface ScanStatus {
    data object Idle : ScanStatus
    data class Scanning(val stage: String, val done: Long, val total: Long) : ScanStatus
}

data class ScanUiState(
    val network: CurrentNetwork? = null,
    val status: ScanStatus = ScanStatus.Idle,
    val result: ScanResult? = null,
    val errorMessage: String? = null,
)

/**
 * Escaneo completo de puertos de un equipo, disparado desde su detalle. Vive
 * en WorkManager (ver [PortScanWorker]), no en la pantalla: sigue aunque se
 * salga, y el último resultado se vuelve a ver al entrar.
 */
sealed interface PortScanState {
    data object Idle : PortScanState
    data object Scanning : PortScanState
    /** [ip] la que tenía el equipo al escanearlo; [finishedAt] null solo en datos de muestra. */
    data class Done(val ip: String, val ports: List<Int>, val finishedAt: Instant? = null) : PortScanState
    data class Failed(val ip: String, val message: String) : PortScanState
}

/**
 * [PortScanState] a partir de lo que WorkManager guarda bajo el nombre único
 * de un equipo. Al encolar con KEEP, WorkManager borra los trabajos ya
 * terminados de ese nombre, así que normalmente hay uno solo.
 */
internal fun portScanStateOf(infos: List<WorkInfo>): PortScanState {
    val info = infos.firstOrNull { !it.state.isFinished } ?: infos.lastOrNull() ?: return PortScanState.Idle
    val out = info.outputData
    val ip = out.getString(PortScanWork.KEY_IP).orEmpty()
    return when (info.state) {
        WorkInfo.State.ENQUEUED, WorkInfo.State.RUNNING, WorkInfo.State.BLOCKED -> PortScanState.Scanning
        WorkInfo.State.SUCCEEDED -> PortScanState.Done(
            ip = ip,
            ports = out.getIntArray(PortScanWork.KEY_PORTS)?.toList().orEmpty(),
            finishedAt = out.getLong(PortScanWork.KEY_FINISHED_AT, 0L).takeIf { it > 0 }?.let(Instant::ofEpochMilli),
        )
        WorkInfo.State.FAILED -> PortScanState.Failed(ip, out.getString(PortScanWork.KEY_ERROR).orEmpty())
        WorkInfo.State.CANCELLED -> PortScanState.Idle
    }
}

/**
 * La tarjeta "Vigilancia 24/7" con la hora del último escaneo al día: el
 * controlador la lee una vez (al evaluar la red) y después la adelanta cada
 * `event: scan` del stream, que llega con cada escaneo del agente, los
 * periódicos y los pedidos con "Escanear".
 */
internal fun AgentPanelState.withScanEventAt(scanEventAt: Instant?): AgentPanelState {
    if (this !is AgentPanelState.Linked || (!monitoringCurrentNetwork && !viewingRemote) || scanEventAt == null) return this
    val current = lastScanAt
    return if (current == null || scanEventAt.isAfter(current)) copy(lastScanAt = scanEventAt) else this
}

/**
 * Estado compartido por Panel, Dispositivos y Detalle: la red actual, el
 * escaneo (con progreso) y su último resultado. Vive a nivel de la Activity
 * (ver [com.baxner.vigia.ui.navigation.VigiaApp]) para que las tres pantallas
 * vean siempre el mismo escaneo sin volver a pedirlo.
 */
class ScanViewModel(application: Application) : AndroidViewModel(application) {
    private val networkInfoProvider = NetworkInfoProvider(application)

    // Solo para el MulticastLock durante el escaneo de red (mDNS/SSDP).
    private val multicastHolder = WifiScanner(application)

    private val repository = ScanRepository()

    // Agente 24/7 (ver data/agent/): un singleton de proceso, para
    // que el SSE sobreviva a que este ViewModel se recree. Decide si la app
    // está en "Red monitoreada" (ver el init de abajo) y, si lo está, da el
    // HistoryRepository que habla con el agente en vez del motor local.
    private val agentController: AgentModeController = AgentModule.controller(application)
    @OptIn(ExperimentalCoroutinesApi::class)
    val agentPanelState: StateFlow<AgentPanelState> = combine(
        agentController.panelState,
        agentController.activeRepository.flatMapLatest { it?.lastScanEventAt ?: flowOf(null) },
    ) { state, scanEventAt -> state.withScanEventAt(scanEventAt) }
        .stateIn(viewModelScope, SharingStarted.Eagerly, agentController.panelState.value)

    private val workManager = WorkManager.getInstance(application)

    // Historial local (equipos conocidos por red, eventos, personalización).
    // En modo demo se cambia por uno en memoria para no ensuciar el real.
    // En "Red monitoreada" lo reemplaza el del agente (ver el init de abajo).
    private val localHistory: HistoryRepository = HistoryModule.repository(application)
    private val history = MutableStateFlow<HistoryRepository>(localHistory)
    private val networkKey = MutableStateFlow<NetworkKey?>(null)

    /** Equipos conocidos de la red actual, conectados y sin conexión. */
    @OptIn(ExperimentalCoroutinesApi::class)
    val knownDevices: StateFlow<List<KnownDevice>> = combine(history, networkKey) { repo, key -> repo to key }
        .flatMapLatest { (repo, key) -> if (key == null) flowOf(emptyList()) else repo.observeDevices(key) }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())

    @OptIn(ExperimentalCoroutinesApi::class)
    fun device(id: Long): Flow<KnownDevice?> = history.flatMapLatest { it.observeDevice(id) }

    @OptIn(ExperimentalCoroutinesApi::class)
    fun events(id: Long): Flow<List<DeviceEvent>> = history.flatMapLatest { it.observeEvents(id) }

    fun customize(id: Long, custom: DeviceCustomization) {
        viewModelScope.launch { history.value.updateCustomization(id, custom) }
    }

    fun forget(id: Long) {
        viewModelScope.launch { history.value.forget(id) }
    }

    private fun keyFor(network: CurrentNetwork?, result: ScanResult?): NetworkKey? {
        val subnet = result?.subnet?.takeIf { it.isNotBlank() } ?: NetworkKey.subnetOf(network?.cidr) ?: return null
        val gateway = network?.gateway ?: result?.gateway?.takeIf { it.isNotBlank() }
        return NetworkKey.of(network?.ssid, gateway, subnet)
    }

    private val _uiState = MutableStateFlow(ScanUiState())
    val uiState: StateFlow<ScanUiState> = _uiState.asStateFlow()

    init {
        refreshNetworkInfo()
        // Cuando el agente toma o suelta el control de la red actual (ver
        // AgentModeController.evaluate, llamado desde refreshNetworkInfo),
        // cambia el repositorio activo. En demo nunca se toca: ese modo no
        // habla con nada real.
        viewModelScope.launch {
            // Repositorio y clave juntos: los dos cambian casi a la vez y, leídos
            // por separado, la UI podía quedarse con el repositorio del agente y
            // una clave vacía o la local (lista vacía en "Red monitoreada").
            combine(agentController.activeRepository, agentController.activeAgentNetworkKey) { repo, key -> repo to key }
                .collect { (agentRepo, agentKey) ->
                    if (demoMode) return@collect
                    if (agentRepo != null && agentKey != null) {
                        networkKey.value = agentKey
                        history.value = agentRepo
                    } else if (agentRepo == null) {
                        history.value = localHistory
                        networkKey.value = keyFor(_uiState.value.network, _uiState.value.result)
                    }
                }
        }
        // Widget de pantalla de inicio (fase 8, ver `data/widget/WidgetSummaryUpdater.kt`
        // y el encargo: "se actualiza... cuando la app recibe datos nuevos").
        // Nunca con datos de demo: el widget vive en la pantalla de inicio de
        // verdad, no en el modo de muestra.
        viewModelScope.launch {
            agentPanelState.collect { state ->
                if (!demoMode) WidgetSummaryUpdater.pushLinked(application, state is AgentPanelState.Linked)
            }
        }
        viewModelScope.launch {
            knownDevices.collect { devices ->
                if (!demoMode) WidgetSummaryUpdater.pushOnlineDevices(application, devices.count { it.online })
            }
        }
    }

    fun refreshNetworkInfo() {
        if (demoMode) return
        val network = networkInfoProvider.current()
        _uiState.update { it.copy(network = network) }
        // Antes del primer escaneo ya se ve lo que se sabe de esta red (si el
        // agente no toma el control; ver el collect en el init de arriba).
        if (agentController.activeRepository.value == null) {
            networkKey.value = keyFor(network, null)
        }
        // Re-evalúa si esta red la vigila el agente. Async: mientras contesta,
        // la UI sigue con lo que ya tenía (local, o el agente si ya estaba
        // activo de antes) — ver el encargo: nunca rompe nada.
        viewModelScope.launch { agentController.evaluate(network) }
    }

    /** Primer escaneo automático al abrir la app (como Fing): solo si nunca se escaneó. */
    fun startScanIfNeverScanned() {
        if (_uiState.value.result == null && !autoScanDone) {
            autoScanDone = true
            startScan()
        }
    }

    private var autoScanDone = false
    private var demoMode = false

    /**
     * Modo demo (solo debug, ver [DemoMode]): carga los datos de muestra ya
     * mismo, sin escanear. Si luego se pulsa "Escanear", [startScan] simula
     * el progreso en vez de llamar al motor real.
     */
    fun enableDemoMode() {
        demoMode = true
        autoScanDone = true
        _uiState.update { it.copy(network = DemoMode.network, status = ScanStatus.Idle, result = DemoMode.scanResult) }
        val repo = InMemoryHistoryRepository()
        history.value = repo
        // El propio sembrado hace su recordScan "de ahora" con DemoMode.scanResult,
        // además de un par de escaneos pasados (ver DemoFixtures.seedHistory):
        // así el historial trae de entrada equipos sin conexión, uno "Nuevo"
        // y una personalización de ejemplo, no solo el snapshot actual.
        val key = keyFor(DemoMode.network, DemoMode.scanResult) ?: return
        networkKey.value = key
        viewModelScope.launch { DemoMode.seedHistory(repo, key) }
    }

    /**
     * Pasa un resultado al historial local de su red. Si el agente ya tomó el
     * control de esta red (pudo pasar mientras corría este escaneo), el
     * escaneo igual alimenta el historial local, pero NO cambia la red que
     * muestra la UI: si no, la app pedía al agente la clave local
     * ("Starlink|…"), que él no conoce, y la lista quedaba vacía.
     */
    private fun recordInHistory(network: CurrentNetwork?, result: ScanResult) {
        val key = keyFor(network, result) ?: return
        val agentActive = !demoMode && agentController.activeRepository.value != null
        val repo = if (agentActive) localHistory else history.value
        if (!agentActive) networkKey.value = key
        viewModelScope.launch { repo.recordScan(key, result, Instant.now()) }
    }

    fun startScan() {
        if (_uiState.value.status is ScanStatus.Scanning) return
        if (demoMode) {
            startDemoScan()
            return
        }
        val agentRepo = agentController.activeRepository.value
        if (agentRepo != null) {
            startAgentScan(agentRepo)
            return
        }
        refreshNetworkInfo()
        val network = _uiState.value.network
        val cidr = network?.cidr
        if (network == null || cidr == null) {
            _uiState.update { it.copy(errorMessage = getApplication<Application>().getString(R.string.scan_error_no_network)) }
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(status = ScanStatus.Scanning(stage = "", done = 0, total = 0), errorMessage = null)
            }
            multicastHolder.acquireMulticastLock()
            val bound = network.network?.let { networkInfoProvider.bindProcessTo(it) } == true
            try {
                repository.scanNetwork(
                    cidr = cidr,
                    gateway = network.gateway.orEmpty(),
                    withPorts = true,
                    onProgress = { stage, done, total ->
                        _uiState.update { it.copy(status = ScanStatus.Scanning(stage, done, total)) }
                    },
                ).onSuccess { result ->
                    _uiState.update { it.copy(status = ScanStatus.Idle, result = result) }
                    recordInHistory(network, result)
                }.onFailure { error ->
                    _uiState.update {
                        it.copy(
                            status = ScanStatus.Idle,
                            errorMessage = if (error.isUserCancelled()) null else error.message,
                        )
                    }
                }
            } finally {
                if (bound) networkInfoProvider.bindProcessTo(null)
                multicastHolder.releaseMulticastLock()
            }
        }
    }

    /**
     * "Escanear" en Red monitoreada (ver el encargo, punto 3): `POST
     * /api/v1/scan` y, como el agente no manda progreso granular (solo
     * "empezó"/"terminó", ver el contrato), un indicador indeterminado hasta
     * que el stream avise que ESTE escaneo terminó o pasen 20 s.
     */
    private fun startAgentScan(agentRepo: AgentHistoryRepository) {
        agentScanJob = viewModelScope.launch {
            _uiState.update { it.copy(status = ScanStatus.Scanning(stage = "agent", done = 0, total = 0), errorMessage = null) }
            val before = agentRepo.lastScanEventAt.value
            agentRepo.requestScan().onFailure { error ->
                val yaHabiaUnoEnCurso = (error as? AgentApiException)?.status == 409
                if (!yaHabiaUnoEnCurso) {
                    _uiState.update { it.copy(status = ScanStatus.Idle, errorMessage = error.message) }
                    return@launch
                }
            }
            withTimeoutOrNull(AgentScanTimeoutMs) {
                agentRepo.lastScanEventAt.first { it != null && it != before }
            }
            _uiState.update { if (it.status is ScanStatus.Scanning) it.copy(status = ScanStatus.Idle) else it }
        }
    }

    private var agentScanJob: Job? = null

    /** Pide cortar el escaneo en curso. Es seguro llamarlo aunque no haya ninguno. */
    fun cancelScan() {
        if (demoMode) return
        if (agentController.activeRepository.value != null) {
            agentScanJob?.cancel()
            _uiState.update { it.copy(status = ScanStatus.Idle) }
            return
        }
        repository.cancelNetworkScan()
    }

    /** "Escanear" en demo: simula el progreso y vuelve a mostrar los mismos datos de muestra. */
    private fun startDemoScan() {
        viewModelScope.launch {
            _uiState.update { it.copy(status = ScanStatus.Scanning(stage = "", done = 0, total = 0), errorMessage = null) }
            DemoMode.simulateScan { stage, done, total ->
                _uiState.update { it.copy(status = ScanStatus.Scanning(stage, done, total)) }
            }
            _uiState.update { it.copy(status = ScanStatus.Idle, result = DemoMode.scanResult) }
            recordInHistory(DemoMode.network, DemoMode.scanResult)
        }
    }

    /**
     * Escaneo completo de puertos de [device] ([EXTENDED_PORTS]) en segundo
     * plano: sigue aunque se salga del detalle o de la app (ver
     * [PortScanWorker]). Si ya hay uno en curso para ese equipo, no se repite.
     */
    fun scanAllPorts(device: KnownDevice) {
        workManager.enqueueUniqueWork(
            PortScanWork.uniqueName(device.network, device.id),
            ExistingWorkPolicy.KEEP,
            PortScanWork.request(device.last.ip, EXTENDED_PORTS),
        )
    }

    /** Último escaneo completo de puertos del equipo [id] de [network]: en curso, terminado o con error. */
    fun portScan(network: NetworkKey, id: Long): Flow<PortScanState> =
        workManager.getWorkInfosForUniqueWorkFlow(PortScanWork.uniqueName(network, id)).map(::portScanStateOf)

    // En modo agente, el agente ve a este teléfono solo por ARP (MAC aleatoria,
    // sin anuncios) y sale "Sin identificar". La app sí sabe qué es: se lo dice
    // una vez, si nadie lo personalizó. Al final de la clase a propósito: el
    // collect corre en el acto y necesita todo lo de arriba ya inicializado.
    private val identifiedPhoneIds = mutableSetOf<Long>()

    init {
        viewModelScope.launch {
            combine(knownDevices, agentPanelState) { devices, panelState -> devices to panelState }
                .collect { (devices, panelState) ->
                    val hasRepo = agentController.activeRepository.value != null
                    if (!shouldAutoIdentifyPhone(demoMode, hasRepo, panelState)) return@collect
                    val ip = _uiState.value.network?.cidr?.substringBefore('/') ?: return@collect
                    val me = devices.firstOrNull { it.lastIp == ip && it.online } ?: return@collect
                    if (me.custom.name != null || me.custom.typeId != null) return@collect
                    if (!identifiedPhoneIds.add(me.id)) return@collect
                    customize(me.id, me.custom.copy(name = phoneName(), typeId = "phone"))
                }
        }
    }

    /** Nombre configurado del teléfono en Ajustes o, si no hay, fabricante y modelo. */
    private fun phoneName(): String {
        val app = getApplication<Application>()
        val configured = runCatching {
            android.provider.Settings.Global.getString(app.contentResolver, android.provider.Settings.Global.DEVICE_NAME)
        }.getOrNull()
        return configured?.takeIf { it.isNotBlank() }
            ?: "${android.os.Build.MANUFACTURER.replaceFirstChar { it.uppercase() }} ${android.os.Build.MODEL}"
    }

    companion object {
        /**
         * Escaneo "completo" desde el detalle: los 1024 puertos conocidos más
         * los servicios altos típicos de una casa (cámaras, NAS, Plex, bases
         * de datos, paneles web, UPnP, sincronización del iPhone).
         */
        const val EXTENDED_PORTS = "1-1024,1080,1194,1433,1883,1900,2049,3000,3306,3389,3689," +
            "5000,5001,5353,5432,5555,5900,6379,7000,8000-8100,8443,8554,8883,8888,8899,9000," +
            "9100,9443,10000,27017,32400,49152-49155,51820,62078"

        /** Tope para que "Escanear" en Red monitoreada no se quede pegado en "Escaneando…" si el stream no avisa (ver [startAgentScan]). */
        private const val AgentScanTimeoutMs = 20_000L
    }

    override fun onCleared() {
        repository.cancelNetworkScan()
        multicastHolder.releaseMulticastLock()
    }
}

/**
 * Only auto-identify the phone when we are actively monitoring the current
 * network (or standalone local mode); in remote-viewing mode the phone's own
 * LAN IP doesn't match the remote network's devices.
 */
internal fun shouldAutoIdentifyPhone(
    demoMode: Boolean,
    hasActiveRepository: Boolean,
    panelState: AgentPanelState,
): Boolean = !demoMode && (!hasActiveRepository || (panelState as? AgentPanelState.Linked)?.monitoringCurrentNetwork == true)
