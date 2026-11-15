package com.baxner.vigia.ui.security

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.data.agent.AlertsModule
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.WifiScanner
import com.baxner.vigia.data.network.WifiSecurity
import com.baxner.vigia.data.security.SecurityModule
import com.baxner.vigia.data.security.SecurityRepository
import com.baxner.vigia.data.security.WifiInfo
import com.baxner.vigia.data.security.isSecurityEndpointMissing
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SecurityUiState(
    val loading: Boolean = false,
    val report: SecurityReport? = null,
    /** La red actual la vigila el agente ("Red monitoreada"): decide el texto del estado vacío si [report] es null. */
    val monitoredByAgent: Boolean = false,
    /** Un [refresh][SecurityRepository.refresh] falló por un error de red (no por 404/501 de un agente todavía sin este endpoint, ver [isSecurityEndpointMissing]) mientras había agente vinculado vigilando esta red. */
    val agentUnreachable: Boolean = false,
    /** Último [RouterInfo] guardado (ver [SecurityRepository.lastRouterInfo]), para "Tu router visto desde aquí" aunque [report] sea null. */
    val lastRouterInfo: RouterInfo? = null,
)

/** Solo lo transitorio de esta pantalla; [SecurityUiState.report] sale de [SecurityRepository.report] directo (ver la revisión de la fase 5, punto 2), no de aquí. */
private data class TransientState(
    val loading: Boolean = false,
    val monitoredByAgent: Boolean = false,
    val agentUnreachable: Boolean = false,
    val lastRouterInfo: RouterInfo? = null,
)

/**
 * Pantalla Seguridad (ver el encargo, puntos 2 y 3): envuelve
 * [SecurityRepository] con lo único que le falta para armar el
 * [com.baxner.vigia.data.security.SecurityInput] y que el repositorio no
 * sabe por sí solo — la Wi-Fi actual del teléfono (ver el contrato §
 * "Seguridad" → "Un solo motor") y las alertas recientes cacheadas. Los
 * equipos y la red actual los trae quien llama (la misma
 * [com.baxner.vigia.ui.scan.ScanViewModel] que ya usan Panel y Dispositivos,
 * ver `SecurityScreen.kt`): así no hay dos copias del historial.
 *
 * [uiState.report][SecurityUiState.report] sale de [SecurityRepository.report]
 * (el `StateFlow` compartido por todo el proceso): si la pantalla Seguridad
 * refresca, la tile del Panel —una instancia distinta de este ViewModel—
 * se entera sola, sin esperar su propio refresco (ver la revisión de la
 * fase 5, punto 2).
 */
class SecurityViewModel(application: Application) : AndroidViewModel(application) {
    private val repository: SecurityRepository = SecurityModule.repository(application)
    private val alertsRepository = AlertsModule.repository(application)
    private val wifiScanner = WifiScanner(application)

    private val _transient = MutableStateFlow(
        TransientState(monitoredByAgent = repository.monitoredByAgent, lastRouterInfo = repository.lastRouterInfo()),
    )

    val uiState: StateFlow<SecurityUiState> = combine(repository.report, _transient) { report, transient ->
        SecurityUiState(
            loading = transient.loading,
            report = report,
            monitoredByAgent = transient.monitoredByAgent,
            agentUnreachable = transient.agentUnreachable,
            lastRouterInfo = transient.lastRouterInfo,
        )
    }.stateIn(
        viewModelScope,
        SharingStarted.WhileSubscribed(5_000),
        SecurityUiState(report = repository.report.value, monitoredByAgent = repository.monitoredByAgent, lastRouterInfo = repository.lastRouterInfo()),
    )

    /** La Wi-Fi solo se vuelve a escanear en [onOpen] (ver esa nota); [onDevicesChanged] reusa esto. */
    private var lastWifiInfo: WifiInfo? = null
    private var lastWifiBssid: String? = null

    /** Lo último que mandó la pantalla (ver [onOpen]/[onDevicesChanged]): lo reusan [acknowledge]/[unacknowledge] para refrescar sin que la UI tenga que volver a pasarlo. */
    private var lastDevices: List<KnownDevice> = emptyList()
    private var lastNetwork: CurrentNetwork? = null

    /** El refresco pendiente (de [onOpen] o [onDevicesChanged]): el segundo cancela al primero, nunca corren dos a la vez desde este ViewModel (el [SecurityRepository] además se protege solo con su Mutex, ver la revisión de la fase 5, punto 1). */
    private var refreshJob: Job? = null

    /** Al abrir la pantalla (ver el encargo, punto 2: "Refresca al abrir la pantalla"): escanea la Wi-Fi una vez — para saber si el punto de acceso actual es abierto/WEP/WPS, algo que solo sabe el teléfono — y refresca ya mismo (`force = true`: el usuario acaba de abrir la pantalla, no sirve un dato de hace 59 s). */
    fun onOpen(devices: List<KnownDevice>, network: CurrentNetwork?) {
        lastDevices = devices
        lastNetwork = network
        syncWifiContext(network)
        refreshJob?.cancel()
        refreshJob = viewModelScope.launch {
            val scanned = runCatching { wifiInfoFor(network) }.getOrNull()
            if (scanned != null) {
                lastWifiInfo = scanned
                lastWifiBssid = network?.bssid?.takeIf { it.isNotBlank() }?.lowercase()
            } else if (network?.isWifi != true) {
                lastWifiInfo = null
                lastWifiBssid = null
            }
            doRefresh(devices, network, force = true)
        }
    }

