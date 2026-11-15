package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.AgentApiException
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.OuterRing
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.network.CurrentNetwork
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Arma el [SecurityInput] que se le manda a [SecurityEngine.report]. Puro —
 * sin red ni Android — para poder probarlo con JUnit plano (ver el encargo,
 * "Pruebas": "armado del SecurityInput").
 */
object SecurityInputBuilder {
    fun build(
        devices: List<KnownDevice>,
        router: RouterInfo? = null,
        outerRing: OuterRing? = null,
        wifi: WifiInfo? = null,
        recentAlerts: List<AlertDto> = emptyList(),
        /** Ver [SecurityInput.acknowledged]: solo lo llena el modo sin agente. */
        acknowledged: List<AcknowledgedFinding> = emptyList(),
    ): SecurityInput = SecurityInput(
        devices = devices.map { it.toSecurityInputDevice() },
        router = router,
        outerRing = outerRing,
        wifi = wifi,
        recentAlerts = recentAlerts,
        acknowledged = acknowledged,
    )

    /** El nombre que ve el motor para redactar sus textos: el que puso el usuario o, si no, la mejor pista que ya tiene el equipo. */
    private fun KnownDevice.toSecurityInputDevice(): SecurityInputDevice {
        val name = custom.name
            ?: last.hostname.takeIf { it.isNotBlank() }
            ?: last.identity.brand.takeIf { it.isNotBlank() }
            ?: last.vendor.takeIf { it.isNotBlank() }
            ?: last.ip
        return SecurityInputDevice(id = id, name = name, online = online, device = last)
    }
}

/**
 * "Tu router visto desde aquí" (ver el encargo, punto 3, y el contrato §
 * "Seguridad" → "Anillos"): el teléfono está conectado a la Wi-Fi de la ONT
 * (el anillo exterior) cuando su subred actual contiene la `externalIp` del
 * último [RouterInfo] guardado. Pura: sin red ni Android.
 */
object RouterVisibility {
    /**
     * [phoneCidr] es la IP+prefijo del teléfono tal como la da
     * [com.baxner.vigia.data.network.CurrentNetwork.cidr] (p. ej.
     * "192.168.100.5/24"); [routerExternalIp] es [RouterInfo.externalIp].
     * `false` si falta cualquiera de los dos. Reutiliza
     * [NetworkKey.subnetOf] en vez de repetir aritmética de máscaras: dos
     * IP caen en la misma subred si, aplicado el mismo prefijo, su subred
     * da igual.
     */
    fun routerSeenFromHere(phoneCidr: String?, routerExternalIp: String?): Boolean {
        if (phoneCidr.isNullOrBlank() || routerExternalIp.isNullOrBlank()) return false
        val phoneSubnet = NetworkKey.subnetOf(phoneCidr) ?: return false
        val prefix = phoneSubnet.substringAfterLast('/')
        val externalSubnet = NetworkKey.subnetOf("$routerExternalIp/$prefix") ?: return false
        return externalSubnet == phoneSubnet
    }
}

/**
 * La clave de red local para guardar reconocimientos "por red" sin agente
 * (ver el contrato § "Hallazgos reconocidos v1.3": "la app sin agente los
 * guarda en el teléfono, por red"). Misma fórmula que
 * [com.baxner.vigia.ui.scan.ScanViewModel.keyFor]: SSID + gateway + subred,
 * para que un reconocimiento hecho en Seguridad valga para la misma red que
 * ve el resto de la app. `null` sin subred (sin red usable no hay nada que
 * guardar). Pura: sin red ni Android.
 */
fun localNetworkKeyFor(network: CurrentNetwork?): NetworkKey? {
    val subnet = NetworkKey.subnetOf(network?.cidr) ?: return null
    return NetworkKey.of(network?.ssid, network?.gateway, subnet)
}

/**
 * `404`/`501` de `GET /api/v1/security` (ver la revisión de la fase 5, punto
 * 3): el agente instalado todavía no trae esa parte de la fase 5 — no es que
 * esté inalcanzable. [resolveMonitoredSecurityReport] lo usa para decidir el
 * plan B, y `ui/security/SecurityViewModel.kt` para no marcar
 * `agentUnreachable` por esto.
 */
fun Throwable?.isSecurityEndpointMissing(): Boolean =
    (this as? AgentApiException)?.status in SecurityEndpointMissingStatuses

