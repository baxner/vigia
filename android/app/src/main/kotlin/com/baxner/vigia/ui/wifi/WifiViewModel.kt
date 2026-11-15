package com.baxner.vigia.ui.wifi

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.R
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.NetworkInfoProvider
import com.baxner.vigia.data.network.WifiNetworkInfo
import com.baxner.vigia.data.network.WifiScanner
import com.baxner.vigia.demo.DemoMode
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class WifiUiState(
    val networks: List<WifiNetworkInfo> = emptyList(),
    /** Velocidad de enlace e IP de la conexión activa; [WifiScanner] no puede darlas. */
    val currentNetwork: CurrentNetwork? = null,
    val isScanning: Boolean = false,
    val throttled: Boolean = false,
    /** true si el sistema tiene prendida la limitación general de escaneos (ver [WifiScanner]). */
    val scanThrottleEnabled: Boolean = false,
    val errorMessage: String? = null,
) {
    val hasScannedOnce: Boolean get() = networks.isNotEmpty() || (!isScanning && errorMessage == null)
}

class WifiViewModel(application: Application) : AndroidViewModel(application) {
    private val wifiScanner = WifiScanner(application)
    private val networkInfoProvider = NetworkInfoProvider(application)

    private val _uiState = MutableStateFlow(WifiUiState())
    val uiState: StateFlow<WifiUiState> = _uiState.asStateFlow()

    private var lastScanAtMs = 0L
    private var demoMode = false

    /** Modo demo (solo debug, ver [DemoMode]): una red Wi-Fi de muestra en vez de escanear. */
    fun enableDemoMode() {
        demoMode = true
        _uiState.update {
            it.copy(
                networks = DemoMode.wifiNetworks,
                currentNetwork = DemoMode.network,
                isScanning = false,
                throttled = false,
                // true a propósito en demo: así la captura de revisión visual también
                // ejercita el aviso de limitación de escaneos (ver WifiScreen).
                scanThrottleEnabled = true,
                errorMessage = null,
            )
        }
    }

    /**
     * Al entrar a la pestaña: escanea si no hay datos recientes. Android solo
     * permite 4 escaneos cada 2 minutos, así que no se gasta uno al abrir la
     * app ni cada vez que se cambia de pestaña.
     */
    fun rescanIfStale() {
        if (demoMode) return
        if (System.currentTimeMillis() - lastScanAtMs > STALE_AFTER_MS) rescan()
    }

    fun rescan() {
        if (_uiState.value.isScanning) return
        if (demoMode) {
            _uiState.update {
                it.copy(networks = DemoMode.wifiNetworks, currentNetwork = DemoMode.network, throttled = false, errorMessage = null)
            }
            return
        }
        lastScanAtMs = System.currentTimeMillis()
        viewModelScope.launch {
            _uiState.update { it.copy(isScanning = true, errorMessage = null) }
            runCatching { wifiScanner.scanNetworks() }
                .onSuccess { outcome ->
                    val current = runCatching { networkInfoProvider.current() }.getOrNull()
                    _uiState.update {
                        it.copy(
                            isScanning = false,
                            networks = outcome.networks,
                            currentNetwork = current,
                            throttled = outcome.throttled,
                            scanThrottleEnabled = outcome.scanThrottleEnabled,
                        )
                    }
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(
                            isScanning = false,
                            errorMessage = error.message ?: getApplication<Application>().getString(R.string.error_unknown),
                        )
                    }
                }
        }
    }

    private companion object {
        const val STALE_AFTER_MS = 30_000L
    }
}
