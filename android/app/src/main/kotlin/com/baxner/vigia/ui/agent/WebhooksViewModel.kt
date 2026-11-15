package com.baxner.vigia.ui.agent

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.WebhookDto
import com.baxner.vigia.data.agent.WebhooksRepository
import com.baxner.vigia.data.agent.isValidWebhookUrl
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Resultado de "Probar" (ver el encargo: "mostrando el status o el error") para un webhook puntual, por [WebhookDto.id]. */
sealed interface WebhookTestOutcome {
    data class Succeeded(val status: Int) : WebhookTestOutcome
    data class Failed(val message: String) : WebhookTestOutcome
}

data class WebhooksUiState(
    /** Sin agente vinculado no hay a quién pedirle webhooks (mismo criterio que [com.baxner.vigia.ui.alerts.AlertsUiState.linked]). */
    val linked: Boolean = false,
    val loading: Boolean = false,
    val webhooks: List<WebhookDto> = emptyList(),
    /** Error al cargar la lista (p. ej. ninguna dirección del agente contestó). `null` = sin error. */
    val loadError: String? = null,
    val testResults: Map<Long, WebhookTestOutcome> = emptyMap(),
    /** Id en curso de borrar/probar (para deshabilitar su botón mientras responde), o -1 sin ninguno. */
    val pendingActionId: Long = NoPendingAction,
) {
    companion object {
        const val NoPendingAction = -1L
    }
}

/** Formulario de "Agregar webhook" (ver el encargo, punto 2): separado del resto del estado porque vive solo mientras el diálogo está abierto. */
data class AddWebhookFormState(
    val url: String = "",
    val secret: String = "",
    val selectedTypes: Set<String> = emptySet(),
    val submitting: Boolean = false,
    /** Error de validación local (URL inválida) o el que mandó el agente al guardar. */
    val error: String? = null,
) {
    /** El contrato dice "`types`? vacío = todos los tipos" (ver `docs/agente-api.md`): no mandar la lista si no se marcó ninguno. */
    fun typesForRequest(): List<String> = selectedTypes.toList()
}

/**
 * Pantalla "Webhooks" de Ajustes del agente (ver el encargo de la fase 8,
 * "Qué construir", punto 2): listar, agregar, borrar y "Probar". Todo pasa
 * por [WebhooksRepository] (sin caché propia — a diferencia de
 * [com.baxner.vigia.ui.alerts.AlertsViewModel], esta pantalla no la necesita
 * fuera de aquí), así que cada apertura vuelve a pedir la lista.
 */
class WebhooksViewModel(application: Application) : AndroidViewModel(application) {
    private val agentController = AgentModule.controller(application)
    private val repository = WebhooksRepository(application)

    private val _uiState = MutableStateFlow(WebhooksUiState(linked = agentController.isLinked()))
    val uiState: StateFlow<WebhooksUiState> = _uiState.asStateFlow()

    private val _form = MutableStateFlow(AddWebhookFormState())
    val form: StateFlow<AddWebhookFormState> = _form.asStateFlow()

    /** Al abrir la pantalla: trae la lista si hay agente vinculado (ver el encargo, estado "sin vincular"). */
    fun onOpen() {
        val linked = agentController.isLinked()
        _uiState.update { it.copy(linked = linked) }
        if (linked) refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, loadError = null) }
            repository.list()
                .onSuccess { webhooks -> _uiState.update { it.copy(loading = false, webhooks = webhooks, loadError = null) } }
                .onFailure { error -> _uiState.update { it.copy(loading = false, loadError = error.message) } }
        }
    }

    fun resetForm() {
        _form.value = AddWebhookFormState()
    }

    fun setFormUrl(url: String) {
        _form.update { it.copy(url = url, error = null) }
    }

    fun setFormSecret(secret: String) {
        _form.update { it.copy(secret = secret) }
    }

    fun toggleFormType(type: String, enabled: Boolean) {
        _form.update { state ->
            val updated = if (enabled) state.selectedTypes + type else state.selectedTypes - type
            state.copy(selectedTypes = updated)
        }
    }

    /**
     * "Agregar" (ver el encargo: "URL http/https validada"): valida antes de
     * llamar a la red con [isValidWebhookUrl]; [onAdded] solo se llama si el
     * agente lo guardó, para que la pantalla cierre el formulario.
     */
    fun submitForm(onAdded: () -> Unit) {
        val current = _form.value
        if (!isValidWebhookUrl(current.url)) {
            _form.update { it.copy(error = InvalidUrlMarker) }
            return
        }
        _form.update { it.copy(submitting = true, error = null) }
        viewModelScope.launch {
            repository.add(current.url.trim(), current.typesForRequest(), current.secret.trim().ifBlank { null })
                .onSuccess {
                    _form.value = AddWebhookFormState()
                    refresh()
                    onAdded()
                }
                .onFailure { error -> _form.update { it.copy(submitting = false, error = error.message ?: GenericErrorMarker) } }
        }
    }

    fun delete(id: Long) {
        viewModelScope.launch {
            _uiState.update { it.copy(pendingActionId = id) }
            repository.delete(id)
                .onSuccess {
                    _uiState.update { state ->
                        state.copy(
                            pendingActionId = WebhooksUiState.NoPendingAction,
                            webhooks = state.webhooks.filterNot { it.id == id },
                            testResults = state.testResults - id,
                        )
                    }
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(pendingActionId = WebhooksUiState.NoPendingAction, loadError = error.message)
                    }
                }
        }
    }

    /** "Probar" (ver el encargo): el resultado se guarda por id, no reemplaza la lista — así se ve junto a su fila. */
    fun test(id: Long) {
        viewModelScope.launch {
            _uiState.update { it.copy(pendingActionId = id) }
            repository.test(id)
                .onSuccess { result ->
                    val outcome = result.error?.let { WebhookTestOutcome.Failed(it) }
                        ?: WebhookTestOutcome.Succeeded(result.status ?: 0)
                    _uiState.update {
                        it.copy(pendingActionId = WebhooksUiState.NoPendingAction, testResults = it.testResults + (id to outcome))
                    }
                    // El estado real (lastStatus/lastError) lo guarda el agente: se refleja al refrescar.
                    refresh()
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(
                            pendingActionId = WebhooksUiState.NoPendingAction,
                            testResults = it.testResults + (id to WebhookTestOutcome.Failed(error.message ?: GenericErrorMarker)),
                        )
                    }
                }
        }
    }

    companion object {
        /** Marcador para que la UI muestre el string de "URL inválida" (ver `WebhooksScreen.kt`) en vez de un mensaje de red crudo. */
        const val InvalidUrlMarker = "invalid_url"
        const val GenericErrorMarker = "error"
    }
}
