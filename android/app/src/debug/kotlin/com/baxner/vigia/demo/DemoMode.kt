package com.baxner.vigia.demo

import android.content.Intent
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.WifiNetworkInfo
import kotlinx.coroutines.delay

/**
 * Modo demo: solo existe en builds debug. `src/main` lo referencia por su
 * nombre de paquete+clase, que también existe en `src/release` (como un
 * stub vacío, ver ese archivo) para que compile en los dos sabores sin que
 * ningún dato ni texto de muestra viaje en el APK de release.
 *
 * Se activa con:
 *   adb shell am start -n com.baxner.vigia/.MainActivity --ez demo true
 *
 * Pensado para revisar la app en un emulador, que no ve la LAN real.
 */
object DemoMode {
    private const val EXTRA_DEMO = "demo"

    fun isRequestedBy(intent: Intent?): Boolean = intent?.getBooleanExtra(EXTRA_DEMO, false) == true

    val network: CurrentNetwork get() = DemoFixtures.currentNetwork
    val scanResult: ScanResult get() = DemoFixtures.scanResult
    val wifiNetworks: List<WifiNetworkInfo> get() = DemoFixtures.wifiNetworks

    /** Puebla [repository] con el historial de muestra (ver [DemoFixtures.seedHistory]). */
    suspend fun seedHistory(repository: HistoryRepository, network: NetworkKey) =
        DemoFixtures.seedHistory(repository, network)

    /**
     * Progreso simulado (sweep → names → identify) de ~4 s para el botón
     * "Escanear" en demo, antes de volver a mostrar los mismos datos.
     */
    suspend fun simulateScan(onProgress: (stage: String, done: Long, total: Long) -> Unit) {
        val total = DemoFixtures.scanResult.devices.size.toLong()
        onProgress("sweep", 0L, 0L)
        delay(1100)
        onProgress("names", 0L, 0L)
        delay(900)
        val steps = total.coerceAtLeast(1)
        val stepDelayMs = 2000L / steps
        for (i in 1..steps) {
            delay(stepDelayMs)
            onProgress("identify", i, total)
        }
    }
}
