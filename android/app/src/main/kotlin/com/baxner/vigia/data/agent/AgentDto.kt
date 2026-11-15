package com.baxner.vigia.data.agent

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceEvent
import com.baxner.vigia.data.history.DeviceEventType
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import java.time.Instant
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * Formas JSON del contrato `docs/agente-api.md`. Van aparte de los tipos de
 * dominio de `data/history/` (igual que [com.baxner.vigia.data.model.ScanResult]
 * va aparte de [KnownDevice]) por dos motivos: [KnownDevice]/[DeviceEvent]
 * usan [Instant] (el agente manda texto RFC 3339) y no son `@Serializable` —
 * y porque así no hace falta tocar `HistoryModels.kt` para nada de esto.
 * `last` sí se reutiliza directo: el contrato dice que es "el `core.Device`
 * del motor tal cual", el mismo tipo que ya decodifica [ScanRepository][com.baxner.vigia.data.repository.ScanRepository].
 */

@Serializable
data class AgentInfoDto(
    val id: String,
    val name: String,
    val version: String = "",
    val scanIntervalSec: Int = 0,
    val lastScanAt: String? = null,
    /** Aviso de salud del agente (p. ej. sin permiso de Red local en el servidor); null si todo bien. */
    val warning: String? = null,
    /**
     * IPv4 de las interfaces activas del agente (LAN, Tailscale…), para
     * alcanzarlo fuera de casa (ver el contrato, "Alertas v1.1"). Default
     * vacío: un agente v1.0 (de antes de este campo) no lo manda.
     */
    val addresses: List<String> = emptyList(),
    /**
     * Huella SHA-256 (hex) del certificado TLS del agente (ver TLS v1.5,
     * `docs/agente-api.md`). Default vacío: solo lo usa la migración de un
     * vínculo de antes de esta fase (ver `migrateLegacyLinkIfNeeded` en
     * `AgentTls.kt`), que ya trata "" igual que ausente.
     */
    val certSha256: String = "",
)

@Serializable
data class AgentNetworkDto(
    val key: String,
    val subnet: String,
    val gateway: String,
    val gatewayMac: String? = null,
    val firstSeen: String? = null,
    val lastSeen: String? = null,
    val lastScanAt: String? = null,
    /**
     * v1.2 (ver el contrato § "Seguridad" → "Anillos"): `"inner"` la red del
     * router de casa, `"outer"` la de delante del router. Default `"inner"`:
     * un agente v1.1 (de antes de este campo) no lo manda, y esa red siempre
     * fue la interior.
     */
    val role: String = "inner",
)

/** Espejo serializable de [DeviceCustomization] (ver el porqué arriba). */
@Serializable
data class DeviceCustomizationDto(
    val name: String? = null,
    val typeId: String? = null,
    val notes: String? = null,
    val location: String? = null,
    val important: Boolean = false,
)

fun DeviceCustomizationDto.toDomain(): DeviceCustomization =
    DeviceCustomization(name = name, typeId = typeId, notes = notes, location = location, important = important)

@Serializable
data class KnownDeviceDto(
    val id: Long,
    val network: String,
    val mac: String? = null,
    val lastIp: String,
    val firstSeen: String,
    val lastSeen: String,
    val online: Boolean,
    val onlineSince: String? = null,
    val last: Device,
    val custom: DeviceCustomizationDto = DeviceCustomizationDto(),
)

fun KnownDeviceDto.toDomain(): KnownDevice = KnownDevice(
    id = id,
    network = NetworkKey(network),
    mac = mac,
    lastIp = lastIp,
    firstSeen = Instant.parse(firstSeen),
    lastSeen = Instant.parse(lastSeen),
    online = online,
    onlineSince = onlineSince?.let(Instant::parse),
    last = last,
    custom = custom.toDomain(),
)

@Serializable
data class DeviceEventDto(
    val deviceId: Long,
    /** Texto, no el enum: así no hace falta marcar [DeviceEventType] `@Serializable`. */
    val type: String,
    val at: String,
    val detail: String? = null,
)

fun DeviceEventDto.toDomain(): DeviceEvent = DeviceEvent(
    deviceId = deviceId,
    type = DeviceEventType.valueOf(type),
    at = Instant.parse(at),
    detail = detail,
)

@Serializable
data class PairRequest(val code: String, val client: String)

@Serializable
data class PairResponseDto(
    val token: String,
    val agentId: String,
    val agentName: String,
    /** Huella SHA-256 (hex) del certificado TLS del agente (ver TLS v1.5): se fija en el vínculo de entrada, sin necesitar un segundo viaje a /info. */
    val certSha256: String = "",
)