private val SecurityEndpointMissingStatuses = setOf(404, 501)

/**
 * La parte de [SecurityRepository.refresh] para cuando hay agente vigilando
 * la red actual, separada en una función libre para poder probarla sin un
 * [com.baxner.vigia.data.agent.AgentHistoryRepository] real — [agentSecurity]
 * es su `security()`, inyectado (ver la revisión de la fase 5, punto 9).
 *
 * Primero el informe del agente tal cual. Si falla porque el endpoint
 * todavía no existe ([isSecurityEndpointMissing]) y el motor está
 * disponible, plan B: el teléfono calcula el informe local con los mismos
 * equipos del agente, el router actual (sondeado por el motor) y la Wi-Fi —
 * ver la revisión de la fase 5, punto 3. Cualquier otro error (agente
 * inalcanzable de verdad) se propaga tal cual.
 */
suspend fun resolveMonitoredSecurityReport(
    agentSecurity: suspend () -> Result<SecurityReport>,
    engine: SecurityEngine,
    devices: List<KnownDevice>,
    network: CurrentNetwork?,
    wifi: WifiInfo?,
    recentAlerts: List<AlertDto>,
): Result<SecurityReport> {
    val agentResult = agentSecurity()
    val agentReport = agentResult.getOrNull()
    if (agentReport == null) {
        val error = agentResult.exceptionOrNull() ?: SecurityUnavailableException("el agente no mandó ningún informe")
        if (error.isSecurityEndpointMissing() && engine.isAvailable) {
            val router = withContext(Dispatchers.IO) { network?.gateway?.let(engine::routerInfo) }
            val input = SecurityInputBuilder.build(devices = devices, router = router, outerRing = null, wifi = wifi, recentAlerts = recentAlerts)
            val computed = withContext(Dispatchers.Default) { engine.report(input) }
            if (computed != null) return Result.success(computed)
        }
        return Result.failure(error)
    }
    if (!engine.isAvailable) return Result.success(agentReport)
    // Con motor disponible, informe completo: los datos del agente (router,
    // outerRing) más la Wi-Fi actual, que el agente nunca puede saber (ver
    // el contrato § "Seguridad" → "Un solo motor").
    val acknowledged = agentReport.findings
        .filter { it.acknowledged }
        .map { AcknowledgedFinding(findingId = it.id, at = it.acknowledgedAt.orEmpty()) }
    val input = SecurityInputBuilder.build(
        devices = devices,
        router = agentReport.router,
        outerRing = agentReport.outerRing,
        wifi = wifi,
        recentAlerts = recentAlerts,
        acknowledged = acknowledged,
    )
    val merged = withContext(Dispatchers.Default) { engine.report(input) }
    return Result.success(merged ?: agentReport)
}

/**
 * La parte de [SecurityRepository.refresh] para cuando no hay agente: el
 * input sale del último escaneo local, el router actual (sondeado por el
 * motor —`routerInfo` hace E/S de red de hasta 6 s, ver la revisión de la
 * fase 5, punto 4, por eso [Dispatchers.IO] y no [Dispatchers.Default]) y la
 * Wi-Fi. Sin motor disponible no hay informe que mostrar.
 */
suspend fun resolveLocalSecurityReport(
    engine: SecurityEngine,
    devices: List<KnownDevice>,
    network: CurrentNetwork?,
    wifi: WifiInfo?,
    recentAlerts: List<AlertDto>,
    /** Ver [SecurityInput.acknowledged]: lo guardado en el teléfono para la red de [network] (ver [localNetworkKeyFor]). */
    acknowledged: List<AcknowledgedFinding> = emptyList(),
): Result<SecurityReport> {
    if (!engine.isAvailable) {
        return Result.failure(SecurityUnavailableException("El motor de seguridad todavía no está disponible en esta build."))
    }
    val gateway = network?.gateway
    val router = withContext(Dispatchers.IO) { gateway?.let(engine::routerInfo) }
    val input = SecurityInputBuilder.build(devices = devices, router = router, outerRing = null, wifi = wifi, recentAlerts = recentAlerts, acknowledged = acknowledged)
    val computed = withContext(Dispatchers.Default) { engine.report(input) }
        ?: return Result.failure(SecurityUnavailableException("El motor de seguridad no pudo calcular un informe."))
    return Result.success(computed)
}
