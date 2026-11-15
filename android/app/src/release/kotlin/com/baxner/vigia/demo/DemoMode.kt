package com.baxner.vigia.demo

import android.content.Intent
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.WifiNetworkInfo

/**
 * Stub de release: el modo demo NO existe en release (ver el real en
 * `src/debug/.../DemoMode.kt`). Mismo nombre de paquete+clase para que
 * `src/main` compile en los dos sabores sin un solo dato ni texto de muestra
 * en este APK; [isRequestedBy] siempre da falso, así que el resto de esta
 * clase nunca se llama en producción (y si se llamara, no revienta).
 */
object DemoMode {
    fun isRequestedBy(intent: Intent?): Boolean = false

    val network: CurrentNetwork
        get() = CurrentNetwork(
            cidr = null, gateway = null, isWifi = false, ssid = null, bssid = null,
            rssi = null, frequencyMhz = null, linkSpeedMbps = null,
        )
    val scanResult: ScanResult get() = ScanResult()
    val wifiNetworks: List<WifiNetworkInfo> get() = emptyList()

    suspend fun simulateScan(onProgress: (stage: String, done: Long, total: Long) -> Unit) {
        // No-op: en release jamás se activa el modo demo.
    }

    suspend fun seedHistory(repository: HistoryRepository, network: NetworkKey) {
        // No-op: en release jamás se activa el modo demo.
    }
}
