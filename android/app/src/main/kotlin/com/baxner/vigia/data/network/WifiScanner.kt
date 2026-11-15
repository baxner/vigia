package com.baxner.vigia.data.network

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.wifi.WifiManager
import android.os.Build
import androidx.core.content.ContextCompat
import com.baxner.vigia.engine.mobile.Mobile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/** Una red Wi-Fi vista en el último escaneo, ya lista para mostrar. */
data class WifiNetworkInfo(
    val ssid: String,
    val bssid: String,
    val rssiDbm: Int,
    val frequencyMhz: Int,
    val band: WifiBand,
    val channel: Int,
    val channelWidthLabel: String,
    /** Canal central y rango si el ancho es mayor a 20 MHz; ver [channelSpanFor]. */
    val channelSpan: ChannelSpan? = null,
    /** Generación Wi-Fi (API 30+ solamente); ver [wifiStandardFor]. */
    val wifiStandard: WifiStandard? = null,
    val security: WifiSecurity,
    /** true si `capabilities` anuncia WPS (aprovisionamiento con PIN); ver [hasWps]. */
    val hasWps: Boolean = false,
    val vendor: String,
    /** Distancia aproximada por pérdida en espacio libre; ver [estimatedDistanceLabel]. */
    val distanceLabel: String = "? m",
    val isConnected: Boolean,
    /** Responde a Wi-Fi RTT (802.11mc): "Buscar señal" mide la distancia en vez de estimarla. */
    val rttResponder: Boolean = false,
)

data class WifiScanOutcome(
    val networks: List<WifiNetworkInfo>,
    /** true si Android frenó el escaneo manual y esto son resultados en caché. */
    val throttled: Boolean,
    /**
     * true si el sistema tiene prendida la limitación general de escaneos
     * (Opciones de desarrollador → "Limitación de búsqueda de Wi-Fi"), no si
     * este escaneo puntual la sufrió (eso es [throttled]). Solo se puede leer
     * desde API 30; en versiones viejas se asume `false`.
     */
    val scanThrottleEnabled: Boolean = false,
)

/**
 * Cuántos puntos de acceso visibles anuncian el SSID [ssid] (red mesh o
 * varios repetidores con el mismo nombre). 0 si [ssid] viene vacío: las
 * redes ocultas comparten SSID "" sin ser "la misma red".
 */
fun apCountForSsid(networks: List<WifiNetworkInfo>, ssid: String): Int =
    if (ssid.isBlank()) 0 else networks.count { it.ssid == ssid }

/**
 * Escanea las redes Wi-Fi visibles. `WifiManager.startScan()` está limitado a
 * 4 llamadas cada 2 minutos por app en foreground: si Android lo frena,
 * devolvemos `scanResults` (la última lista que tiene el sistema) marcando
 * `throttled = true` en vez de fallar.
 */
class WifiScanner(context: Context) {
    private val appContext = context.applicationContext
    private val wifiManager =
        appContext.getSystemService(Context.WIFI_SERVICE) as? WifiManager

    private var multicastLock: WifiManager.MulticastLock? = null

    /**
     * Hay que tenerlo tomado durante todo el escaneo de red (mDNS/SSDP), o el
     * Wi-Fi descarta esos paquetes multicast para ahorrar batería.
     */
    fun acquireMulticastLock() {
        if (multicastLock?.isHeld == true) return
        val lock = wifiManager?.createMulticastLock("vigia") ?: return
        lock.setReferenceCounted(true)
        lock.acquire()
        multicastLock = lock
    }

    fun releaseMulticastLock() {
        val lock = multicastLock ?: return
        if (lock.isHeld) lock.release()
        multicastLock = null
    }

