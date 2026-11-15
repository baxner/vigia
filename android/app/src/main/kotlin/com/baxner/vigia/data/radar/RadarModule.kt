package com.baxner.vigia.data.radar

import android.content.Context
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.launch

/**
 * De dónde saca la UI las fuentes de "Buscar señal" (fase 6). La señal y el
 * escáner Bluetooth son singletons de proceso (puro acceso a radios, con el
 * contexto de la aplicación; así no hay dos `BluetoothLeScanner` a la vez);
 * la brújula es una por pantalla (ver [headingSource]). La UI llama solo a
 * estas tres funciones.
 */
object RadarModule {
    @Volatile private var companiesInstance: BluetoothCompanies? = null
    @Volatile private var signalSourceInstance: SignalSource? = null
    @Volatile private var bleScannerInstance: BleScanner? = null

    fun signalSource(context: Context): SignalSource = signalSourceInstance ?: synchronized(this) {
        signalSourceInstance ?: DelegatingSignalSource(
            wifiSource = AndroidWifiSignalSource(context),
            bleSource = AndroidBleSignalSource(context),
        ).also { signalSourceInstance = it }
    }

    fun bleScanner(context: Context): BleScanner = bleScannerInstance ?: synchronized(this) {
        bleScannerInstance ?: AndroidBleScanner(context, companies(context)).also { bleScannerInstance = it }
    }

    /**
     * Una brújula por pantalla, NO un singleton: necesita el contexto de la
     * Activity para saber cómo está girada la pantalla, y un singleton
     * retendría esa Activity para siempre. Pásale el contexto de la pantalla
     * (LocalContext); solo escucha sensores mientras alguien colecta.
     */
    fun headingSource(context: Context): HeadingSource = RotationHeadingSource(context)

    private fun companies(context: Context): BluetoothCompanies = companiesInstance ?: synchronized(this) {
        companiesInstance ?: BluetoothCompanies.load(context).also { companiesInstance = it }
    }
}

/**
 * [SignalSource] que atiende los dos tipos de [SignalTarget] (ver
 * RadarContract.kt: "una fuente que delegue según el tipo"), repartiendo a
 * [wifiSource] o [bleSource] según corresponda. [state] refleja el de la
 * fuente que esté atendiendo el [samples] que alguien esté colectando en
 * ese momento: como "Buscar señal" persigue un solo objetivo a la vez,
 * alcanza con un único StateFlow compartido en vez de uno por objetivo.
 */
private class DelegatingSignalSource(
    private val wifiSource: SignalSource,
    private val bleSource: SignalSource,
) : SignalSource {
    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun samples(target: SignalTarget): Flow<SignalSample> {
        val delegate = when (target) {
            is SignalTarget.WifiAp -> wifiSource
            is SignalTarget.Ble -> bleSource
        }
        return channelFlow {
            // El estado de la fuente delegada se reenvía mientras dure esta
            // colección; launch (no collect directo) para poder recibir
            // samples y state al mismo tiempo. channelFlow cancela este hijo
            // solo al cancelar la colección de afuera.
            launch { delegate.state.collect { _state.value = it } }
            delegate.samples(target).collect { send(it) }
        }
    }
}
