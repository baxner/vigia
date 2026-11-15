package com.baxner.vigia.ui.radar

import com.baxner.vigia.data.radar.AzimuthRssi
import com.baxner.vigia.data.radar.BearingEstimate
import com.baxner.vigia.data.radar.BearingEstimator
import com.baxner.vigia.data.radar.DistanceModel
import com.baxner.vigia.data.radar.DistanceRange
import com.baxner.vigia.data.radar.Geiger
import com.baxner.vigia.data.radar.HeadingSample
import com.baxner.vigia.data.radar.HeadingSource
import com.baxner.vigia.data.radar.RssiFilter
import com.baxner.vigia.data.radar.SignalSample
import com.baxner.vigia.data.radar.SignalSource
import com.baxner.vigia.data.radar.SignalSourceState
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.data.radar.Trend
import com.baxner.vigia.data.radar.TrendDetector
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

/** Ver RadarMath.kt y el encargo de la fase 6: ventana de tendencia y lecturas mínimas por sector, según el tipo de objetivo. */
private const val WifiTrendWindowMillis = 8_000L
private const val BleTrendWindowMillis = 3_000L
private const val WifiMinPerSector = 2
private const val BleMinPerSector = 5

/** Estado de la pantalla "Buscar señal" (fase 6, ver docs/plan.md). */
data class HuntUiState(
    val target: SignalTarget? = null,
    val sourceState: SignalSourceState = SignalSourceState.OK,
    /** Señal ya pasada por [RssiFilter]; null antes de la primera lectura con RSSI. */
    val smoothedRssi: Double? = null,
    val trend: Trend = Trend.STEADY,
    /** Rango aproximado por señal (ver [DistanceModel]); sigue presente aunque haya RTT, por si se pierde la medición exacta. */
    val distance: DistanceRange? = null,
    /** Distancia medida por Wi-Fi RTT (802.11mc), cuando el objetivo la trae; más precisa que [distance]. */
    val rttMeters: Double? = null,
    val headingDeg: Float? = null,
    val headingReliable: Boolean = true,
    /** Lecturas por sector de la vuelta guiada en curso (o de la última), para el anillo de 8 sectores. */
    val sectorCounts: List<Int> = List(BearingEstimator.Sectors) { 0 },
    val guidedTurnActive: Boolean = false,
    val bearing: BearingEstimate? = null,
    val soundEnabled: Boolean = true,
    val vibrationEnabled: Boolean = true,
) {
    /** Ritmo del "Geiger" para la señal actual, o null antes de la primera lectura (ver [Geiger.intervalMillis]). */
    val geigerIntervalMillis: Long? get() = smoothedRssi?.let(Geiger::intervalMillis)
}

/**
 * "Buscar señal" (fase 6, ver docs/plan.md y RadarContract.kt): combina
 * [SignalSource.samples] y [HeadingSource.headings] con las matemáticas puras
 * de RadarMath.kt (filtro de señal, distancia, tendencia, rumbo por ajuste de
 * coseno). No es un `androidx.lifecycle.ViewModel` de Android — no necesita
 * `Application` ni sobrevivir una rotación por sí solo — así que su dueño
 * ([HuntScreen]) lo crea con `remember` y gobierna [start]/[stop] según la
 * visibilidad de la pantalla (ver el encargo: "Fuentes y sensores solo con la
 * pantalla visible").
 */
