package com.baxner.vigia.data.agent

import android.content.Context
import com.baxner.vigia.data.widget.WidgetSummaryUpdater
import com.baxner.vigia.notifications.ensureAlertChannels
import com.baxner.vigia.notifications.notifyAlert
import com.baxner.vigia.work.AlertsSyncWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onSubscription
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/**
 * Dueño de las alertas del agente en el proceso de la app (ver el contrato §
 * "Alertas v1.1" y el encargo, puntos 2-5): las trae por red (LAN primero y,
 * si no contesta, las [AlertsStore.agentAddresses] de Tailscale — ver
 * [AgentReachability]), las cachea para la pantalla Alertas y decide cuáles
 * notificar ([AlertNotificationPlanner], nunca una ya notificada).
 *
 * A propósito independiente de [AgentModeController.activeRepository]
 * ("Red monitoreada") para las llamadas REST: el agente puede estar
 * vinculado y no vigilar la red actual (el teléfono está fuera de casa) y
 * las alertas igual deben poder leerse/marcarse por Tailscale. Lo que SÍ
 * depende de esa sesión es el empuje en vivo por SSE (punto 3): mientras
 * [AgentModeController.activeRepository] tenga una sesión, esta clase
 * escucha su [AgentHistoryRepository.newAlerts].
 *
 * Vive mientras el proceso (ver [AlertsModule]), igual que
 * [AgentModeController]: así una notificación en vivo y el refresco manual
 * de la pantalla comparten el mismo caché y el mismo cursor.
 */
