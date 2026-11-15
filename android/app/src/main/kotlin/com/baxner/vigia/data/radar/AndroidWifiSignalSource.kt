package com.baxner.vigia.data.radar

import android.content.Context
import android.content.pm.PackageManager
import android.net.wifi.ScanResult
import android.net.wifi.WifiManager
import android.net.wifi.rtt.RangingRequest
import android.net.wifi.rtt.RangingResult
import android.net.wifi.rtt.RangingResultCallback
import android.net.wifi.rtt.WifiRttManager
import android.os.Build
import androidx.annotation.RequiresApi
import com.baxner.vigia.data.network.WifiScanner
import java.util.concurrent.Executor
import kotlin.coroutines.resume
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.suspendCancellableCoroutine

/**
 * Señal real de un punto de acceso Wi-Fi (fase 6, ver docs/plan.md "Buscar
 * señal" y RadarContract.kt). Dos caminos muy distintos según
 * [SignalTarget.WifiAp.connected]:
 * - El AP conectado se lee sin escanear ([connectedSamples], cada ~1 s).
 * - Cualquier otro AP necesita escanear ([scannedSamples], reusa
 *   [WifiScanner]; cada ~3 s, o ~30 s si Android limita los escaneos).
 * Encima de cualquiera de los dos, si el objetivo y el teléfono soportan
 * Wi-Fi RTT, se suman muestras de distancia ([rttSamples]).
 */
class AndroidWifiSignalSource(context: Context) : SignalSource {
    private val appContext = context.applicationContext
    private val wifiManager = appContext.getSystemService(Context.WIFI_SERVICE) as? WifiManager
    private val wifiScanner = WifiScanner(appContext)

