package com.baxner.vigia.data.agent

import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.baxner.vigia.MainActivity
import com.baxner.vigia.R
import com.baxner.vigia.notifications.AlertChannels
import com.baxner.vigia.notifications.ensureLiveWatchChannel
import com.baxner.vigia.data.network.NetworkInfoProvider
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): servicio en
 * primer plano tipo `connectedDevice` (ver el manifiesto:
 * `FOREGROUND_SERVICE_CONNECTED_DEVICE`, que en Android 14+ exige además uno
 * de un puñado de permisos de conectividad — acá `CHANGE_WIFI_STATE`, que
 * Vigía ya pide para escanear) que mantiene viva la sesión de "Red
 * monitoreada" de [AgentModeController] (SSE incluido) mientras el teléfono
 * esté en la red que vigila el agente, para que las alertas lleguen en
 * segundos con la app cerrada — [AlertsRepository] ya escucha esa sesión
 * sin importar quién la mantenga viva, así que no hay un camino de
 * notificación distinto para esto: es el mismo de siempre (candado +
 * cursor), solo que la conexión sigue abierta sin pantalla.
 *
 * Arranca (ver [start]): desde [AgentModeController] (toggle en ajustes, o
 * cualquier `evaluate()` mientras la app está abierta: eso SIEMPRE puede
 * arrancar un servicio en primer plano) y desde [BootStartReceiver] (`BOOT_COMPLETED`
 * es una de las excepciones documentadas a la restricción de Android 12+ de
 * arrancar servicios en primer plano desde segundo plano).
 *
 * **Revisión de diseño** (la queja original era "llegan cuando abro la
 * app"): con el interruptor prendido y un agente vinculado, el servicio
 * sigue vivo aunque el teléfono salga de la red de casa (ver
 * [LiveWatchPolicy.shouldRun] — ya no exige "Red monitoreada"). Fuera de
 * casa queda "en espera": sin sesión SSE (eso lo cierra solo
 * [AgentModeController.evaluate] cuando ninguna red del agente coincide,
 * ver [AgentModeController.activate]/`deactivate`) y con el
 * [ConnectivityManager.NetworkCallback] de este servicio como único gasto
 * — nada de red hasta que algo cambie. Al volver a la red del agente, ese
 * mismo callback llama [reevaluate] → `evaluate()` reconecta el SSE solo,
 * sin que el usuario tenga que abrir la app. La notificación (siempre fija,
 * silenciosa, mismo canal) cambia de texto con el estado — ver
 * [LiveWatchPolicy.notificationKindFor] y [liveWatchTitle]: vigilando (en
 * casa), en espera (fuera) o sin conexión (vinculado, debería ser su red,
 * pero el agente no contestó).
 *
 * Solo se detiene de verdad (ver [LiveWatchPolicy.shouldRun]) al apagar el
 * interruptor o al desvincular ([AgentModeController.forget]) — ambos
 * gestos explícitos del usuario, nunca un simple "saliste de casa".
 * [com.baxner.vigia.work.AlertsSyncWorker] sigue cubriendo las alertas cada
 * ~15 min de todas formas, por si el proceso llegara a morir.
 */
class LiveWatchService : Service() {
    private var scope: CoroutineScope? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private var reevaluateJob: Job? = null

    private val agentController by lazy { AgentModule.controller(applicationContext) }
    private val alertsStore: AlertsStore by lazy { SharedPrefsAlertsStore(applicationContext) }

    override fun onBind(intent: Intent?) = null

    override fun onCreate() {
        super.onCreate()
        ensureLiveWatchChannel(applicationContext)
        // Placeholder hasta que la primera reevaluate() (abajo) resuelva el
        // estado real: "en espera" es el más neutral de los tres mientras
        // tanto, y startForeground debe llamarse ya mismo en onCreate.
        startForeground(NotificationId, buildNotification(LiveWatchNotificationKind.AWAY_WAITING, deviceCount = null))
        // Por si el proceso arrancó recién para este servicio (p. ej. desde
        // BootStartReceiver): que exista para que recoja las alertas que
        // lleguen por el stream, no solo las que pida esta clase por REST.
        AlertsModule.repository(applicationContext)

        val session = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        scope = session
        val connectivityManager = getSystemService(ConnectivityManager::class.java)
        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .build()
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = reevaluate(delayMs = NetworkDebounceMs)
            override fun onLost(network: Network) = reevaluate(delayMs = NetworkDebounceMs)
            override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) =
                reevaluate(delayMs = NetworkDebounceMs)
        }
        networkCallback = callback
        runCatching { connectivityManager?.registerNetworkCallback(request, callback) }
        reevaluate()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        reevaluate()
        return START_STICKY
    }

    override fun onDestroy() {
        networkCallback?.let { callback ->
            runCatching { getSystemService(ConnectivityManager::class.java)?.unregisterNetworkCallback(callback) }
        }
        networkCallback = null
        reevaluateJob?.cancel()
        scope?.cancel()
        scope = null
        super.onDestroy()
    }

    /**
     * Vuelve a evaluar la red actual (ver [AgentModeController.evaluate]) y
     * decide si sigue haciendo falta este servicio ([LiveWatchPolicy]): si
     * no, se detiene solo; si sí, refresca la notificación con cuántos
     * equipos ve el agente ahora. Se relanza (no se encola) en cada llamada:
     * un [ConnectivityManager.NetworkCallback] puede disparar varias veces
     * seguidas por el mismo cambio de red, y solo importa el resultado de la
     * última.
     */
    private fun reevaluate(delayMs: Long = 0L) {
        reevaluateJob?.cancel()
        reevaluateJob = scope?.launch {
            if (delayMs > 0L) delay(delayMs)
            val enabled = alertsStore.liveWatchEnabled()
            if (!enabled || !agentController.isLinked()) {
                stopSelf()
                return@launch
            }
            val network = NetworkInfoProvider(applicationContext).current()
            // syncService = false: este servicio ya está evaluando su propio ciclo
            // de vida y llamará stopSelf()/startForeground abajo, evitando volver a
            // disparar LiveWatchService.start() -> onStartCommand() -> reevaluate().
            agentController.evaluate(network, syncService = false)
            val panelState = agentController.panelState.value
            if (!LiveWatchPolicy.shouldRun(enabled, panelState)) {
                stopSelf()
                return@launch
            }
            val kind = LiveWatchPolicy.notificationKindFor(panelState)
            val count = if (kind == LiveWatchNotificationKind.WATCHING) deviceCountOrNull() else null
            startForeground(NotificationId, buildNotification(kind, count))
        }
    }

    private suspend fun deviceCountOrNull(): Int? {
        val repo = agentController.activeRepository.value ?: return null
        val key = agentController.activeAgentNetworkKey.value ?: return null
        return runCatching { repo.observeDevices(key).first().size }.getOrNull()
    }

    private fun buildNotification(kind: LiveWatchNotificationKind, deviceCount: Int?) =
        NotificationCompat.Builder(this, AlertChannels.LIVE_WATCH)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(liveWatchTitle(kind, deviceCount))
            .setOngoing(true)
            .setSilent(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setContentIntent(openAppIntent())
            .build()

    /** Ver [LiveWatchPolicy.notificationKindFor]: el texto de cada uno de los 3 estados, siempre en el mismo canal silencioso. */
    private fun liveWatchTitle(kind: LiveWatchNotificationKind, deviceCount: Int?): String = when (kind) {
        LiveWatchNotificationKind.WATCHING -> when {
            deviceCount == null -> getString(R.string.live_watch_notification_title_unknown)
            deviceCount == 1 -> getString(R.string.live_watch_notification_title_one)
            else -> getString(R.string.live_watch_notification_title_other, deviceCount)
        }
        LiveWatchNotificationKind.AWAY_WAITING -> getString(R.string.live_watch_notification_title_away_waiting)
        LiveWatchNotificationKind.AGENT_UNREACHABLE -> getString(R.string.live_watch_notification_title_agent_unreachable)
    }

    private fun openAppIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        return PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    companion object {
        private const val NotificationId = 1001
        private const val NetworkDebounceMs = 300L

        fun start(context: Context) {
            runCatching { ContextCompat.startForegroundService(context, Intent(context, LiveWatchService::class.java)) }
        }

        fun stop(context: Context) {
            runCatching { context.stopService(Intent(context, LiveWatchService::class.java)) }
        }
    }
}
