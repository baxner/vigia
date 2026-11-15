package com.baxner.vigia.ui.alerts

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.AlertSettingsDto
import com.baxner.vigia.data.agent.AlertType
import com.baxner.vigia.data.agent.AlertsModule
import com.baxner.vigia.data.agent.AlertsRepository
import com.baxner.vigia.data.agent.with
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

data class AlertsUiState(
    /** Sin agente vinculado no hay de dónde sacar alertas (ver el encargo, punto 5: estado vacío que lo explica). */
    val linked: Boolean = false,
    val alerts: List<AlertDto> = emptyList(),
    val unreadCount: Int = 0,
    val settings: AlertSettingsDto = AlertSettingsDto(),
)

/**
 * Envoltura liviana sobre [AlertsRepository] (el singleton de proceso que de
 * verdad guarda el estado): varias instancias de este ViewModel — una por
 * pantalla que lo pida, ver `AlertsScreen.kt`/`PanelScreen.kt` — leen todas
 * la misma fuente, así que no hace falta compartir una sola instancia como
 * sí hace [com.baxner.vigia.ui.scan.ScanViewModel].
 */
class AlertsViewModel(application: Application) : AndroidViewModel(application) {
    private val repository: AlertsRepository = AlertsModule.repository(application)
    private val agentController = AgentModule.controller(application)

    val uiState: StateFlow<AlertsUiState> = combine(
        repository.alerts,
        repository.unreadCount,
        repository.settings,
    ) { alerts, unread, settings ->
        AlertsUiState(linked = agentController.isLinked(), alerts = alerts, unreadCount = unread, settings = settings)
    }.stateIn(
        viewModelScope,
        SharingStarted.WhileSubscribed(5_000),
        AlertsUiState(linked = agentController.isLinked(), alerts = repository.alerts.value, unreadCount = repository.unreadCount.value, settings = repository.settings.value),
    )

    /** Al entrar a la pantalla Alertas (ver el encargo, punto 5): se pone al día y marca leído. */
    fun onOpen() {
        viewModelScope.launch {
            repository.refreshFromNetwork()
            repository.markAllRead()
            repository.loadSettingsFromNetwork()
        }
    }

    fun setTypeEnabled(type: AlertType, enabled: Boolean) {
        viewModelScope.launch {
            repository.updateSettings(repository.settings.value.with(type, enabled))
        }
    }
}
