package com.baxner.vigia.data.radar

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * Fakes de "Buscar señal" (fase 6, ver docs/plan.md) para HuntViewModelTest,
 * BluetoothViewModelTest y capturas que quieran manejar las fuentes a mano.
 */

/**
 * [SignalSource] de prueba: [emit] manda una lectura a quien esté
 * coleccionando [samples] en ese momento, como haría una fuente real
 * (Wi-Fi o Bluetooth) al llegar un escaneo o anuncio nuevo. Ignora [target]
 * a propósito: en las pruebas y en [com.baxner.vigia.ui.radar.HuntScreen]
 * solo se busca un objetivo a la vez.
 */
class FakeSignalSource : SignalSource {
    private val flow = MutableSharedFlow<SignalSample>(extraBufferCapacity = 64)
    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun samples(target: SignalTarget): Flow<SignalSample> = flow

    suspend fun emit(sample: SignalSample) = flow.emit(sample)

    fun setState(value: SignalSourceState) {
        _state.value = value
    }
}

/** [HeadingSource] de prueba: [emit] manda un rumbo a quien esté coleccionando [headings]. */
class FakeHeadingSource : HeadingSource {
    private val flow = MutableSharedFlow<HeadingSample>(extraBufferCapacity = 64)

    override fun headings(): Flow<HeadingSample> = flow

    suspend fun emit(sample: HeadingSample) = flow.emit(sample)
}

/**
 * [BleScanner] de prueba: [setDevices]/[setState] quedan como el último
 * valor, visible de inmediato a quien colecciona (como un escaneo real que ya
 * está en curso), a diferencia de [FakeSignalSource]/[FakeHeadingSource] que
 * modelan lecturas puntuales.
 */
class FakeBleScanner : BleScanner {
    private val _devices = MutableStateFlow<List<BleDevice>>(emptyList())
    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun devices(): Flow<List<BleDevice>> = _devices.asStateFlow()

    fun setDevices(devices: List<BleDevice>) {
        _devices.value = devices
    }

    fun setState(value: SignalSourceState) {
        _state.value = value
    }
}
