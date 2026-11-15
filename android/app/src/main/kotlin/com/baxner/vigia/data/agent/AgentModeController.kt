package com.baxner.vigia.data.agent

import android.content.Context
import android.net.Network
import android.os.Build
import com.baxner.vigia.R
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.work.AlertsSyncWorker
import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient

/** Estado de la tarjeta "Vigilancia 24/7" del Panel (ver el encargo, punto 5). */
sealed interface AgentPanelState {
    data object NotLinked : AgentPanelState

    data class Linked(
        val agentName: String,
        /** El agente contestó, por LAN o por Tailscale (ver [viewingRemote]/[remoteAvailable]). */
        val reachable: Boolean,
        /** La red actual del teléfono es una de las que vigila el agente: "Red monitoreada" de verdad, por la LAN. */
        val monitoringCurrentNetwork: Boolean,
        val lastScanAt: Instant?,
        /** Lo que reporta el agente en /info si algo le impide ver la red. */
        val warning: String? = null,
        /**
         * Viendo la casa remoto por Tailscale ahora mismo (ver el encargo,
         * fase 8: "todas las pantallas fuera de casa"): [AgentModeController.activeRepository]
         * es, en este caso, una sesión remota (no atada a la Wi-Fi) en vez de
         * la de "Red monitoreada". Solo puede ser `true` con
         * [monitoringCurrentNetwork] en `false` — nunca a la vez.
         */
        val viewingRemote: Boolean = false,
        /**
         * El agente contestó por Tailscale (hay una red de casa que mostrar
         * remoto), esté [viewingRemote] activado o no — para que la UI pueda
         * ofrecer el selector aunque el usuario haya elegido quedarse con la
         * red actual (ver `AgentSettingsScreen.kt`, "Ver tu casa remoto").
         */
        val remoteAvailable: Boolean = false,
    ) : AgentPanelState
}

/**
 * Dueño del vínculo con el agente y de si la app está en "Red monitoreada"
 * ahora mismo (ver el encargo, punto 3). Un singleton de proceso (ver
 * [AgentModule]), igual que [com.baxner.vigia.data.history.HistoryModule]:
 * así la conexión SSE sobrevive a que el Panel se recree, y
 * [com.baxner.vigia.ui.scan.ScanViewModel] solo necesita observar
 * [panelState]/[activeRepository].
 */