    /**
     * "Y después de cada escaneo" (ver el encargo, punto 2): sin rescanear
     * la Wi-Fi de nuevo (4 escaneos cada 2 min es poco para pedir uno por
     * cada escaneo de red) — reusa la que ya se sabía. Es lo que llaman
     * tanto la pantalla Seguridad en escaneos siguientes como la tile del
     * Panel (ver `ui/panel/PanelScreen.kt`), que nunca abre esta pantalla.
     *
     * Con espera de [DebounceMs] y cancelando el pedido anterior (ver la
     * revisión de la fase 5, punto 1): en Red monitoreada la lista de
     * equipos cambia con cada evento del stream del agente, decenas por
     * escaneo, y no hace falta refrescar por cada uno — solo cuando se
     * asienta. El [SecurityRepository.refresh] además nunca pide dos veces
     * dentro del mismo minuto salvo `force`.
     */
    fun onDevicesChanged(devices: List<KnownDevice>, network: CurrentNetwork?) {
        lastDevices = devices
        lastNetwork = network
        syncWifiContext(network)
        refreshJob?.cancel()
        refreshJob = viewModelScope.launch {
            delay(DebounceMs)
            if (network?.isWifi == true && lastWifiInfo == null) {
                lastWifiInfo = runCatching { cachedWifiInfoFor(network) }.getOrNull()
                if (lastWifiInfo != null) {
                    lastWifiBssid = network.bssid?.takeIf { it.isNotBlank() }?.lowercase()
                }
            }
            doRefresh(devices, network, force = false)
        }
    }

    /**
     * "Ya lo sé, es a propósito" (ver el encargo, punto 2): reconoce y
     * refresca el informe ya mismo (ver [SecurityRepository.acknowledge] y
     * el contrato § "Hallazgos reconocidos v1.3": "después de reconocer, se
     * refresca el informe").
     */
    fun acknowledge(findingId: String) {
        viewModelScope.launch {
            repository.acknowledge(findingId, lastNetwork)
            doRefresh(lastDevices, lastNetwork, force = true)
        }
    }

    /** "Deshacer" (ver [acknowledge]). */
    fun unacknowledge(findingId: String) {
        viewModelScope.launch {
            repository.unacknowledge(findingId, lastNetwork)
            doRefresh(lastDevices, lastNetwork, force = true)
        }
    }

    private suspend fun doRefresh(devices: List<KnownDevice>, network: CurrentNetwork?, force: Boolean) {
        _transient.update { it.copy(loading = true, monitoredByAgent = repository.monitoredByAgent) }
        val recentAlerts = alertsRepository.alerts.value
        val result = repository.refresh(devices, network, lastWifiInfo, recentAlerts, force = force)
        _transient.update { current ->
            current.copy(
                loading = false,
                monitoredByAgent = repository.monitoredByAgent,
                // 404/501 (el agente instalado todavía no tiene /api/v1/security, ver la
                // revisión de la fase 5, punto 3) no es que esté inalcanzable.
                agentUnreachable = result.isFailure && repository.monitoredByAgent && !result.exceptionOrNull().isSecurityEndpointMissing(),
                lastRouterInfo = repository.lastRouterInfo(),
            )
        }
    }

    /** Ver [SecurityRepository.routerSeenFromHere]. */
    fun routerSeenFromHere(phoneCidr: String?): Boolean = repository.routerSeenFromHere(phoneCidr)

    private fun syncWifiContext(network: CurrentNetwork?) {
        if (network?.isWifi != true) {
            lastWifiInfo = null
            lastWifiBssid = null
            return
        }
        val currentBssid = network.bssid?.takeIf { it.isNotBlank() }?.lowercase()
        if (currentBssid != null && lastWifiBssid != null && currentBssid != lastWifiBssid) {
            lastWifiInfo = null
            lastWifiBssid = currentBssid
        } else if (lastWifiBssid == null && currentBssid != null) {
            lastWifiBssid = currentBssid
        }
    }

    /** La [WifiInfo] del contrato para el punto de acceso al que está conectado el teléfono ahora, o null sin Wi-Fi/sin BSSID. */
    private suspend fun wifiInfoFor(network: CurrentNetwork?): WifiInfo? {
        if (network?.isWifi != true) return null
        val bssid = network.bssid?.takeIf { it.isNotBlank() }
        val outcome = wifiScanner.scanNetworks()
        val connected = bssid?.let { target ->
            outcome.networks.firstOrNull { it.bssid.equals(target, ignoreCase = true) }
        }
        if (connected != null) {
            return WifiInfo(ssid = network.ssid.orEmpty(), security = connected.security.toWireValue(), wps = connected.hasWps)
        }
        return cachedWifiInfoFor(network)
    }

    private fun cachedWifiInfoFor(network: CurrentNetwork?): WifiInfo? {
        if (network?.isWifi != true) return null
        val snapshot = wifiScanner.cachedConnectedWifi(network.bssid) ?: return null
        return WifiInfo(ssid = network.ssid.orEmpty(), security = snapshot.security.toWireValue(), wps = snapshot.hasWps)
    }

    private fun WifiSecurity.toWireValue(): String = when (this) {
        WifiSecurity.OPEN -> "open"
        WifiSecurity.UNKNOWN -> ""
        WifiSecurity.WEP -> "wep"
        WifiSecurity.WPA -> "wpa"
        WifiSecurity.WPA2 -> "wpa2"
        WifiSecurity.WPA3 -> "wpa3"
    }

    private companion object {
        /** Ver [onDevicesChanged]. */
        const val DebounceMs = 2_000L
    }
}
