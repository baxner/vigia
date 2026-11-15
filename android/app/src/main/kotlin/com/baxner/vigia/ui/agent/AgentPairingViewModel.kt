package com.baxner.vigia.ui.agent

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentApiException
import com.baxner.vigia.data.agent.AgentModeController
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.data.agent.DiscoveredAgent
import com.baxner.vigia.data.history.HistoryModule
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.NetworkInfoProvider
import java.io.IOException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** A quién se está a punto de vincular: lo que se eligió de la lista de descubiertos, o lo que se tecleó a mano. */
data class PairingTarget(
    val name: String,
    val host: String,
    val port: Int,
    val certSha256: String? = null,
)

/** Los dos pasos de la vinculación (ver el encargo, punto 5). */
sealed interface PairingStep {
    data class ChooseAgent(
        val discovered: List<DiscoveredAgent> = emptyList(),
        val searching: Boolean = true,
        val showManualEntry: Boolean = false,
        val manualHost: String = "",
        val manualPort: String = "7341",
        val checkingManual: Boolean = false,
        val errorMessage: String? = null,
    ) : PairingStep

    data class EnterCode(
        val target: PairingTarget,
        val code: String = "",
        val submitting: Boolean = false,
        val errorMessage: String? = null,
    ) : PairingStep

    data class Done(val agentName: String) : PairingStep
}

/**
 * Descubre agentes por mDNS (con opción de IP:puerto a mano) y, elegido uno,
 * pide el código de 6 dígitos y se vincula (ver [AgentModeController.pair] —
 * ahí vive la subida única de personalizaciones, esta clase solo junta los
 * datos que necesita: la red actual y el historial local).
 */
class AgentPairingViewModel(application: Application) : AndroidViewModel(application) {
    private val networkInfoProvider = NetworkInfoProvider(application)
    private val agentController: AgentModeController = AgentModule.controller(application)
    private val localHistory: HistoryRepository = HistoryModule.repository(application)

    private val _step = MutableStateFlow<PairingStep>(PairingStep.ChooseAgent())
    val step: StateFlow<PairingStep> = _step.asStateFlow()

    private var discoveryJob: Job? = null

    init {
        startDiscovery()
    }

    fun startDiscovery() {
        discoveryJob?.cancel()
        _step.value = PairingStep.ChooseAgent(searching = true)
        discoveryJob = viewModelScope.launch {
            launch {
                agentController.discoverAgents().collect { found ->
                    updateChooseAgent { it.copy(discovered = found) }
                }
            }
            // mDNS no avisa "ya terminé de buscar": tras un rato razonable sin
            // nada, se le dice al usuario que puede seguir esperando o meter
            // la IP a mano — sin bloquear que algo aparezca después.
            delay(SearchGraceMs)
            updateChooseAgent { it.copy(searching = false) }
        }
    }

    fun toggleManualEntry() = updateChooseAgent { it.copy(showManualEntry = !it.showManualEntry, errorMessage = null) }

    fun onManualHostChange(host: String) = updateChooseAgent { it.copy(manualHost = host.trim()) }

    fun onManualPortChange(port: String) = updateChooseAgent { it.copy(manualPort = port.filter(Char::isDigit).take(5)) }

    fun selectDiscovered(agent: DiscoveredAgent) {
        _step.value = PairingStep.EnterCode(target = PairingTarget(agent.name, agent.host, agent.port, agent.certSha256))
    }

    /** "Continuar" de IP:puerto a mano: valida que el agente conteste ([AgentModeController.previewAgent]) antes de pedir el código. */
    fun continueManual() {
        val current = _step.value as? PairingStep.ChooseAgent ?: return
        val host = current.manualHost
        val port = current.manualPort.toIntOrNull()
        if (host.isBlank() || port == null || port !in 1..65535) {
            updateChooseAgent { it.copy(errorMessage = getApplication<Application>().getString(R.string.pairing_error_no_connection)) }
            return
        }
        viewModelScope.launch {
            updateChooseAgent { it.copy(checkingManual = true, errorMessage = null) }
            val network = networkInfoProvider.current()
            agentController.previewAgent(host, port, network)
                .onSuccess { info -> _step.value = PairingStep.EnterCode(target = PairingTarget(info.name, host, port)) }
                .onFailure {
                    updateChooseAgent {
                        it.copy(checkingManual = false, errorMessage = getApplication<Application>().getString(R.string.pairing_error_no_connection))
                    }
                }
        }
    }

    fun onCodeChange(code: String) {
        val current = _step.value as? PairingStep.EnterCode ?: return
        _step.value = current.copy(code = code.filter(Char::isDigit).take(6), errorMessage = null)
    }

    fun submitCode() {
        val current = _step.value as? PairingStep.EnterCode ?: return
        if (current.code.length != 6 || current.submitting) return
        viewModelScope.launch {
            _step.value = current.copy(submitting = true, errorMessage = null)
            val network = networkInfoProvider.current()
            agentController.pair(
                host = current.target.host,
                port = current.target.port,
                code = current.code,
                network = network,
                localHistory = localHistory,
                localNetworkKey = localNetworkKeyFor(network),
                expectedCertSha256 = current.target.certSha256,
            ).onSuccess { link ->
                _step.value = PairingStep.Done(link.agentName)
            }.onFailure { error ->
                val app = getApplication<Application>()
                // AgentApiException primero: también es IOException (ver AgentApiClient),
                // así que si fuera al revés un 500 del agente caería mal en "sin conexión".
                val message = when {
                    error is AgentApiException && error.status == 401 -> app.getString(R.string.pairing_error_invalid_code)
                    error is AgentApiException -> app.getString(R.string.pairing_error_generic, error.message)
                    error is IOException -> app.getString(R.string.pairing_error_no_connection)
                    else -> app.getString(R.string.pairing_error_generic, error.message ?: error.toString())
                }
                _step.value = current.copy(submitting = false, errorMessage = message)
            }
        }
    }

    private fun localNetworkKeyFor(network: CurrentNetwork): NetworkKey? {
        val subnet = NetworkKey.subnetOf(network.cidr) ?: return null
        return NetworkKey.of(network.ssid, network.gateway, subnet)
    }

    private fun updateChooseAgent(transform: (PairingStep.ChooseAgent) -> PairingStep.ChooseAgent) {
        val current = _step.value as? PairingStep.ChooseAgent ?: return
        _step.value = transform(current)
    }

    override fun onCleared() {
        discoveryJob?.cancel()
    }

    private companion object {
        const val SearchGraceMs = 6_000L
    }
}