class HuntViewModel(
    private val signalSource: SignalSource,
    private val headingSource: HeadingSource,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var collectionJob: Job? = null

    private val _uiState = MutableStateFlow(HuntUiState())
    val uiState: StateFlow<HuntUiState> = _uiState.asStateFlow()

    private var rssiFilter = RssiFilter.forWifi()
    private var trendDetector = TrendDetector(windowMillis = WifiTrendWindowMillis)
    private var minPerSector = WifiMinPerSector
    private val guidedTurnSamples = mutableListOf<AzimuthRssi>()
    private var lastReliableHeadingDeg: Double? = null

    /**
     * Empieza (o reanuda) a escuchar [target]: sin efecto si ya está activo
     * con el mismo objetivo. Cambiar de objetivo reinicia todo el estado
     * (filtro, tendencia, vuelta guiada); reanudar el mismo objetivo después
     * de [stop] conserva lo ya calculado.
     */
    fun start(target: SignalTarget) {
        if (collectionJob?.isActive == true && _uiState.value.target == target) return
        if (_uiState.value.target != target) resetFor(target)
        collectionJob?.cancel()
        collectionJob = scope.launch {
            launch { signalSource.samples(target).collect(::onSample) }
            launch { signalSource.state.collect(::onSourceState) }
            launch { headingSource.headings().collect(::onHeading) }
        }
    }

    /** Corta los escaneos/sensores (pantalla no visible); conserva el estado ya calculado para cuando [start] reanude. */
    fun stop() {
        collectionJob?.cancel()
        collectionJob = null
    }

    fun onCleared() {
        stop()
        scope.cancel()
    }

    private fun resetFor(target: SignalTarget) {
        rssiFilter = if (target is SignalTarget.Ble) RssiFilter.forBle() else RssiFilter.forWifi()
        trendDetector = TrendDetector(windowMillis = if (target is SignalTarget.Ble) BleTrendWindowMillis else WifiTrendWindowMillis)
        minPerSector = if (target is SignalTarget.Ble) BleMinPerSector else WifiMinPerSector
        guidedTurnSamples.clear()
        lastReliableHeadingDeg = null
        _uiState.value = HuntUiState(target = target)
    }

    private fun onSourceState(state: SignalSourceState) {
        _uiState.update { it.copy(sourceState = state) }
    }

    private fun onSample(sample: SignalSample) {
        val current = _uiState.value
        val target = current.target ?: return

        val smoothed = sample.rssi?.let { rssiFilter.update(it.toDouble()) } ?: current.smoothedRssi
        val trend = if (sample.rssi != null && smoothed != null) {
            trendDetector.update(sample.atMillis, smoothed)
        } else {
            current.trend
        }
        val distance = smoothed?.let { DistanceModel.estimate(it, DistanceModel.referenceAt1m(target)) } ?: current.distance
        val rtt = sample.rttMeters ?: current.rttMeters

        var sectorCounts = current.sectorCounts
        var bearing = current.bearing
        var guidedActive = current.guidedTurnActive
        // Vuelta guiada: cada lectura se empareja con el último rumbo confiable, con la señal
        // CRUDA. La suavizada va atrasada respecto del giro (el filtro tarda varias lecturas
        // en alcanzar el cambio) y eso corría el rumbo en el sentido de la vuelta; el ajuste
        // del coseno ya promedia el ruido.
        if (guidedActive && sample.rssi != null) {
            lastReliableHeadingDeg?.let { heading ->
                guidedTurnSamples.add(AzimuthRssi(heading, sample.rssi.toDouble()))
                sectorCounts = BearingEstimator.sectorCounts(guidedTurnSamples).toList()
                BearingEstimator.estimate(guidedTurnSamples, minPerSector)?.let { estimate ->
                    bearing = estimate
                    guidedActive = false
                }
            }
        }

        _uiState.value = current.copy(
            smoothedRssi = smoothed,
            trend = trend,
            distance = distance,
            rttMeters = rtt,
            sectorCounts = sectorCounts,
            bearing = bearing,
            guidedTurnActive = guidedActive,
        )
    }

    private fun onHeading(heading: HeadingSample) {
        if (heading.reliable) lastReliableHeadingDeg = heading.azimuthDeg.toDouble()
        _uiState.update { it.copy(headingDeg = heading.azimuthDeg, headingReliable = heading.reliable) }
    }

    /** "Vuelta guiada" (ver el encargo): empieza o reinicia la recolección de pares rumbo/señal suavizada. */
    fun startGuidedTurn() {
        guidedTurnSamples.clear()
        _uiState.update {
            it.copy(guidedTurnActive = true, bearing = null, sectorCounts = List(BearingEstimator.Sectors) { 0 })
        }
    }

    fun toggleSound() = _uiState.update { it.copy(soundEnabled = !it.soundEnabled) }

    fun toggleVibration() = _uiState.update { it.copy(vibrationEnabled = !it.vibrationEnabled) }
}
