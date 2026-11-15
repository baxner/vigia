package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.OuterRing
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityDevice
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.model.Device
import kotlinx.serialization.Serializable

/**
 * `WifiInfo` del contrato (ver `docs/agente-api.md` § "Seguridad" → "Un solo
 * motor"): lo único que solo sabe la app, nunca el agente (corre por cable
 * en el servidor). `security` es texto crudo porque el motor lo espera así:
 * "open" | "wep" | "wpa" | "wpa2" | "wpa3" | "wpa2-wpa3" (ver
 * [com.baxner.vigia.data.network.WifiSecurity] para el enum de la UI; aquí
 * no se reutiliza ese enum para no atar este contrato a sus nombres).
 */
@Serializable
data class WifiInfo(
    val ssid: String = "",
    val security: String = "open",
    val wps: Boolean = false,
)

/** Un dispositivo dentro de [SecurityInput.devices] (ver el contrato): el motor solo necesita su [Device] y, para poder referenciarlo en un [com.baxner.vigia.data.agent.Finding], su id local. */
/** Ver [SecurityDevice]: la misma forma corta que usa el anillo exterior del informe. */
typealias SecurityInputDevice = SecurityDevice

/**
 * Un elemento de [SecurityInput.acknowledged] (ver el contrato § "Hallazgos
 * reconocidos v1.3"): "Ya lo sé, es a propósito" guardado en el teléfono
 * (modo local, sin agente — con agente el reconocimiento vive en el
 * servidor y el informe ya llega con `Finding.acknowledged` en true, ver
 * [com.baxner.vigia.data.agent.AgentApiClient.acknowledgeFinding]).
 */
@Serializable
data class AcknowledgedFinding(
    val findingId: String,
    /** RFC 3339, cuándo se reconoció. */
    val at: String,
    val note: String? = null,
)

/**
 * `input` de `Mobile.securityReport(input)` (ver el contrato § "Seguridad" →
 * "Un solo motor"): lo que la app junta para pedirle al motor un informe
 * completo — los equipos que conoce (del agente o de su propio escaneo
 * local), el router y el anillo exterior si los tiene, la Wi-Fi actual y las
 * alertas recientes (para que el motor sepa si `ROUTER_CHANGED`/
 * `ARP_CONFLICT` siguen vigentes, ver el contrato § "Seguridad" →
 * "Informe" → "Tipos de hallazgo"). [acknowledged] solo lo llena el modo sin
 * agente (ver [SecurityRepository.refresh]): con agente, el reconocimiento
 * ya se aplicó del lado del servidor.
 */
@Serializable
data class SecurityInput(
    val devices: List<SecurityInputDevice> = emptyList(),
    val router: RouterInfo? = null,
    val outerRing: OuterRing? = null,
    val wifi: WifiInfo? = null,
    val recentAlerts: List<AlertDto> = emptyList(),
    val acknowledged: List<AcknowledgedFinding> = emptyList(),
)

/** Una fila de `Mobile.portCatalog()` (ver el contrato): nombre y riesgo de un puerto, redactados por el motor. No la usa todavía la UI — [com.baxner.vigia.data.model.PortCatalog] se queda con su tabla fija hasta que el motor la traiga (ver el encargo, punto 1) —, pero el contrato ya la define y conviene tenerla lista. */
@Serializable
data class PortCatalogEntry(
    val port: Int,
    val name: String,
    val risk: String,
    val why: String,
    val action: String,
)

/**
 * El motor (`core`, vía gomobile) visto desde la app: calcula el informe de
 * seguridad y resuelve el router actual por UPnP/NAT-PMP (ver el contrato §
 * "Seguridad" → "Un solo motor"). HOY el `.aar` (`android/app/libs/vigia-core.aar`)
 * **todavía no tiene** `Mobile.securityReport`/`Mobile.routerInfo`/
 * `Mobile.portCatalog` — los está agregando otro subagente en Go en
 * paralelo —, así que esta interfaz es el punto de conexión que usará
 * [SecurityRepository] en vez de llamar a `Mobile` directo: cuando el motor
 * ya los tenga, el orquestador escribe una implementación que delega en
 * `Mobile.securityReport`/etc. (deserializando su JSON con
 * [com.baxner.vigia.data.repository.VigiaJson], igual que el resto del
 * cliente del agente) y la conecta en [SecurityModule]. Ningún código de
 * este encargo llama a una función de `Mobile` que no exista todavía: eso
 * rompería la compilación hoy mismo.
 *
 * [report]/[routerInfo] devuelven `null` (no lanzan) cuando el motor no
 * puede calcular algo — mismo criterio que ya usa el contrato
 * ("`Mobile.routerInfo`... `\"null\"` si no contesta") —, para que
 * [SecurityRepository] sepa cuándo caer a "mostrar el informe tal cual" o al
 * estado vacío, en vez de que una excepción se escape hasta la UI.
 */
interface SecurityEngine {
    /** `false` mientras el `.aar` no tenga las funciones de seguridad (ver [UnavailableSecurityEngine]); `true` en [FakeSecurityEngine] y en la implementación real futura. */
    val isAvailable: Boolean

    /** `Mobile.securityReport(input)` (ver el contrato). */
    fun report(input: SecurityInput): SecurityReport?

    /** `Mobile.routerInfo(gateway)` (ver el contrato): el router de la red actual, sondeado por UPnP/NAT-PMP. */
    fun routerInfo(gateway: String): RouterInfo?

    /** `Mobile.portCatalog()` (ver el contrato). Ver la nota de [PortCatalogEntry]: no la consume la UI todavía. */
    fun portCatalog(): List<PortCatalogEntry>
}

/**
 * Placeholder de producción mientras el `.aar` no tenga las funciones de
 * seguridad (ver la nota grande en [SecurityEngine]). [SecurityModule] lo da
 * por defecto: con esto, [SecurityRepository] sigue el camino "motor no
 * disponible" (mostrar el informe del agente tal cual, o el estado vacío sin
 * agente) en vez de fallar. El orquestador reemplaza este objeto por una
 * implementación real al integrar el motor nuevo.
 */
object UnavailableSecurityEngine : SecurityEngine {
    override val isAvailable: Boolean = false
    override fun report(input: SecurityInput): SecurityReport? = null
    override fun routerInfo(gateway: String): RouterInfo? = null
    override fun portCatalog(): List<PortCatalogEntry> = emptyList()
}
