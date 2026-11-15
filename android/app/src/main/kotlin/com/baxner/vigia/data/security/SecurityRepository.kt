package com.baxner.vigia.data.security

import android.content.Context
import com.baxner.vigia.data.agent.AgentHistoryRepository
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.network.CurrentNetwork
import java.io.IOException
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** El motor no pudo calcular nada (ver [SecurityEngine.report]/[SecurityEngine.routerInfo] devolviendo `null`), o todavía no está disponible en esta build (ver [UnavailableSecurityEngine]). */
class SecurityUnavailableException(message: String) : IOException(message)

/**
 * Qué tan segura está la red (ver el encargo, puntos 2 y 3, y el contrato §
 * "Seguridad"): junta lo que sabe el agente (si está vigilando la red
 * actual — "Red monitoreada", ver [activeAgentRepository]) o el último
 * escaneo local, más el motor ([SecurityEngine], hoy
 * [UnavailableSecurityEngine] — ver esa clase) y la Wi-Fi actual del
 * teléfono. El armado en sí (plan A/plan B, E/S en el hilo correcto) vive en
 * funciones libres ([resolveMonitoredSecurityReport]/[resolveLocalSecurityReport],
 * ver `SecurityLogic.kt`) para poder probarlas sin Android; esta clase solo
 * decide el camino, limita cuántas veces se pide de verdad y cachea.
 *
 * Singleton de proceso (ver [SecurityModule]), igual criterio que
 * [com.baxner.vigia.data.agent.AlertsRepository]: el Panel (la tile
 * "Seguridad") y la pantalla Seguridad comparten el mismo caché — ver
 * [report], un `StateFlow` — sin pedir el informe dos veces a la vez.
 *
 * El constructor primario no pide [Context] a propósito (ver la revisión de
 * la fase 5, punto 9): así se puede probar con un [activeAgentRepository] y
 * un [store] en memoria, sin Robolectric. [SecurityModule]/producción usan
 * el constructor secundario.
 */