    private val rttHelper: RttHelper? = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
        RttHelper.createIfSupported(appContext, wifiManager)
    } else {
        null
    }

    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun samples(target: SignalTarget): Flow<SignalSample> {
        require(target is SignalTarget.WifiAp) { "AndroidWifiSignalSource solo atiende SignalTarget.WifiAp, llegó $target" }
        val rssi = if (target.connected) connectedSamples(target) else scannedSamples(target)
        val rtt = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P && target.rttCapable) {
            rttHelper?.samples(target)
        } else {
            null
        }
        return if (rtt == null) rssi else merge(rssi, rtt)
    }

    /**
     * AP conectado: encuesta propia a `WifiManager.connectionInfo` cada
     * ~1 s, no `ConnectivityManager.NetworkCallback` con
     * `NetworkCapabilities.getTransportInfo()` (la alternativa "moderna",
     * API 31+, que de hecho usa `@Suppress("DEPRECATION")` para evitar).
     * Se descartó el NetworkCallback porque Android no garantiza que
     * `onCapabilitiesChanged` avise cada cambio de RSSI a este ritmo: el
     * sistema limita esos avisos para no despertar la radio seguido
     * (pensado para ahorrar batería, no para un medidor en vivo), así que
     * el ritmo quedaría fuera de nuestro control — justo lo que "Buscar
     * señal" necesita. Con una encuesta propia, el ritmo es el mismo en
     * toda la franja de API soportada (26-36) y no depende de qué tan
     * seguido decida avisar el sistema. No se pudo verificar en un
     * teléfono real qué tan seguido dispara de verdad el NetworkCallback
     * en la práctica (ver el reporte).
     */
    private fun connectedSamples(target: SignalTarget.WifiAp): Flow<SignalSample> = flow {
        while (true) {
            val manager = wifiManager
            val hasPermission = RadarPermissions.hasWifiScan(appContext)
            val radioOn = manager?.isWifiEnabled == true
            val info = if (hasPermission && radioOn) {
                runCatching {
                    @Suppress("DEPRECATION") // WifiManager.getConnectionInfo(): igual que WifiScanner.kt, sin reemplazo simple compatible con minSdk 26.
                    manager?.connectionInfo
                }.getOrNull()
            } else {
                null
            }
            val connectedToTarget = bssidEquals(info?.bssid, target.bssid)
            _state.value = wifiConnectedState(hasPermission, radioOn, connectedToTarget)
            if (connectedToTarget) {
                emit(SignalSample(atMillis = System.currentTimeMillis(), rssi = info!!.rssi))
            }
            delay(ConnectedIntervalMs)
        }
    }

    /** Cualquier AP que no sea el conectado: escanea con [wifiScanner] (ya hace la danza de permisos/broadcast/throttling, ver data/network/WifiScanner.kt). */
    private fun scannedSamples(target: SignalTarget.WifiAp): Flow<SignalSample> = flow {
        var consecutiveMisses = 0
        while (true) {
            val hasPermission = RadarPermissions.hasWifiScan(appContext)
            val radioOn = wifiManager?.isWifiEnabled == true
            var throttled = false
            var found = false
            if (hasPermission && radioOn) {
                val outcome = wifiScanner.scanNetworks()
                throttled = outcome.throttled
                val match = outcome.networks.firstOrNull { bssidEquals(it.bssid, target.bssid) }
                found = match != null
                consecutiveMisses = if (found) 0 else consecutiveMisses + 1
                if (match != null) emit(SignalSample(atMillis = System.currentTimeMillis(), rssi = match.rssiDbm))
            }
            _state.value = wifiScanState(hasPermission, radioOn, found, throttled, consecutiveMisses)
            delay(if (throttled) ThrottledScanIntervalMs else ScanIntervalMs)
        }
    }

    /**
     * Wi-Fi RTT (802.11mc, API 28+): necesita el [ScanResult] del AP de un escaneo
     * reciente, no alcanza con el BSSID solo — por eso lee
     * `wifiManager.scanResults` (la caché del sistema, sin pedir un
     * escaneo nuevo: no compite por el cupo de 4 escaneos/2 min con
     * [scannedSamples]). Si el AP nunca aparece en esa caché (puede pasar
     * con el AP conectado: algunos teléfonos no refrescan ahí el BSSID al
     * que ya están asociados) sencillamente no hay muestras RTT — no se
     * fuerza un escaneo extra solo para esto. No se pudo probar en un
     * teléfono con soporte real (ver el reporte).
     */
    @RequiresApi(Build.VERSION_CODES.P)
    private class RttHelper(
        private val appContext: Context,
        private val wifiManager: WifiManager?,
        private val manager: WifiRttManager,
    ) {
        fun samples(target: SignalTarget.WifiAp): Flow<SignalSample>? {
            if (!runCatching { manager.isAvailable }.getOrDefault(false)) return null
            return flow {
                while (true) {
                    delay(RttIntervalMs)
                    if (!RadarPermissions.hasWifiScan(appContext)) continue
                    val scanResult = try {
                        @Suppress("DEPRECATION")
                        wifiManager?.scanResults
                    } catch (_: SecurityException) {
                        null
                    }
                        ?.firstOrNull { bssidEquals(it.BSSID, target.bssid) }
                        ?: continue
                    val result = rangeOnce(scanResult) ?: continue
                    if (result.status == RangingResult.STATUS_SUCCESS) {
                        emit(
                            SignalSample(
                                atMillis = System.currentTimeMillis(),
                                rssi = null,
                                rttMeters = result.distanceMm / 1000.0,
                                rttStdDevMeters = result.distanceStdDevMm / 1000.0,
                            ),
                        )
                    }
                }
            }
        }

        private suspend fun rangeOnce(scanResult: ScanResult): RangingResult? =
            suspendCancellableCoroutine { continuation ->
                val request = RangingRequest.Builder().addAccessPoint(scanResult).build()
                val callback = object : RangingResultCallback() {
                    override fun onRangingFailure(code: Int) {
                        if (continuation.isActive) continuation.resume(null)
                    }

                    override fun onRangingResults(results: MutableList<RangingResult>) {
                        if (continuation.isActive) continuation.resume(results.firstOrNull())
                    }
                }
                val started = try {
                    manager.startRanging(request, DirectExecutor, callback)
                    true
                } catch (_: SecurityException) {
                    false
                }
                if (!started && continuation.isActive) continuation.resume(null)
            }

        companion object {
            fun createIfSupported(appContext: Context, wifiManager: WifiManager?): RttHelper? {
                val manager = runCatching {
                    if (appContext.packageManager.hasSystemFeature(PackageManager.FEATURE_WIFI_RTT)) {
                        appContext.getSystemService(Context.WIFI_RTT_RANGING_SERVICE) as? WifiRttManager
                    } else {
                        null
                    }
                }.getOrNull() ?: return null
                return RttHelper(appContext, wifiManager, manager)
            }
        }
    }

    private companion object {
        const val ConnectedIntervalMs = 1_000L
        const val ScanIntervalMs = 3_000L
        const val ThrottledScanIntervalMs = 30_000L
        const val RttIntervalMs = 500L

        // El callback de startRanging solo resuelve una continuación (trabajo
        // mínimo): correrlo en el hilo que ya usa el sistema para avisar
        // evita sumar un Dispatcher más solo para esto.
        val DirectExecutor = Executor { command -> command.run() }
    }
}