@Serializable
data class ApiErrorBody(val error: String)

/**
 * Cuerpo opcional de `PUT /api/v1/security/acknowledged/{findingId}` (ver el
 * contrato § "Hallazgos reconocidos v1.3"). Sin pantalla para escribir una
 * nota todavía (el encargo no la pide); el campo va listo para cuando haga
 * falta, el agente ya lo acepta.
 */
@Serializable
data class AcknowledgeFindingRequest(val note: String? = null)

@Serializable
data class ScanStartedDto(val started: Boolean)

// ---- Alertas (v1.1, ver el contrato § "Alertas") ----

/**
 * Los tipos de alerta del contrato: los 5 de v1.1 más los 4 de Seguridad
 * (v1.2, ver el contrato § "Seguridad" → "Alertas nuevas") y los 2 de
 * Internet (v1.4, ver el contrato § "Internet" → "Caídas"). Texto en
 * [AlertDto.type] (no este enum directo), igual criterio que
 * [DeviceEventDto.type]: un tipo nuevo de una versión futura del agente no
 * debe romper el parseo, solo quedar sin reconocer (ver [fromRaw] y, para
 * notificaciones, [AlertNotificationPlanner]).
 */
enum class AlertType {
    NEW_DEVICE, IMPORTANT_OFFLINE, IMPORTANT_ONLINE, AGENT_BLIND, AGENT_RECOVERED,
    ROUTER_CHANGED, ARP_CONFLICT, PORT_MAPPING_ADDED, RISKY_SERVICE,
    INTERNET_DOWN, INTERNET_RESTORED;

    companion object {
        fun fromRaw(raw: String): AlertType? = runCatching { valueOf(raw) }.getOrNull()
    }
}

/** `high`/`medium`/`low` de [AlertDto.severity] (ver el contrato); cualquier otro texto cae en [UNKNOWN] en vez de reventar. */
enum class AlertSeverity { HIGH, MEDIUM, LOW, UNKNOWN }

fun AlertDto.severityOrUnknown(): AlertSeverity = when (severity.lowercase()) {
    "high" -> AlertSeverity.HIGH
    "medium" -> AlertSeverity.MEDIUM
    "low" -> AlertSeverity.LOW
    else -> AlertSeverity.UNKNOWN
}

/**
 * Una alerta del agente (ver el contrato § "Alertas v1.1" → "Formas JSON").
 * `title`/`body` ya vienen redactados en español por el agente: la app los
 * muestra tal cual, sin recomponerlos. `deviceId`/`network` nulos en las de
 * tipo `AGENT_*` (no hay un equipo puntual involucrado).
 */
@Serializable
data class AlertDto(
    val id: Long,
    val type: String,
    val severity: String,
    val at: String,
    val title: String,
    val body: String,
    val deviceId: Long? = null,
    val network: String? = null,
    val read: Boolean = false,
)

/**
 * Los interruptores de `/alert-settings` (ver el contrato): los 5 de v1.1
 * más los 4 de Seguridad (v1.2) y los 2 de Internet (v1.4). Campos con
 * nombre (no un `Map<String, Boolean>`) porque el contrato define tipos
 * cerrados — ver [AlertType]; si el agente agrega un tipo más algún día, hay
 * que agregar el campo acá también (y en [isEnabled]/[with]), el mismo
 * trato que ya tiene cualquier endpoint de este cliente.
 */
@Serializable
data class AlertSettingsDto(
    @SerialName("NEW_DEVICE") val newDevice: Boolean = true,
    @SerialName("IMPORTANT_OFFLINE") val importantOffline: Boolean = true,
    @SerialName("IMPORTANT_ONLINE") val importantOnline: Boolean = true,
    @SerialName("AGENT_BLIND") val agentBlind: Boolean = true,
    @SerialName("AGENT_RECOVERED") val agentRecovered: Boolean = true,
    @SerialName("ROUTER_CHANGED") val routerChanged: Boolean = true,
    @SerialName("ARP_CONFLICT") val arpConflict: Boolean = true,
    @SerialName("PORT_MAPPING_ADDED") val portMappingAdded: Boolean = true,
    @SerialName("RISKY_SERVICE") val riskyService: Boolean = true,
    @SerialName("INTERNET_DOWN") val internetDown: Boolean = true,
    @SerialName("INTERNET_RESTORED") val internetRestored: Boolean = true,
)