class AgentModeController(
    context: Context,
    private val linkStore: AgentLinkStore = EncryptedAgentLinkStore(context),
    /** Direcciones de /info guardadas para las alertas (ver [AlertsStore] y el encargo, punto 4). */
    private val alertsStore: AlertsStore = SharedPrefsAlertsStore(context),
) {
    private val appContext = context.applicationContext

    /** Vida de la conexión activa (SSE incluido). Se cancela al salir de "Red monitoreada"/"remoto" o al desvincular. */
    private var sessionScope: CoroutineScope? = null

    /** Para qué sirve [_activeRepository] ahora mismo (ver [activate]/[activateRemote]/[deactivate]): evita reconectar el SSE de más en cada [evaluate]. */
    private enum class ActiveSource { NONE, MONITORED, REMOTE }
    private var activeSource: ActiveSource = ActiveSource.NONE

    /** Vida de este controlador (singleton de proceso, ver [AgentModule]): solo para relanzar [evaluate] cuando [setRemoteViewPreferred] cambia la preferencia a medio camino. */
    private val controllerScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /** Serializa las evaluaciones concurrentes (UI, servicio en primer plano, cambio de preferencia o primer plano) para evitar transiciones superpuestas sobre [_panelState] y la sesión SSE. */
    private val evaluateMutex = Mutex()

    /** Cliente base compartido (un único ConnectionPool y Dispatcher en memoria) del que derivan los clientes atados a Wi-Fi o con huella fijada. */
    private val baseHttpClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(4, TimeUnit.SECONDS)
            .readTimeout(40, TimeUnit.SECONDS) // el stream manda "ping" cada 25 s (ver el contrato): con margen para no matarlo por timeout.
            .writeTimeout(10, TimeUnit.SECONDS)
            .build()
    }

    private val networkClientsLock = Any()
    private val networkClients = object : LinkedHashMap<Pair<Network?, String?>, OkHttpClient>(8, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Pair<Network?, String?>, OkHttpClient>?): Boolean =
            size > MaxCachedHttpClients
    }
    private val pinnedRemoteClients = object : LinkedHashMap<String, OkHttpClient>(4, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, OkHttpClient>?): Boolean =
            size > MaxCachedHttpClients
    }

    /** La red con la que se llamó [evaluate] por última vez, para poder repetirla desde [setRemoteViewPreferred] sin esperar al próximo cambio de red. */
    private var lastNetwork: CurrentNetwork? = null

    private val _panelState = MutableStateFlow<AgentPanelState>(AgentPanelState.NotLinked)
    val panelState: StateFlow<AgentPanelState> = _panelState.asStateFlow()

    private val _activeRepository = MutableStateFlow<AgentHistoryRepository?>(null)
    val activeRepository: StateFlow<AgentHistoryRepository?> = _activeRepository.asStateFlow()

    private val _activeAgentNetworkKey = MutableStateFlow<NetworkKey?>(null)
    val activeAgentNetworkKey: StateFlow<NetworkKey?> = _activeAgentNetworkKey.asStateFlow()

    // ---- "Ver tu casa remoto" (ver el encargo, fase 8, y AgentSettingsScreen.kt) ----

    private val _remoteViewPreferred = MutableStateFlow(alertsStore.remoteViewPreferred())
    val remoteViewPreferred: StateFlow<Boolean> = _remoteViewPreferred.asStateFlow()

    /** El interruptor de [com.baxner.vigia.ui.agent.AgentSettingsScreen]: re-evalúa ya mismo con la última red conocida, para que el cambio se note sin esperar el próximo [evaluate] (cambio de red, o el siguiente refresco manual). */
    fun setRemoteViewPreferred(preferred: Boolean) {
        if (_remoteViewPreferred.value == preferred) return
        _remoteViewPreferred.value = preferred
        alertsStore.setRemoteViewPreferred(preferred)
        val network = lastNetwork ?: return
        controllerScope.launch { evaluate(network) }
    }

    /**
     * Si la app está en primer plano (ver MainActivity.onStart/onStop). La
     * vista remota por Tailscale solo vive mientras se ve la app: su SSE
     * abierto con la app en segundo plano —o desde [LiveWatchService], que
     * mantiene vivo el proceso— gastaría batería y datos todo el día fuera
     * de casa sin que nadie lo esté mirando.
     */
    @Volatile private var appInForeground = false

    fun onAppForegroundChanged(foreground: Boolean) {
        if (appInForeground == foreground) return
        appInForeground = foreground
        if (foreground) {
            lastNetwork?.let { network -> controllerScope.launch { runCatching { evaluate(network) } } }
        } else if (activeSource == ActiveSource.REMOTE) {
            deactivate()
            _panelState.update { state -> if (state is AgentPanelState.Linked) state.copy(viewingRemote = false) else state }
        }
    }

    fun isLinked(): Boolean = linkStore.get() != null

    /**
     * Re-evalúa el modo para [network] (ver
     * [com.baxner.vigia.ui.scan.ScanViewModel.refreshNetworkInfo]): sin
     * vínculo, no hace nada. Con vínculo, le pregunta al agente sus redes;
     * si no contesta o ninguna coincide con [network], cae a modo local sin
     * romper nada (ver el encargo, punto 3) — el vínculo se conserva, solo
     * no se usa ahora.
     *
     * [syncService] es `false` cuando lo llama el propio [LiveWatchService]
     * para evitar el bucle `onStartCommand() -> evaluate() -> syncLiveWatch() -> start()`.
     */
    suspend fun evaluate(network: CurrentNetwork, syncService: Boolean = true) {
        evaluateMutex.withLock {
            evaluateLocked(network)
        }
        // "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): cada
        // evaluación externa es el momento de prender o apagar el servicio
        // en primer plano según [LiveWatchPolicy].
        if (syncService) syncLiveWatch()
    }

    private suspend fun evaluateLocked(network: CurrentNetwork) {
        lastNetwork = network
        val storedLink = linkStore.get()
        if (storedLink == null) {
            deactivate()
            _panelState.value = AgentPanelState.NotLinked
            return
        }

        // TLS v1.5 (ver `docs/agente-api.md`): un vínculo de antes de esta
        // fase todavía no tiene huella fijada — se toma por la vía vieja
        // (HTTP de transición) antes de poder hablarle por HTTPS. Si no
        // contesta, no es "sin conexión con un certificado distinto": es
        // sencillamente "no contesta todavía", igual que cualquier otro
        // agente inalcanzable (se reintenta en la próxima evaluación).
        val link = migrateLegacyLinkIfNeeded(linkStore, storedLink, httpClientFor(network))
        if (link == null) {
            deactivate()
            _panelState.value = AgentPanelState.Linked(
                agentName = storedLink.agentName,
                reachable = false,
                monitoringCurrentNetwork = false,
                lastScanAt = null,
            )
            return
        }

        val api = apiClientFor(link, network)
        val networksResult = api.networks()
        val subnet = NetworkKey.subnetOf(network.cidr)
        val match = networksResult.getOrNull()?.let { AgentModeSelector.matchingNetwork(it, network.gateway, subnet) }

        if (match != null) {
            activate(link, network, match)
            val info = api.info().getOrNull()
            val warning = info?.warning?.takeIf { it.isNotBlank() }
            // "al actualizar" (ver el encargo, punto 4): cada vez que esta
            // evaluación reconfirma el vínculo activo, refresca las
            // direcciones por las que WorkManager puede alcanzar al agente
            // fuera de esta LAN (Tailscale, ver AgentReachability).
            info?.addresses?.takeIf { it.isNotEmpty() }?.let(alertsStore::setAgentAddresses)
            _panelState.value = AgentPanelState.Linked(
                agentName = link.agentName,
                reachable = true,
                monitoringCurrentNetwork = true,
                lastScanAt = match.lastScanAt?.let(::parseInstantOrNull),
                warning = warning,
            )
        } else {
            // Ni la red actual es la de casa, ni (si falló del todo) el
            // agente contestó por esa LAN: antes de rendirse, probar por
            // Tailscale (ver el encargo, fase 8: "todas las pantallas fuera
            // de casa, no solo las alertas"). Si cambió la huella (p. ej.
            // tras `vigia-agent cert -rotate`), CUALQUIER candidato —LAN o
            // Tailscale, el agente sirve el mismo certificado en todas sus
            // interfaces— la rechaza por igual (ver AgentTls.pinnedSocketFactory):
            // no hace falta revisar networksResult por separado, el sondeo
            // remoto ya cubre ese caso. Nunca se cae a HTTP.
            // Con la app en segundo plano no se prueba nada remoto (ver
            // [appInForeground]): al volver a primer plano se re-evalúa.
            val remote = if (appInForeground) probeRemote(link) else RemoteProbe.Unreachable
            // Lógica pura (ver AgentViewSource.kt, probada con JUnit plano):
            // qué fuente toca según si la red actual es la de casa (acá
            // nunca, estamos en el "else"), si el agente contestó remoto y
            // si el usuario lo prefiere así (ver AgentSettingsScreen.kt).
            val source = AgentViewSourceSelector.decide(
                localMatch = false,
                remoteReachable = remote is RemoteProbe.Reachable,
                remoteViewPreferred = _remoteViewPreferred.value,
            )
            when (remote) {
                is RemoteProbe.Reachable -> {
                    val viewingRemote = source == AgentViewSource.Remote
                    if (viewingRemote) activateRemote(link, remote) else deactivate()
                    _panelState.value = AgentPanelState.Linked(
                        agentName = link.agentName,
                        reachable = true,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                        viewingRemote = viewingRemote,
                        remoteAvailable = true,
                    )
                }
                RemoteProbe.CertMismatch -> {
                    deactivate()
                    _panelState.value = AgentPanelState.Linked(
                        agentName = link.agentName,
                        reachable = false,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                        warning = appContext.getString(R.string.agent_cert_changed_warning),
                    )
                }
                RemoteProbe.Unreachable -> {
                    deactivate()
                    _panelState.value = AgentPanelState.Linked(
                        agentName = link.agentName,
                        reachable = false,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                    )
                }
            }
        }
    }

    /** Qué pasó al probar el agente por Tailscale (ver [probeRemote]): con qué dirección y red de casa, o por qué no se pudo. */
    private sealed interface RemoteProbe {
        /** [baseUrl] es la dirección (LAN del vínculo o Tailscale) que contestó — ver [resolvePinnedReachableAgent] — y [home] la red de casa a mostrar (ver [AgentModeSelector.primaryRemoteNetwork]). */
        data class Reachable(val baseUrl: String, val home: AgentNetworkDto) : RemoteProbe
        /** Alguna dirección contestó, pero con otro certificado: ver [isCertPinMismatch]. Nunca se cae a HTTP. */
        data object CertMismatch : RemoteProbe
        /** Ninguna dirección contestó (LAN del vínculo ni Tailscale): "revisa que Tailscale esté encendido". */
        data object Unreachable : RemoteProbe
    }

    /**
     * Prueba el agente por cada dirección candidata (ver [AgentReachability]:
     * LAN del vínculo y, si no, Tailscale) con un cliente que **no** se ata a
     * la Wi-Fi actual (ver [remoteHttpClient] — a diferencia de
     * [httpClientFor]: en casa hay que salir por la Wi-Fi para no entrar al
     * túnel; fuera de casa es justo lo contrario, hay que dejar que el
     * sistema enrute por el túnel de Tailscale, que es una VPN de Android).
     * Reusa [resolvePinnedReachableAgent] (ver AgentTls.kt) en vez de
     * reimplementar "LAN y luego Tailscale" acá.
     */
    private suspend fun probeRemote(link: AgentLink): RemoteProbe {
        val outcome = resolvePinnedReachableAgent(linkStore, alertsStore.agentAddresses(), remoteHttpClient()) { it.networks() }
        return outcome.fold(
            onSuccess = { (client, networks) ->
                val home = AgentModeSelector.primaryRemoteNetwork(networks)
                if (home != null) RemoteProbe.Reachable(client.baseUrl, home) else RemoteProbe.Unreachable
            },
            onFailure = { error -> if (error.isCertPinMismatch()) RemoteProbe.CertMismatch else RemoteProbe.Unreachable },
        )
    }

    private fun activate(link: AgentLink, network: CurrentNetwork, match: AgentNetworkDto) {
        val agentKey = NetworkKey(match.key)
        // Ya conectado a esta misma red del agente: no reconectar el SSE de más.
        if (activeSource == ActiveSource.MONITORED && _activeAgentNetworkKey.value == agentKey) return
        deactivate()
        activeSource = ActiveSource.MONITORED
        val session = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        sessionScope = session
        val api = apiClientFor(link, network)
        val stream = AgentEventStream(httpClientFor(network, link.certSha256), baseUrlFor(link)) { linkStore.get()?.token }
        // La clave ANTES que el repositorio: quien reacciona al repositorio lee
        // la clave en ese momento (si iba después, la UI se quedaba con null
        // y la lista vacía; observado en dispositivos Android reales).
        _activeAgentNetworkKey.value = agentKey
        _activeRepository.value = AgentHistoryRepository(api, stream, session)
    }

    /**
     * Igual que [activate], pero para ver la casa remoto por Tailscale (ver
     * el encargo, fase 8): [remote] ya tiene la dirección que contestó
     * ([probeRemote]) y la red de casa a mostrar. El cliente de esta sesión
     * ([remoteHttpClient]) nunca se ata a la Wi-Fi actual — "dejar que el
     * sistema enrute por el túnel de Tailscale" — a diferencia de [activate],
     * que sí ata a la Wi-Fi de casa para no salir por un túnel VPN activo.
     */
    private fun activateRemote(link: AgentLink, remote: RemoteProbe.Reachable) {
        val agentKey = NetworkKey(remote.home.key)
        // Ya viendo esta misma red de casa por Tailscale: no reconectar el SSE de más.
        if (activeSource == ActiveSource.REMOTE && _activeAgentNetworkKey.value == agentKey) return
        deactivate()
        activeSource = ActiveSource.REMOTE
        val session = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        sessionScope = session
        val client = pinnedRemoteClient(link.certSha256!!) // ya migrado: ver evaluateLocked().
        val api = AgentApiClient(client, remote.baseUrl) { linkStore.get()?.token }
        val stream = AgentEventStream(client, remote.baseUrl) { linkStore.get()?.token }
        _activeAgentNetworkKey.value = agentKey
        _activeRepository.value = AgentHistoryRepository(api, stream, session)
    }

    private fun deactivate() {
        sessionScope?.cancel()
        sessionScope = null
        _activeRepository.value = null
        _activeAgentNetworkKey.value = null
        activeSource = ActiveSource.NONE
    }

    // ---- Vinculación (ver el encargo, puntos 1 y 4) ----

    /**
     * Empareja con el agente en [host]:[port] usando el código de 6 dígitos
     * (ver el contrato: `POST /api/v1/pair`). Si funciona, guarda el vínculo
     * y sube una sola vez las personalizaciones de [localNetworkKey] en
     * [localHistory] (si hay alguna) — ver [AgentCustomizationMigrator].
     * No borra nada local (ver el encargo, punto 4).
     */
    suspend fun pair(
        host: String,
        port: Int,
        code: String,
        network: CurrentNetwork,
        localHistory: HistoryRepository,
        localNetworkKey: NetworkKey?,
        expectedCertSha256: String? = null,
    ): Result<AgentLink> {
        // Si mDNS trajo la huella completa SHA-256 en el TXT `cert`, fijamos
        // TLS antes incluso de mandar el código; si no, TOFU y verificación
        // contra la huella devuelta al responder el emparejamiento.
        val normalizedExpected = AgentTls.normalizeFingerprint(expectedCertSha256)
        val fullPin = normalizedExpected?.takeIf { it.length == 64 }
        val pairingClient = AgentApiClient(httpClientFor(network, fullPin), AgentTls.httpsBaseUrl(host, port)) { null }
        val response = pairingClient.pair(code, clientName()).getOrElse { return Result.failure(it) }
        val responseCert = AgentTls.normalizeFingerprint(response.certSha256)
        if (normalizedExpected != null && responseCert != null && !responseCert.startsWith(normalizedExpected)) {
            return Result.failure(CertPinMismatchException("la huella del agente no coincide con el anuncio mDNS"))
        }
        val link = AgentLink(
            agentId = response.agentId,
            agentName = response.agentName,
            host = host,
            port = port,
            token = response.token,
            certSha256 = (responseCert ?: fullPin)?.takeIf { it.isNotBlank() },
        )
        linkStore.save(link)

        if (localNetworkKey != null) {
            val authedClient = AgentApiClient(httpClientFor(network, link.certSha256), AgentTls.httpsBaseUrl(host, port)) { link.token }
            runCatching { uploadLocalCustomizations(authedClient, localHistory, localNetworkKey, network) }
        }
        // Que el Panel pase a "Vigilando" ya, no hasta el próximo arranque: sin
        // esto la tarjeta seguía en "Vincular" y el usuario reintentaba con un
        // código ya usado (visto en la prueba real).
        runCatching { evaluate(network) }
        // Alertas con la app cerrada (ver el encargo, punto 4): "al vincular"
        // es el momento más claro para empezar a sondear; AlertsSyncWorker.schedule
        // es idempotente (enqueueUniquePeriodicWork + KEEP), así que no hay
        // problema en que AlertsRepository también la llame por su cuenta.
        AlertsSyncWorker.schedule(appContext)
        return Result.success(link)
    }

    private suspend fun uploadLocalCustomizations(
        api: AgentApiClient,
        localHistory: HistoryRepository,
        localNetworkKey: NetworkKey,
        network: CurrentNetwork,
    ) {
        val local = localHistory.observeDevices(localNetworkKey).first()
        if (local.none { it.custom.hasCustomization() }) return

        val gateway = network.gateway ?: return
        val subnet = NetworkKey.subnetOf(network.cidr) ?: return
        val agentNetworks = api.networks().getOrNull() ?: return
        val match = AgentModeSelector.matchingNetwork(agentNetworks, gateway, subnet) ?: return
        val remote = api.devicesForNetwork(match.key).getOrNull()?.map(KnownDeviceDto::toDomain) ?: return

        for ((id, patch) in AgentCustomizationMigrator.plan(local, remote)) {
            api.patchCustomization(id, patch)
        }
    }

    /** Desvincular (ver el encargo, punto 5: "Ajustes mínimos: desvincular"). */
    fun forget() {
        deactivate()
        linkStore.clear()
        _panelState.value = AgentPanelState.NotLinked
        // No seguir sondeando alertas de un agente que esta app ya no conoce.
        AlertsSyncWorker.cancel(appContext)
        // "Avisos al instante" es un ajuste de ESTE vínculo (ver
        // AgentSettingsScreen): sin agente no hay nada que vigilar en vivo, y
        // no debe quedar "fantasma" prendido para el próximo que se vincule.
        alertsStore.setLiveWatchEnabled(false)
        LiveWatchService.stop(appContext)
    }

    fun linkedAgentName(): String? = linkStore.get()?.agentName
    fun linkedHostAndPort(): Pair<String, Int>? = linkStore.get()?.let { it.host to it.port }

    // ---- "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1") ----

    fun isLiveWatchEnabled(): Boolean = alertsStore.liveWatchEnabled()

    /** Lo toca el interruptor de [com.baxner.vigia.ui.agent.AgentSettingsScreen]. */
    fun setLiveWatchEnabled(enabled: Boolean) {
        alertsStore.setLiveWatchEnabled(enabled)
        syncLiveWatch()
    }

    /**
     * Prende o apaga [LiveWatchService] según [LiveWatchPolicy], con el
     * [panelState] vigente. Las llamadas a `ContextCompat.startForegroundService`/
     * `stopService` son baratas e idempotentes (ver [LiveWatchService]): no
     * hace falta rastrear "ya estaba corriendo" acá.
     */
    private fun syncLiveWatch() {
        if (LiveWatchPolicy.shouldRun(alertsStore.liveWatchEnabled(), _panelState.value)) {
            LiveWatchService.start(appContext)
        } else {
            LiveWatchService.stop(appContext)
        }
    }

    fun discoverAgents() = AgentDiscovery(appContext).discover()

    /** `GET /api/v1/info` sin token, para la pantalla de vinculación ("vas a vincularte con X"). TOFU: todavía no hay huella que fijar (ver [pair]). */
    suspend fun previewAgent(host: String, port: Int, network: CurrentNetwork): Result<AgentInfoDto> =
        AgentApiClient(httpClientFor(network), AgentTls.httpsBaseUrl(host, port)) { null }.info()

    /**
     * [pinnedFingerprint] viene de [AgentLink.certSha256]: con huella, el
     * cliente exige ESE certificado exacto (ver
     * [AgentTls.pinnedSocketFactory]); sin ella (vinculación nueva,
     * previsualización, o la lectura por HTTP de
     * [migrateLegacyLinkIfNeeded]), confía en lo que presente el agente
     * (ver [AgentTls.trustOnFirstUseSocketFactory] — y para una llamada
     * puramente HTTP, como la de la migración, esta configuración TLS
     * simplemente no se usa). Sin hostname en ningún caso (ver TLS v1.5:
     * "es una IP de la LAN o de Tailscale, no un nombre").
     */
    private fun httpClientFor(network: CurrentNetwork, pinnedFingerprint: String? = null): OkHttpClient {
        val normalizedPin = AgentTls.normalizeFingerprint(pinnedFingerprint)
        val key = network.network to normalizedPin
        synchronized(networkClientsLock) {
            networkClients[key]?.let { return it }
            val builder = baseHttpClient.newBuilder()
            // Con VPN activa (Tailscale) hay que salir por la Wi-Fi, no por el túnel — igual que el escaneo (ver NetworkInfoProvider.bindProcessTo).
            network.network?.let { builder.socketFactory(it.socketFactory) }
            val (sslSocketFactory, trustManager) = if (normalizedPin != null) {
                AgentTls.pinnedSocketFactory(normalizedPin)
            } else {
                AgentTls.trustOnFirstUseSocketFactory()
            }
            builder.sslSocketFactory(sslSocketFactory, trustManager)
            builder.hostnameVerifier { _, _ -> true } // IP de la LAN o de Tailscale: no hay hostname que comparar (ver TLS v1.5).
            return builder.build().also { networkClients[key] = it }
        }
    }

    /** Solo se llama con un [link] ya migrado (ver [evaluateLocked]/[migrateLegacyLinkIfNeeded]): siempre HTTPS. */
    private fun baseUrlFor(link: AgentLink) = AgentTls.httpsBaseUrl(link.host, link.port)

    private fun apiClientFor(link: AgentLink, network: CurrentNetwork) =
        AgentApiClient(httpClientFor(network, link.certSha256), baseUrlFor(link)) { linkStore.get()?.token }

    /**
     * A diferencia de [httpClientFor] — que ata el socket a la Wi-Fi actual
     * para no salir por el túnel de Tailscale estando en casa—, este cliente
     * no recibe ningún [CurrentNetwork] ni llama a `socketFactory(...)`: lo
     * usan [probeRemote]/[activateRemote] (ver el encargo, fase 8) para que
     * el sistema decida la ruta, que es como Tailscale espera que se hable
     * con él desde fuera de casa (es una VPN de Android, no una red Wi-Fi
     * más). Comparte el pool de conexiones y timeouts de [baseHttpClient]; la
     * huella se fija aparte (ver [resolvePinnedReachableAgent]/[pinnedRemoteClient]).
     */
    private fun remoteHttpClient(): OkHttpClient = baseHttpClient

    private fun pinnedRemoteClient(fingerprint: String): OkHttpClient {
        val normalized = AgentTls.normalizeFingerprint(fingerprint) ?: fingerprint
        synchronized(networkClientsLock) {
            return pinnedRemoteClients.getOrPut(normalized) { baseHttpClient.pinnedTo(normalized) }
        }
    }

    private fun clientName(): String = "${Build.MANUFACTURER} ${Build.MODEL}".trim().ifBlank { "Android" }

    private companion object {
        const val MaxCachedHttpClients = 8
    }
}

private fun parseInstantOrNull(text: String): Instant? = runCatching { Instant.parse(text) }.getOrNull()

/** Singleton de proceso, igual que [com.baxner.vigia.data.history.HistoryModule]. */
object AgentModule {
    @Volatile private var instance: AgentModeController? = null

    fun controller(context: Context): AgentModeController =
        instance ?: synchronized(this) {
            instance ?: AgentModeController(context.applicationContext).also { instance = it }
        }
}
