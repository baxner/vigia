package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.FindingSeverity
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.agent.reachOrUnknown
import com.baxner.vigia.data.agent.severityOrInfo
import com.baxner.vigia.data.model.PortCatalog
import java.time.Instant

/**
 * Implementación falsa de [SecurityEngine], **solo para pruebas y
 * capturas** (ver el encargo, punto 1): calcula un puntaje y hallazgos
 * razonables a partir de [SecurityInput] con las mismas reglas en español
 * que describe el contrato, pero sin ser el motor real — no replica el
 * cálculo exacto de `core` en Go, solo lo suficiente para ejercitar la rama
 * "motor disponible" de [SecurityRepository] de forma determinista.
 *
 * NO se usa en producción (ver [UnavailableSecurityEngine], el default de
 * [SecurityModule]): mostrarle al usuario un puntaje inventado como si fuera
 * real sería peor que no mostrar nada.
 */
class FakeSecurityEngine(private val now: () -> Instant = Instant::now) : SecurityEngine {
    override val isAvailable: Boolean = true

    override fun report(input: SecurityInput): SecurityReport {
        val findings = mutableListOf<Finding>()

        input.wifi?.let { findings += wifiFindings(it) }
        findings += riskyServiceFindings(input)
        findings += portMappingFindings(input.router)
        findings += routerPostureFindings(input.router)

        val ordered = findings.sortedByDescending { it.severityOrInfo().rank() }
        val score = (100 - ordered.sumOf { it.points }).coerceIn(0, 100)
        return SecurityReport(
            generatedAt = now().toString(),
            score = score,
            findings = ordered,
            router = input.router,
            outerRing = input.outerRing,
        )
    }

    /** El fake no sondea red real (no hay UPnP/NAT-PMP en una prueba JVM): siempre "no disponible", igual que [UnavailableSecurityEngine]. */
    override fun routerInfo(gateway: String): RouterInfo? = null

    override fun portCatalog(): List<PortCatalogEntry> =
        PortCatalog.riskyPorts.map { risky ->
            PortCatalogEntry(port = risky.port, name = risky.serviceName, risk = "high", why = risky.reason, action = "Desactívalo si no lo usas.")
        }

    private fun wifiFindings(wifi: WifiInfo): List<Finding> = buildList {
        when (wifi.security.lowercase()) {
            "open" -> add(
                Finding(
                    id = "WIFI_OPEN", kind = "WIFI_OPEN", severity = "high", points = 20,
                    title = "Tu Wi-Fi no tiene contraseña",
                    detail = "Cualquiera puede conectarse a \"${wifi.ssid}\" sin contraseña.",
                    action = "Activa WPA2 o WPA3 en el router.",
                ),
            )
            "wep" -> add(
                Finding(
                    id = "WIFI_WEP", kind = "WIFI_WEP", severity = "high", points = 15,
                    title = "Tu Wi-Fi usa WEP",
                    detail = "WEP se puede romper en minutos con herramientas comunes.",
                    action = "Cambia la seguridad de tu Wi-Fi a WPA2 o WPA3 en el router.",
                ),
            )
            "wpa2" -> add(
                Finding(
                    id = "WIFI_WPA2", kind = "WIFI_WPA2", severity = "low", points = 3,
                    title = "Tu Wi-Fi usa WPA2",
                    detail = "WPA2 sigue siendo razonable, pero WPA3 es más resistente.",
                    action = "Si tu router lo soporta, cambia a WPA3.",
                ),
            )
            else -> Unit
        }
        if (wifi.wps) {
            add(
                Finding(
                    id = "WIFI_WPS", kind = "WIFI_WPS", severity = "medium", points = 5,
                    title = "WPS activado en tu Wi-Fi",
                    detail = "WPS permite entrar con un PIN de 8 dígitos, más fácil de forzar que la contraseña.",
                    action = "Apaga WPS en el router.",
                ),
            )
        }
    }

    private fun riskyServiceFindings(input: SecurityInput): List<Finding> =
        input.devices.flatMap { d ->
            PortCatalog.riskyAmong(d.device.openPorts).map { risky ->
                val label = d.name.ifBlank { d.device.ip }
                Finding(
                    id = "RISKY_SERVICE|${d.device.ip}|${risky.port}",
                    kind = "RISKY_SERVICE", severity = "medium", points = 8,
                    title = "Un equipo ofrece ${risky.serviceName} a toda la red",
                    detail = "$label (${d.device.ip}) tiene abierto ${risky.serviceName} (${risky.port}). ${risky.reason}.",
                    action = "Si no usas ${risky.serviceName} desde otros equipos, desactívalo en $label.",
                    deviceId = d.id, ip = d.device.ip, port = risky.port,
                )
            }
        }

    private fun portMappingFindings(router: RouterInfo?): List<Finding> =
        router?.portMappings.orEmpty().map { mapping ->
            val risky = PortCatalog.isRisky(mapping.internalPort)
            Finding(
                id = "PORT_MAPPING|${mapping.protocol}|${mapping.externalPort}",
                kind = "PORT_MAPPING", severity = if (risky) "high" else "medium", points = if (risky) 12 else 6,
                title = "Tu router redirige el puerto ${mapping.externalPort}",
                detail = "${mapping.protocol} ${mapping.externalPort} → ${mapping.internalIp}:${mapping.internalPort} (${PortCatalog.nameFor(mapping.internalPort)}).",
                action = "Si no la pediste a propósito, bórrala en el router.",
                deviceId = mapping.deviceId, ip = mapping.internalIp, port = mapping.internalPort,
            )
        }

    private fun routerPostureFindings(router: RouterInfo?): List<Finding> = buildList {
        if (router == null) return@buildList
        if (router.upnp) {
            add(
                Finding(
                    id = "UPNP_ENABLED", kind = "UPNP_ENABLED", severity = "info", points = 0,
                    title = "UPnP activo en tu router",
                    detail = "Cualquier equipo de tu red puede pedirle al router que abra un puerto hacia afuera.",
                    action = "Desactívalo si ningún equipo lo necesita.",
                ),
            )
        }
        if (router.portMappings.isEmpty()) {
            add(
                Finding(
                    id = "NO_INBOUND", kind = "NO_INBOUND", severity = "info", points = 0,
                    title = "Nada llega desde internet",
                    detail = when (router.reachOrUnknown()) {
                        com.baxner.vigia.data.agent.RouterReach.OUTER_RING -> "Tu router está detrás de otra red (CGNAT): nada de tu casa es alcanzable desde internet."
                        com.baxner.vigia.data.agent.RouterReach.INTERNET -> "Tu router tiene IP pública y no tiene redirecciones activas."
                        com.baxner.vigia.data.agent.RouterReach.UNKNOWN -> "No se pudo determinar si algo de tu red es alcanzable desde internet."
                    },
                    action = "",
                ),
            )
        }
    }

    private fun FindingSeverity.rank(): Int = when (this) {
        FindingSeverity.HIGH -> 3
        FindingSeverity.MEDIUM -> 2
        FindingSeverity.LOW -> 1
        FindingSeverity.INFO, FindingSeverity.UNKNOWN -> 0
    }
}
