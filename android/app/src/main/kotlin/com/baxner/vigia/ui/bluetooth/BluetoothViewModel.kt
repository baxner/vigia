package com.baxner.vigia.ui.bluetooth

import com.baxner.vigia.data.radar.BleDevice
import com.baxner.vigia.data.radar.BleScanner
import com.baxner.vigia.data.radar.SignalSourceState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class BluetoothUiState(
    /** Del más reciente al más viejo, como los entrega [BleScanner]; la pantalla ordena/filtra al mostrar. */
    val devices: List<BleDevice> = emptyList(),
    val state: SignalSourceState = SignalSourceState.OK,
    val onlyNamed: Boolean = false,
)

/**
 * Pestaña Bluetooth (fase 6, ver docs/plan.md): envuelve [BleScanner]. Igual
 * que [com.baxner.vigia.ui.radar.HuntViewModel], no es un
 * `androidx.lifecycle.ViewModel` de Android — su dueño ([BluetoothScreen]) lo
 * crea con `remember` y gobierna [start]/[stop] con el ciclo de vida: escanea
 * solo mientras la pestaña está visible.
 */
class BluetoothViewModel(private val bleScanner: BleScanner) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var job: Job? = null

    private val _uiState = MutableStateFlow(BluetoothUiState())
    val uiState: StateFlow<BluetoothUiState> = _uiState.asStateFlow()

    fun start() {
        if (job?.isActive == true) return
        job = scope.launch {
            launch { bleScanner.devices().collect { devices -> _uiState.update { it.copy(devices = devices) } } }
            launch { bleScanner.state.collect { state -> _uiState.update { it.copy(state = state) } } }
        }
    }

    fun stop() {
        job?.cancel()
        job = null
    }

    fun onCleared() {
        stop()
        scope.cancel()
    }

    fun setOnlyNamed(value: Boolean) {
        _uiState.update { it.copy(onlyNamed = value) }
    }
}