class AlertsRepository(
    context: Context,
    agentController: AgentModeController = AgentModule.controller(context),
    private val linkStore: AgentLinkStore = EncryptedAgentLinkStore(context),
    private val store: AlertsStore = SharedPrefsAlertsStore(context),
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    private val appContext = context.applicationContext

    private val httpClient = OkHttpClient.Builder()
        .connectTimeout(ConnectTimeoutSeconds, TimeUnit.SECONDS)
        .readTimeout(ReadTimeoutSeconds, TimeUnit.SECONDS)
        .build()

    private val _alerts = MutableStateFlow(store.cachedAlerts().sortedByDescending { it.id })

    /** De la más nueva a la más vieja (ver la pantalla Alertas: la lista se lee así). */
    val alerts: StateFlow<List<AlertDto>> = _alerts.asStateFlow()

    val unreadCount: StateFlow<Int> = alerts
        .map { list -> list.count { !it.read } }
        .stateIn(scope, SharingStarted.Eagerly, _alerts.value.count { !it.read })

    private val _settings = MutableStateFlow(store.cachedSettings())
    val settings: StateFlow<AlertSettingsDto> = _settings.asStateFlow()

    init {
        ensureAlertChannels(appContext)
        // "al actualizar" (ver el encargo, punto 4): por si el proceso
        // arrancó con un vínculo ya guardado de una sesión anterior y
        // AgentModeController.pair() no corrió en este arranque.
        if (linkStore.get() != null) AlertsSyncWorker.schedule(appContext)

        // Punto 3 del encargo: mientras dure la sesión SSE de "Red
        // monitoreada", cada `alert` nueva se procesa en el acto.
        // collectLatest: si la sesión cambia (reconexión, otra red), se
        // deja de escuchar la vieja y se vuelve a poner al día con la nueva.
        scope.launch {
            agentController.activeRepository.collectLatest { repo ->
                if (repo == null) return@collectLatest
                // Suscribirse primero y ponerse al día después: una alerta que
                // llegue entre las dos cosas no se pierde (y el cursor y el
                // candado de handleIncoming evitan avisarla dos veces).
                repo.newAlerts
                    .onSubscription { refreshFromNetwork() }
                    .collect { alert -> handleIncoming(listOf(alert)) }
            }
        }
    }

    /**
     * `GET /alerts?after=<cursor>` probando cada dirección candidata (ver
     * [AgentReachability]) hasta que una conteste. Lo usan la app abierta y
     * [AlertsSyncWorker] (el mismo camino, para que no avisen dos veces).
     */
    suspend fun refreshFromNetwork(): Result<Unit> {
        val fetched = withReachableAgent { it.alerts(after = store.lastNotifiedAlertId()) }
            .getOrElse { return Result.failure(it) }
        handleIncoming(fetched)
        return Result.success(Unit)
    }

    /** Marca leídas todas las alertas cacheadas (ver el encargo, punto 5: "al abrirla marca leídas"). No hace nada si no hay ninguna. */
    suspend fun markAllRead(): Result<Unit> {
        val maxId = _alerts.value.maxOfOrNull { it.id } ?: return Result.success(Unit)
        return withReachableAgent { it.markRead(upTo = maxId) }.onSuccess {
            val updated = _alerts.value.map { a -> if (a.id <= maxId) a.copy(read = true) else a }
            _alerts.value = updated
            store.setCachedAlerts(updated)
            pushWidgetSummary()
        }.map { }
    }

    suspend fun loadSettingsFromNetwork(): Result<AlertSettingsDto> =
        withReachableAgent { it.alertSettings() }.onSuccess { fetched ->
            _settings.value = fetched
            store.setCachedSettings(fetched)
        }

    suspend fun updateSettings(newSettings: AlertSettingsDto): Result<AlertSettingsDto> =
        withReachableAgent { it.putAlertSettings(newSettings) }.onSuccess { saved ->
            _settings.value = saved
            store.setCachedSettings(saved)
        }

    /**
     * Une [newOnes] al caché (sin duplicar, más nueva primero), decide qué
     * notificar y avanza el cursor — ver [AlertNotificationPlanner]. Con
     * candado: el stream en vivo y el sondeo periódico pueden traer la misma
     * alerta a la vez, y leer el cursor, avisar y guardarlo tiene que ser
     * una sola cosa.
     */
    private suspend fun handleIncoming(newOnes: List<AlertDto>) = deliverLock.withLock {
        if (newOnes.isEmpty()) {
            pushWidgetSummary()
            return@withLock
        }
        val merged = (newOnes + _alerts.value).distinctBy { it.id }.sortedByDescending { it.id }
        _alerts.value = merged
        store.setCachedAlerts(merged)

        val previousCursor = store.lastNotifiedAlertId()
        AlertNotificationPlanner.plan(newOnes, previousCursor, _settings.value)
            .forEach { alert -> notifyAlert(appContext, alert) }
        store.setLastNotifiedAlertId(AlertNotificationPlanner.nextCursor(newOnes, previousCursor))
        pushWidgetSummary()
    }

    /**
     * Widget de pantalla de inicio (fase 8, ver `data/widget/WidgetSummaryUpdater.kt`):
     * se llama tanto desde el sondeo de [AlertsSyncWorker] como desde la app
     * abierta ([handleIncoming]/[markAllRead]) — exactamente los dos
     * momentos que pide el encargo. [linkStore] (no [agentController], que
     * acá es solo un parámetro de constructor, no una propiedad) ya dice si
     * hay vínculo, el mismo criterio que [withReachableAgent].
     */
    private suspend fun pushWidgetSummary() {
        WidgetSummaryUpdater.pushUnreadAlerts(
            context = appContext,
            linked = linkStore.get() != null,
            unreadAlerts = _alerts.value.count { !it.read },
        )
    }

    /** Ver [withPinnedReachableAgent] (LAN, luego Tailscale, con la huella TLS fijada); antes adopta el agente del vínculo. */
    private suspend fun <T> withReachableAgent(block: suspend (AgentApiClient) -> Result<T>): Result<T> {
        val link = linkStore.get() ?: return Result.failure(AgentApiException(401, "sin vínculo: el vínculo no está activo"))
        adoptAgent(link.agentId)
        return withPinnedReachableAgent(linkStore, store.agentAddresses(), httpClient, block)
    }

    /** Si lo guardado es de otro agente (se vinculó uno distinto), se empieza de cero: ver [AlertsStore.ownerAgentId]. */
    private suspend fun adoptAgent(agentId: String) = deliverLock.withLock {
        if (store.ownerAgentId() == agentId) return@withLock
        store.clear()
        store.setOwnerAgentId(agentId)
        _alerts.value = emptyList()
        _settings.value = AlertSettingsDto()
    }

    private val deliverLock = Mutex()

    private companion object {
        const val ConnectTimeoutSeconds = 4L
        const val ReadTimeoutSeconds = 8L
    }
}

/** Singleton de proceso, igual que [AgentModule]/[com.baxner.vigia.data.history.HistoryModule]. */
object AlertsModule {
    @Volatile private var instance: AlertsRepository? = null

    fun repository(context: Context): AlertsRepository =
        instance ?: synchronized(this) {
            instance ?: AlertsRepository(context.applicationContext).also { instance = it }
        }
}