class SecurityRepository(
    private val activeAgentRepository: StateFlow<AgentHistoryRepository?>,
    private val engine: SecurityEngine,
    private val store: SecurityStore,
    private val isViewingRemote: () -> Boolean = { false },
    /** Inyectable para las pruebas del límite de 60 s (ver [MinRefreshInterval]). */
    private val clock: () -> Instant = Instant::now,
) {
    constructor(
        context: Context,
        engine: SecurityEngine = SecurityModule.engine(),
        store: SecurityStore = SharedPrefsSecurityStore(context),
    ) : this(
        activeAgentRepository = AgentModule.controller(context).activeRepository,
        engine = engine,
        store = store,
        isViewingRemote = {
            (AgentModule.controller(context).panelState.value as? com.baxner.vigia.data.agent.AgentPanelState.Linked)
                ?.viewingRemote == true
        },
    )

    private val _report = MutableStateFlow<SecurityReport?>(null)

    /** El último informe calculado con éxito (de cualquiera de los dos caminos). `null` = todavía no hay nada que mostrar (ver el encargo, punto 5: la tile del Panel "se queda como está" sin esto). */
    val report: StateFlow<SecurityReport?> = _report.asStateFlow()

    /** Un solo [refresh] a la vez (ver la revisión de la fase 5, punto 1): sin esto, una ráfaga de eventos del stream del agente podía disparar varias llamadas a `/api/v1/security` superpuestas. */
    private val refreshMutex = Mutex()

    private var lastSuccessAt: Instant? = null

    /** `true` si la red actual la vigila el agente ("Red monitoreada"): decide qué camino sigue [refresh] y qué estado vacío le conviene a la UI si falla. */
    val monitoredByAgent: Boolean get() = activeAgentRepository.value != null

    /** Último [RouterInfo] guardado (de cualquier informe anterior, de esta sesión o de una previa): lo usa "Tu router visto desde aquí" aunque todavía no se haya refrescado nada en este arranque. */
    fun lastRouterInfo(): RouterInfo? = store.lastRouterInfo()

    /** Ver [RouterVisibility.routerSeenFromHere]: el teléfono está en la Wi-Fi de la ONT, viendo al router "desde afuera". */
    fun routerSeenFromHere(phoneCidr: String?): Boolean =
        RouterVisibility.routerSeenFromHere(phoneCidr, lastRouterInfo()?.externalIp)

    /**
     * Refresca el informe (ver el encargo, punto 2: "al abrir la pantalla y
     * después de cada escaneo"). [devices] son los que ya tiene la UI (del
     * agente en Red monitoreada, o del historial local si no — el mismo
     * [com.baxner.vigia.ui.scan.ScanViewModel.knownDevices] que usan Panel y
     * Dispositivos), para no leer el historial dos veces por su cuenta.
     *
     * Sin [force] (el caso de "después de cada escaneo"), no vuelve a pedir
     * nada si el último informe exitoso tiene menos de [MinRefreshInterval]:
     * devuelve ese mismo informe cacheado (ver la revisión de la fase 5,
     * punto 1 — en Red monitoreada la lista de equipos cambia con cada
     * evento del stream, muchas veces por escaneo). [onOpen][com.baxner.vigia.ui.security.SecurityViewModel.onOpen]
     * sí pasa `force = true`: al abrir la pantalla el usuario espera un
     * dato fresco, no el de hace casi un minuto.
     */
    suspend fun refresh(
        devices: List<KnownDevice>,
        network: CurrentNetwork?,
        wifi: WifiInfo?,
        recentAlerts: List<AlertDto> = emptyList(),
        force: Boolean = false,
    ): Result<SecurityReport> = refreshMutex.withLock {
        val last = lastSuccessAt
        val cached = _report.value
        if (!force && last != null && cached != null && Duration.between(last, clock()) < MinRefreshInterval) {
            return@withLock Result.success(cached)
        }

        val agentRepo = activeAgentRepository.value
        val result = if (agentRepo != null) {
            val effectiveWifi = if (isViewingRemote()) null else wifi
            resolveMonitoredSecurityReport(agentRepo::security, engine, devices, network, effectiveWifi, recentAlerts)
        } else {
            // Ver el contrato § "Hallazgos reconocidos v1.3": sin agente, los
            // reconocimientos los guarda el teléfono (por red) y se los pasa
            // al motor; con agente, ya vienen aplicados en el informe.
            val acknowledged = localNetworkKeyFor(network)?.let(store::acknowledgedFindings).orEmpty()
            resolveLocalSecurityReport(engine, devices, network, wifi, recentAlerts, acknowledged)
        }
        result.onSuccess { report ->
            _report.value = report
            lastSuccessAt = clock()
            report.router?.let(store::setLastRouterInfo)
        }
        result
    }

    /**
     * "Ya lo sé, es a propósito" (ver el encargo, punto 2, y el contrato §
     * "Hallazgos reconocidos v1.3"). Con agente: `PUT` al servidor (ver
     * [AgentHistoryRepository.acknowledgeFinding]), compartido entre
     * teléfonos. Sin agente: se guarda en el teléfono, por red (ver
     * [localNetworkKeyFor]) — `null` sin red usable, no hay dónde guardarlo.
     * Quien llama debe refrescar el informe después (ver
     * [com.baxner.vigia.ui.security.SecurityViewModel.acknowledge]):
     * este método no lo hace solo, para no acoplar una red más (los
     * [devices] que haría falta) a esta llamada.
     */
    suspend fun acknowledge(findingId: String, network: CurrentNetwork?): Result<Unit> {
        val agentRepo = activeAgentRepository.value
        if (agentRepo != null) return agentRepo.acknowledgeFinding(findingId)
        val key = localNetworkKeyFor(network)
            ?: return Result.failure(SecurityUnavailableException("sin red: no se puede guardar el reconocimiento"))
        val updated = store.acknowledgedFindings(key).filterNot { it.findingId == findingId } +
            AcknowledgedFinding(findingId = findingId, at = clock().toString())
        store.setAcknowledgedFindings(key, updated)
        return Result.success(Unit)
    }

    /** "Deshacer" (ver [acknowledge]). */
    suspend fun unacknowledge(findingId: String, network: CurrentNetwork?): Result<Unit> {
        val agentRepo = activeAgentRepository.value
        if (agentRepo != null) return agentRepo.unacknowledgeFinding(findingId)
        val key = localNetworkKeyFor(network)
            ?: return Result.failure(SecurityUnavailableException("sin red: no se puede deshacer el reconocimiento"))
        store.setAcknowledgedFindings(key, store.acknowledgedFindings(key).filterNot { it.findingId == findingId })
        return Result.success(Unit)
    }

    companion object {
        /** Ver [refresh]: nada de red/motor de nuevo antes de esto, salvo `force = true`. */
        val MinRefreshInterval: Duration = Duration.ofSeconds(60)
    }
}

/** Singleton de proceso, igual que [AgentModule]/[com.baxner.vigia.data.agent.AlertsModule]. */
object SecurityModule {
    @Volatile private var instance: SecurityRepository? = null

    fun repository(context: Context): SecurityRepository =
        instance ?: synchronized(this) {
            instance ?: SecurityRepository(context.applicationContext).also { instance = it }
        }

    /**
     * El motor en uso. HOY siempre [UnavailableSecurityEngine]: el `.aar`
     * (`android/app/libs/vigia-core.aar`) todavía no tiene
     * `Mobile.securityReport`/`Mobile.routerInfo`/`Mobile.portCatalog` (ver
     * la nota grande en [SecurityEngine]). **Pendiente del orquestador**:
     * cuando el motor en Go ya las tenga, cambiar este método para que
     * devuelva una implementación que delegue en `Mobile` — todo lo demás
     * ([SecurityRepository], `ui/security/`) ya está escrito contra
     * [SecurityEngine] y no necesita tocarse.
     */
    fun engine(): SecurityEngine = GoSecurityEngine
}
