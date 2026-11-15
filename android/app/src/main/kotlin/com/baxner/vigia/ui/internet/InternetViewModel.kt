package com.baxner.vigia.ui.internet

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.data.agent.AgentApiException
import com.baxner.vigia.data.agent.AgentHistoryRepository
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.agent.InternetSeriesPointDto
import com.baxner.vigia.data.agent.InternetSeriesRange
import com.baxner.vigia.data.agent.InternetSettingsDto
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.stateOrUnknown
import com.baxner.vigia.data.widget.WidgetSummaryUpdater
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** "Probar ahora" (ver el encargo, "Velocidad"): el 409 del contrato aparte de cualquier otro fallo, para el mensaje exacto que pide el encargo. */
sealed interface SpeedTestMessage {
    data object AlreadyRunning : SpeedTestMessage
    data class Failed(val detail: String) : SpeedTestMessage
}

data class InternetUiState(
    /** Hay un vínculo guardado (aunque esta red no sea la que vigila, ver [monitoring]). Sin esto, el estado vacío explica que hace falta el agente (ver el encargo, punto 2). */
    val linked: Boolean = false,
    /** La red actual la vigila el agente ("Red monitoreada"): sin esto no hay de dónde medir internet (ver el encargo, punto 2: "solo con agente"). */
    val monitoring: Boolean = false,
    val loading: Boolean = false,
    val status: InternetStatusDto? = null,
    val series: List<InternetSeriesPointDto> = emptyList(),
    val seriesRange: InternetSeriesRange = InternetSeriesRange.HOURS_24,
    val settings: InternetSettingsDto = InternetSettingsDto(),
    val speedTestRunning: Boolean = false,
    val speedTestMessage: SpeedTestMessage? = null,
)

/** Solo lo transitorio; [InternetUiState.monitoring] sale de [com.baxner.vigia.data.agent.AgentModeController.panelState] (ver abajo), no de aquí. */
private data class TransientState(
    val loading: Boolean = false,
    val status: InternetStatusDto? = null,
    val series: List<InternetSeriesPointDto> = emptyList(),
    val seriesRange: InternetSeriesRange = InternetSeriesRange.HOURS_24,
    val settings: InternetSettingsDto = InternetSettingsDto(),
    val speedTestRunning: Boolean = false,
    val speedTestMessage: SpeedTestMessage? = null,
)

/**
 * Pantalla Internet (ver el encargo, "Qué construir", punto 2): todo viene
 * del agente — [com.baxner.vigia.data.agent.AgentModeController.activeRepository],
 * el mismo singleton que ya usan Seguridad y Dispositivos — así que sin vínculo o sin que el
 * agente vigile esta red no hay nada que pedir (ver [InternetUiState.monitoring]
 * y el estado vacío de `InternetScreen.kt`).
 */
class InternetViewModel(application: Application) : AndroidViewModel(application) {
    private val agentController = AgentModule.controller(application)

    private val _transient = MutableStateFlow(TransientState())

    val uiState: StateFlow<InternetUiState> = combine(agentController.panelState, _transient) { panel, transient ->
        InternetUiState(
            linked = panel is AgentPanelState.Linked,
            monitoring = (panel as? AgentPanelState.Linked)?.let { it.monitoringCurrentNetwork || it.viewingRemote } == true,
            loading = transient.loading,
            status = transient.status,
            series = transient.series,
            seriesRange = transient.seriesRange,
            settings = transient.settings,
            speedTestRunning = transient.speedTestRunning,
            speedTestMessage = transient.speedTestMessage,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), InternetUiState())

    private fun repository(): AgentHistoryRepository? = agentController.activeRepository.value

    /** Al abrir la pantalla (ver el encargo): estado, serie del rango vigente y ajustes, los tres de una vez. */
    fun onOpen() {
        refreshStatus()
        loadSeries(_transient.value.seriesRange)
        loadSettings()
    }