fun AlertSettingsDto.isEnabled(type: AlertType): Boolean = when (type) {
    AlertType.NEW_DEVICE -> newDevice
    AlertType.IMPORTANT_OFFLINE -> importantOffline
    AlertType.IMPORTANT_ONLINE -> importantOnline
    AlertType.AGENT_BLIND -> agentBlind
    AlertType.AGENT_RECOVERED -> agentRecovered
    AlertType.ROUTER_CHANGED -> routerChanged
    AlertType.ARP_CONFLICT -> arpConflict
    AlertType.PORT_MAPPING_ADDED -> portMappingAdded
    AlertType.RISKY_SERVICE -> riskyService
    AlertType.INTERNET_DOWN -> internetDown
    AlertType.INTERNET_RESTORED -> internetRestored
}

/** Copia de [AlertSettingsDto] con el interruptor de [type] en [enabled] (para la pantalla de ajustes, ver `ui/alerts/`). */
fun AlertSettingsDto.with(type: AlertType, enabled: Boolean): AlertSettingsDto = when (type) {
    AlertType.NEW_DEVICE -> copy(newDevice = enabled)
    AlertType.IMPORTANT_OFFLINE -> copy(importantOffline = enabled)
    AlertType.IMPORTANT_ONLINE -> copy(importantOnline = enabled)
    AlertType.AGENT_BLIND -> copy(agentBlind = enabled)
    AlertType.AGENT_RECOVERED -> copy(agentRecovered = enabled)
    AlertType.ROUTER_CHANGED -> copy(routerChanged = enabled)
    AlertType.ARP_CONFLICT -> copy(arpConflict = enabled)
    AlertType.PORT_MAPPING_ADDED -> copy(portMappingAdded = enabled)
    AlertType.RISKY_SERVICE -> copy(riskyService = enabled)
    AlertType.INTERNET_DOWN -> copy(internetDown = enabled)
    AlertType.INTERNET_RESTORED -> copy(internetRestored = enabled)
}

/**
 * `POST /api/v1/alerts/read` (ver el contrato): marca leídas todas las
 * alertas con id <= [upTo].
 */
@Serializable
data class AlertsReadRequest(val upTo: Long)

/**
 * Un webhook configurado en el agente (ver el contrato § "Webhooks"):
 * administrado desde Ajustes del agente → "Webhooks" (ver `ui/agent/`), para
 * conectar Vigía con Home Assistant u otros sistemas sin pasar por la nube.
 */
@Serializable
data class WebhookDto(
    val id: Long,
    val url: String,
    val types: List<String> = emptyList(),
    val createdAt: String? = null,
    val lastStatus: Int? = null,
    val lastError: String? = null,
)

/**
 * Cuerpo de `POST /api/v1/webhooks` (ver el contrato): [secret] firma cada
 * envío con `X-Vigia-Signature` (HMAC-SHA256); opcional, y el agente nunca
 * lo devuelve de vuelta (ver el contrato: "sin `secret` en la respuesta").
 * [types] vacío = todos los tipos de alerta.
 */
@Serializable
data class AddWebhookRequest(
    val url: String,
    val types: List<String> = emptyList(),
    val secret: String? = null,
)

/**
 * `POST /api/v1/webhooks/{id}/test` (ver el contrato): el agente manda una
 * alerta de prueba y contesta con el resultado tal cual, nunca como error
 * HTTP — para que "Probar" en la UI pueda mostrar "Error: …" sin que
 * [AgentApiClient] lo trate como un fallo de la llamada en sí.
 */
@Serializable
data class WebhookTestResultDto(
    val status: Int? = null,
    val error: String? = null,
)

/**
 * Un campo de [DeviceCustomizationPatch]: ausente (sin cambio) o presente
 * (con un valor, o `null` para borrar). Ver "Reglas del PATCH" en el
 * contrato: "campos ausentes = sin cambio; `null` = borrar".
 */
sealed interface PatchField<out T> {
    data object Absent : PatchField<Nothing>
    data class Present<T>(val value: T?) : PatchField<T>
}

private fun <T> T?.present(): PatchField<T> = PatchField.Present(this)

/** Azúcar: `null` de origen (campo sin valor local) se manda como "sin cambio", no como borrado. */
private fun <T> T?.presentIfSet(): PatchField<T> = if (this == null) PatchField.Absent else PatchField.Present(this)