    suspend fun scanNetworks(): WifiScanOutcome = withContext(Dispatchers.IO) {
        val manager = wifiManager ?: return@withContext WifiScanOutcome(emptyList(), throttled = false)

        val gotFreshResults = requestScanAndAwait(manager)

        val results = try {
            manager.scanResults ?: emptyList()
        } catch (_: SecurityException) {
            emptyList()
        }

        val connectedBssid = try {
            @Suppress("DEPRECATION")
            manager.connectionInfo?.bssid
        } catch (_: SecurityException) {
            null
        }

        val networks = results
            .map { it.toWifiNetworkInfo(connectedBssid) }
            .sortedByDescending { it.rssiDbm }

        // isScanThrottleEnabled() es API 30+; el && de más a la izquierda evita
        // llamarlo en versiones viejas de Android, donde el método ni existe.
        val scanThrottleEnabled = Build.VERSION.SDK_INT >= Build.VERSION_CODES.R &&
            runCatching { manager.isScanThrottleEnabled() }.getOrDefault(false)

        WifiScanOutcome(networks = networks, throttled = !gotFreshResults, scanThrottleEnabled = scanThrottleEnabled)
    }

    /** true si llegó el broadcast de resultados nuevos; false si hubo timeout o throttling. */
    private suspend fun requestScanAndAwait(manager: WifiManager): Boolean =
        withTimeoutOrNull(SCAN_WAIT_TIMEOUT_MS) {
            suspendCancellableCoroutine { continuation ->
                val receiver = object : BroadcastReceiver() {
                    override fun onReceive(ctx: Context, intent: Intent) {
                        val updated = intent.getBooleanExtra(WifiManager.EXTRA_RESULTS_UPDATED, true)
                        runCatching { appContext.unregisterReceiver(this) }
                        if (continuation.isActive) continuation.resumeWith(Result.success(updated))
                    }
                }
                // El receptor va ANTES de startScan(): si el escaneo termina rápido,
                // el broadcast no se pierde.
                ContextCompat.registerReceiver(
                    appContext,
                    receiver,
                    IntentFilter(WifiManager.SCAN_RESULTS_AVAILABLE_ACTION),
                    ContextCompat.RECEIVER_NOT_EXPORTED,
                )
                continuation.invokeOnCancellation {
                    runCatching { appContext.unregisterReceiver(receiver) }
                }
                val accepted = try {
                    @Suppress("DEPRECATION")
                    manager.startScan()
                } catch (_: SecurityException) {
                    false
                }
                if (!accepted) {
                    runCatching { appContext.unregisterReceiver(receiver) }
                    if (continuation.isActive) continuation.resumeWith(Result.success(false))
                }
            }
        } ?: false

    @Suppress("DEPRECATION") // ScanResult.SSID: sin reemplazo simple compatible con API 26.
    private fun android.net.wifi.ScanResult.toWifiNetworkInfo(connectedBssid: String?): WifiNetworkInfo {
        val channelInfo = channelInfoForFrequencyMhz(frequency)
        val bssidValue = BSSID ?: ""
        val vendor = vendorForBssid(bssidValue) { mac -> runCatching { Mobile.brandForMAC(mac) }.getOrDefault("") }
        val caps = capabilities ?: ""
        // ScanResult.wifiStandard es API 30+; en versiones viejas ni se lee
        // (el campo no existe ahí) y el estándar queda sin dato (null).
        val wifiStandardRaw = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) wifiStandard else -1
        return WifiNetworkInfo(
            ssid = SSID?.takeIf { it.isNotBlank() } ?: "",
            bssid = bssidValue,
            rssiDbm = level,
            frequencyMhz = frequency,
            band = channelInfo.band,
            channel = channelInfo.channel,
            channelWidthLabel = channelWidthLabel(channelWidth),
            channelSpan = channelSpanFor(centerFreq0, channelWidth),
            wifiStandard = wifiStandardFor(wifiStandardRaw, channelInfo.band),
            security = securityFromCapabilities(caps),
            hasWps = hasWps(caps),
            vendor = vendor,
            distanceLabel = estimatedDistanceLabel(frequency, level),
            isConnected = bssidValue.isNotBlank() && bssidValue == connectedBssid,
            // is80211mcResponder es API 28+.
            rttResponder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.P && is80211mcResponder,
        )
    }

    private companion object {
        const val SCAN_WAIT_TIMEOUT_MS = 6000L
    }
}