    fun refreshStatus() {
        val repo = repository() ?: return
        viewModelScope.launch {
            _transient.update { it.copy(loading = true) }
            repo.internet()
                .onSuccess { status ->
                    _transient.update { it.copy(loading = false, status = status) }
                    // Widget de pantalla de inicio (fase 8, ver `data/widget/WidgetSummaryUpdater.kt`):
                    // solo llega hasta aquí si [repository] devolvió algo, es
                    // decir, si esta red sí la vigila el agente — mismo
                    // criterio que la fila "Internet" de `AgentCard.kt`.
                    WidgetSummaryUpdater.pushInternet(getApplication<android.app.Application>(), status.stateOrUnknown(), status.now.internetMs?.let(Math::round)?.toInt())
                }
                .onFailure { _transient.update { it.copy(loading = false) } }
        }
    }

    /** Selector 24 h / 7 días (ver el encargo, "Gráfica"). */
    fun setSeriesRange(range: InternetSeriesRange) {
        _transient.update { it.copy(seriesRange = range) }
        loadSeries(range)
    }

    private fun loadSeries(range: InternetSeriesRange) {
        val repo = repository() ?: return
        viewModelScope.launch {
            repo.internetSeries(range)
                .onSuccess { points -> _transient.update { if (it.seriesRange == range) it.copy(series = points) else it } }
        }
    }

    private fun loadSettings() {
        val repo = repository() ?: return
        viewModelScope.launch {
            repo.internetSettings().onSuccess { settings -> _transient.update { it.copy(settings = settings) } }
        }
    }

    /** Ajuste "pruebas al día" (ver el encargo, "Velocidad": "ajuste de pruebas al día (0–6)"). */
    fun setSpeedTestsPerDay(perDay: Int) {
        val repo = repository() ?: return
        viewModelScope.launch {
            repo.putInternetSettings(InternetSettingsDto(speedTestsPerDay = perDay.coerceIn(0, 6)))
                .onSuccess { settings -> _transient.update { it.copy(settings = settings) } }
        }
    }

    /**
     * "Probar ahora" (ver el encargo): muestra "Midiendo…" y sondea
     * [AgentHistoryRepository.internet] hasta que aparece una prueba más
     * nueva que la que ya se conocía, o se agota [MaxPolls] (la prueba real
     * tarda bastante: ~25 MB de bajada más ~10 MB de subida, ver el
     * contrato). Un `409` (ya hay una corriendo) se muestra tal cual pide el
     * encargo, sin sondear.
     */
    fun runSpeedTestNow() {
        val repo = repository() ?: return
        val previousAt = _transient.value.status?.speedTests?.firstOrNull()?.at
        viewModelScope.launch {
            _transient.update { it.copy(speedTestRunning = true, speedTestMessage = null) }
            repo.startSpeedTest().onFailure { error ->
                val message = if ((error as? AgentApiException)?.status == 409) {
                    SpeedTestMessage.AlreadyRunning
                } else {
                    SpeedTestMessage.Failed(error.message ?: "")
                }
                _transient.update { it.copy(speedTestRunning = false, speedTestMessage = message) }
                return@launch
            }
            pollForSpeedTestResult(repo, previousAt)
        }
    }

    private suspend fun pollForSpeedTestResult(repo: AgentHistoryRepository, previousAt: String?) {
        repeat(MaxPolls) {
            delay(PollIntervalMs)
            val status = repo.internet().getOrNull()
            val latestAt = status?.speedTests?.firstOrNull()?.at
            if (status != null && latestAt != null && latestAt != previousAt) {
                _transient.update { it.copy(speedTestRunning = false, status = status) }
                return
            }
        }
        // Se agotó el sondeo: no se sabe si terminó o sigue — refrescar una
        // última vez y dejar de mostrar "Midiendo…" (mejor que quedarse
        // pegado ahí para siempre).
        refreshStatus()
        _transient.update { it.copy(speedTestRunning = false) }
    }

    private companion object {
        const val MaxPolls = 40
        const val PollIntervalMs = 3_000L
    }
}