/**
 * Cuerpo de `PATCH /api/v1/devices/{id}` (ver contrato). A diferencia de
 * [DeviceCustomization] (que siempre tiene los 5 campos, cada uno con su
 * valor o `null`), esto distingue "ausente" de "presente con `null`" — la
 * única forma de expresar en JSON "no toques este campo" vs. "bórralo".
 */
data class DeviceCustomizationPatch(
    val name: PatchField<String> = PatchField.Absent,
    val typeId: PatchField<String> = PatchField.Absent,
    val notes: PatchField<String> = PatchField.Absent,
    val location: PatchField<String> = PatchField.Absent,
    val important: PatchField<Boolean> = PatchField.Absent,
) {
    companion object {
        /** Reemplazo completo (como hace [com.baxner.vigia.data.history.RoomHistoryRepository]): los 5 campos presentes. */
        fun replaceAll(custom: DeviceCustomization): DeviceCustomizationPatch = DeviceCustomizationPatch(
            name = custom.name.present(),
            typeId = custom.typeId.present(),
            notes = custom.notes.present(),
            location = custom.location.present(),
            important = custom.important.present(),
        )

        /**
         * Solo los campos que [custom] trae puestos (para la subida única al
         * vincular, ver [com.baxner.vigia.data.agent.AgentCustomizationMigrator]):
         * nunca borra nada del lado del agente que acá no se conoce.
         */
        fun onlySet(custom: DeviceCustomization): DeviceCustomizationPatch = DeviceCustomizationPatch(
            name = custom.name.presentIfSet(),
            typeId = custom.typeId.presentIfSet(),
            notes = custom.notes.presentIfSet(),
            location = custom.location.presentIfSet(),
            important = if (custom.important) true.present() else PatchField.Absent,
        )
    }
}

/** El JSON que de verdad se manda: solo los campos [PatchField.Present], `null` incluido. */
fun DeviceCustomizationPatch.toJsonObject(): JsonObject = buildJsonObject {
    putPatchField("name", name)
    putPatchField("typeId", typeId)
    putPatchField("notes", notes)
    putPatchField("location", location)
    putPatchField("important", important)
}

private fun kotlinx.serialization.json.JsonObjectBuilder.putPatchField(key: String, field: PatchField<*>) {
    if (field !is PatchField.Present<*>) return // Absent: el campo ni aparece en el JSON.
    when (val value = field.value) {
        null -> put(key, JsonNull)
        is String -> put(key, value)
        is Boolean -> put(key, value)
        else -> error("DeviceCustomizationPatch: tipo no soportado para \"$key\": $value")
    }
}

// ---- Seguridad (v1.2, ver el contrato § "Seguridad") ----

/**
 * Una redirección de puertos del router (ver el contrato § "Seguridad" →
 * "Redirecciones de puertos"): la pidió un equipo por UPnP (`source =
 * "upnp"`, la lista el router) o la encontró la app escaneando la IP
 * exterior desde el anillo exterior (`source = "probe"`, solo en la app —
 * ver `ui/security/`, "Tu router visto desde aquí").
 */
@Serializable
data class PortMapping(
    val protocol: String,
    val externalPort: Int,
    val internalIp: String,
    val internalPort: Int,
    val description: String = "",
    val source: String = "upnp",
    val deviceId: Long? = null,
    val firstSeen: String? = null,
    val lastSeen: String? = null,
)

/** `source` de [PortMapping]: texto crudo en el campo (igual criterio que [AlertType]), este enum solo para íconos/UI. */
enum class PortMappingSource { UPNP, PROBE, UNKNOWN }

fun PortMapping.sourceOrUnknown(): PortMappingSource = when (source) {
    "upnp" -> PortMappingSource.UPNP
    "probe" -> PortMappingSource.PROBE
    else -> PortMappingSource.UNKNOWN
}

/**
 * El router de la red interior visto por el motor (UPnP/NAT-PMP, ver el
 * contrato § "Seguridad" → "Anillos"); `null` en [SecurityReport.router] si
 * no contesta ninguno de los dos protocolos.
 */
@Serializable
data class RouterInfo(
    val ip: String,
    val mac: String? = null,
    val upnp: Boolean = false,
    val natpmp: Boolean = false,
    val externalIp: String? = null,
    /** "outer_ring" | "internet" | "unknown" (ver [reachOrUnknown]). */
    val reach: String = "unknown",
    val portMappings: List<PortMapping> = emptyList(),
)

/** `reach` de [RouterInfo]: texto crudo en el campo, este enum solo para la UI (ver "Router" en `ui/security/`). */
enum class RouterReach { OUTER_RING, INTERNET, UNKNOWN }

fun RouterInfo.reachOrUnknown(): RouterReach = when (reach) {
    "outer_ring" -> RouterReach.OUTER_RING
    "internet" -> RouterReach.INTERNET
    else -> RouterReach.UNKNOWN
}

/**
 * La red de delante del router (ver el contrato § "Seguridad" → "Anillos").
 * [devices] trae la misma forma que `/networks/{key}/devices`, sin MAC (está
 * detrás del router) ni anuncios multicast.
 */
@Serializable
data class OuterRing(
    val network: String,
    val subnet: String,
    val gateway: String,
    val lastScanAt: String? = null,
    val devices: List<SecurityDevice> = emptyList(),
)

/**
 * Un equipo en la forma corta del motor (ver el contrato § "Seguridad",
 * `SecurityDevice`): la entrada de `Mobile.securityReport` y los equipos del
 * anillo exterior. [id] es null en la app sin agente; [name] ya viene
 * resuelto (personalizado, mDNS, hostname, marca…).
 */
@Serializable
data class SecurityDevice(
    val id: Long? = null,
    val name: String = "",
    val online: Boolean = true,
    val device: Device,
)

/**
 * Un punto que resta (o no) al [SecurityReport.score] (ver el contrato §
 * "Seguridad" → "Informe"): [title]/[detail]/[action] ya vienen redactados
 * en español por el motor, igual criterio que [AlertDto.title]/[AlertDto.body].
 * [id] es estable entre informes (p. ej. "RISKY_SERVICE|192.168.0.55|445"),
 * para no duplicar tarjetas al refrescar.
 */
@Serializable
data class Finding(
    val id: String,
    /**
     * `NO_INBOUND`, `UPNP_ENABLED`, `PORT_MAPPING`, `RISKY_SERVICE`,
     * `UNKNOWN_DEVICES`, `OUTER_RING_DEVICES`, `ROUTER_CHANGED`,
     * `ARP_CONFLICT`, o uno de Wi-Fi que solo agrega la app (`WIFI_OPEN`,
     * `WIFI_WEP`, `WIFI_WPA2`, `WIFI_WPS`). Texto crudo a propósito (como
     * [AlertDto.type]): el catálogo crece y un `kind` nuevo no debe romper
     * el parseo, solo quedar sin ícono especial en la UI.
     */
    val kind: String,
    /** "high" | "medium" | "low" | "info" (ver [severityOrInfo]). */
    val severity: String,
    val points: Int = 0,
    val title: String,
    val detail: String,
    val action: String = "",
    val deviceId: Long? = null,
    val ip: String? = null,
    val port: Int? = null,
    /**
     * "Ya lo sé, es a propósito" (v1.3, ver el contrato § "Hallazgos
     * reconocidos"): el usuario lo marcó como intencional. Un reconocido
     * sigue en [SecurityReport.findings] (al final) pero no resta del
     * `score` — [points] se conserva tal cual para poder mostrar "cuánto
     * restaría" si se deshace.
     */
    val acknowledged: Boolean = false,
    /** Cuándo se reconoció (RFC 3339); null si [acknowledged] es false. */
    val acknowledgedAt: String? = null,
)

/** `severity` de [Finding]: texto crudo en el campo, este enum solo para color/orden en la UI. */
enum class FindingSeverity { HIGH, MEDIUM, LOW, INFO, UNKNOWN }

fun Finding.severityOrInfo(): FindingSeverity = when (severity.lowercase()) {
    "high" -> FindingSeverity.HIGH
    "medium" -> FindingSeverity.MEDIUM
    "low" -> FindingSeverity.LOW
    "info" -> FindingSeverity.INFO
    else -> FindingSeverity.UNKNOWN
}

/**
 * `GET /api/v1/security` (ver el contrato § "Seguridad" → "Informe"):
 * calculado al pedirlo, con lo último de cada escaneo. [findings] viene de
 * mayor a menor severidad (el motor ya lo ordena; la UI no reordena).
 */
@Serializable
data class SecurityReport(
    val generatedAt: String = "",
    /** 100 menos la suma de `points` de [findings], nunca menos de 0. */
    val score: Int = 100,
    val findings: List<Finding> = emptyList(),
    /** `null` si el router no contesta UPnP ni NAT-PMP. */
    val router: RouterInfo? = null,
    /** `null` si no hay anillo exterior o aún no se escaneó. */
    val outerRing: OuterRing? = null,
)
