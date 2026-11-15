package com.baxner.vigia.screenshot

import com.baxner.vigia.data.agent.SecurityDevice
import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.KnownDeviceDto
import com.baxner.vigia.data.agent.OuterRing
import com.baxner.vigia.data.agent.PortMapping
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.Identity
import com.baxner.vigia.ui.scan.PortScanState
import com.baxner.vigia.ui.security.RouterFromHereUi
import com.baxner.vigia.ui.security.SecurityScreenContent
import com.baxner.vigia.ui.security.SecurityUiState
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import java.time.Instant
import java.time.temporal.ChronoUnit
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de la pantalla Seguridad (ver el encargo, punto 6): con
 * agente e informe completo (hallazgos de las 4 severidades, router,
 * redirecciones y anillo exterior), sin agente (estado vacío) y "Tu router
 * visto desde aquí" (el teléfono en la Wi-Fi de la ONT). Datos de muestra
 * realistas de esta red (Router-Lab 192.168.0.4, salida 192.168.100.2 detrás de
 * la ONT). Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class SecurityScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now = Instant.parse("2026-10-08T15:00:00Z")
    private fun ago(minutes: Long) = now.minus(minutes, ChronoUnit.MINUTES).toString()
    private fun agoDays(days: Long) = now.minus(days, ChronoUnit.DAYS).toString()

    /** PC-Escritorio: SMB, RDP y RTSP abiertos — los 3 puertos de riesgo que pide el encargo. */
    private val desktopId = 55L

    private fun fullReport(): SecurityReport {
        val router = RouterInfo(
            ip = "192.168.0.4",
            mac = "02:00:00:10:20:01",
            upnp = true,
            natpmp = true,
            externalIp = "192.168.100.2",
            reach = "outer_ring",
            portMappings = listOf(
                PortMapping(
                    protocol = "TCP", externalPort = 33389, internalIp = "192.168.0.55", internalPort = 3389,
                    description = "RDP-Escritorio", source = "upnp", deviceId = desktopId,
                    firstSeen = agoDays(6), lastSeen = ago(15),
                ),
            ),
        )
        val outerRing = OuterRing(
            network = "192.168.100.1|192.168.100.0/24",
            subnet = "192.168.100.0/24",
            gateway = "192.168.100.1",
            lastScanAt = ago(8),
            devices = listOf(
                SecurityDevice(
                    id = 201, name = "ONT-Proveedor", online = true,
                    device = Device(ip = "192.168.100.1", vendor = "ONT-ISP", identity = Identity(type = "router", brand = "ONT-ISP", confidence = 70), openPorts = listOf(80)),
                ),
                SecurityDevice(
                    id = 202, online = true,
                    device = Device(ip = "192.168.100.23", sources = listOf("icmp")),
                ),
            ),
        )
        val findings = listOf(
            Finding(
                id = "RISKY_SERVICE|192.168.0.55|3389", kind = "RISKY_SERVICE", severity = "high", points = 10,
                title = "Una computadora ofrece escritorio remoto a toda la red",
                detail = "PC-Escritorio (192.168.0.55) tiene abierto RDP (3389), blanco frecuente de fuerza bruta.",
                action = "Si no te conectas por escritorio remoto desde otro equipo de la casa, desactívalo.",
                deviceId = desktopId, ip = "192.168.0.55", port = 3389,
            ),
            Finding(
                id = "PORT_MAPPING|TCP|33389", kind = "PORT_MAPPING", severity = "high", points = 12,
                title = "Tu router redirige el puerto 33389",
                detail = "TCP 33389 → 192.168.0.55:3389 (RDP), pedida por UPnP hace 6 días.",
                action = "Si no la pediste a propósito, bórrala en el router.",
                deviceId = desktopId, ip = "192.168.0.55", port = 3389,
            ),
            Finding(
                id = "RISKY_SERVICE|192.168.0.55|445", kind = "RISKY_SERVICE", severity = "high", points = 10,
                title = "Una computadora comparte archivos con toda la red",
                detail = "PC-Escritorio (192.168.0.55) tiene abierto SMB (445).",
                action = "Si no compartes carpetas desde esa computadora, desactiva el uso compartido.",
                deviceId = desktopId, ip = "192.168.0.55", port = 445,
            ),
            Finding(
                id = "RISKY_SERVICE|192.168.0.55|554", kind = "RISKY_SERVICE", severity = "medium", points = 5,
                title = "Una computadora ofrece un stream de cámara",
                detail = "PC-Escritorio (192.168.0.55) tiene abierto RTSP (554), a veces sin autenticación.",
                action = "Revisa si de verdad necesitas ese servicio ahí.",
                deviceId = desktopId, ip = "192.168.0.55", port = 554,
            ),
            Finding(
                id = "WIFI_WPS", kind = "WIFI_WPS", severity = "medium", points = 5,
                title = "WPS activado en tu Wi-Fi",
                detail = "WPS permite entrar con un PIN de 8 dígitos, más fácil de forzar que la contraseña.",
                action = "Apaga WPS en el router.",
            ),
            Finding(
                id = "WIFI_WPA2", kind = "WIFI_WPA2", severity = "low", points = 3,
                title = "Tu Wi-Fi usa WPA2",
                detail = "WPA2 sigue siendo razonable, pero WPA3 es más resistente.",
                action = "Si tu router lo soporta, cambia a WPA3.",
            ),
            Finding(
                id = "UNKNOWN_DEVICES", kind = "UNKNOWN_DEVICES", severity = "info", points = 0,
                title = "2 equipos sin identificar",
                detail = "Hay 2 equipos en línea que Vigía no pudo identificar.",
                action = "Revísalos en Dispositivos; si no los reconoces, es buen momento para preguntarte qué son.",
            ),
            Finding(
                id = "OUTER_RING_DEVICES", kind = "OUTER_RING_DEVICES", severity = "info", points = 0,
                title = "2 equipos en la red de delante de tu router",
                detail = "La ONT (192.168.100.1) y otro equipo más comparten esa red con la salida de tu router.",
                action = "",
            ),
            Finding(
                id = "UPNP_ENABLED", kind = "UPNP_ENABLED", severity = "info", points = 0,
                title = "UPnP activo en tu router",
                detail = "Cualquier equipo de tu red puede pedirle al router que abra un puerto hacia afuera.",
                action = "Desactívalo si ningún equipo lo necesita.",
            ),
        )
        return SecurityReport(generatedAt = ago(2), score = 55, findings = findings, router = router, outerRing = outerRing)
    }

    @Test
    @Config(qualifiers = "+h2000dp") // pantalla alta: 8 hallazgos + router + redirecciones + anillo exterior no caben en un teléfono normal
    fun seguridad_con_agente() {
        val uiState = SecurityUiState(report = fullReport(), monitoredByAgent = true)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            // Pantalla alta (ver DeviceDetailScreenshotTest): con 8 hallazgos +
            // router + redirecciones + anillo exterior, el contenido pasa de
            // sobra los 2400dp de un teléfono normal (LazyColumn es perezoso:
            // lo que no entra en el lienzo ni se compone).
            VigiaScreenshotFrame(height = 2000.dp) {
                SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/seguridad_con_agente.png")
    }

    @Test
    fun seguridad_sin_agente() {
        val uiState = SecurityUiState(report = null, monitoredByAgent = false)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 400.dp) {
                SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/seguridad_sin_agente.png")
    }

    /** "A propósito" (ver el contrato § "Hallazgos reconocidos v1.3"): un hallazgo activo y uno reconocido, atenuado, al final. */
    @Test
    fun seguridad_con_hallazgo_a_proposito() {
        val findings = listOf(
            Finding(
                id = "RISKY_SERVICE|192.168.0.55|445", kind = "RISKY_SERVICE", severity = "high", points = 10,
                title = "Una computadora comparte archivos con toda la red",
                detail = "PC-Escritorio (192.168.0.55) tiene abierto SMB (445).",
                action = "Si no compartes carpetas desde esa computadora, desactiva el uso compartido.",
                deviceId = desktopId, ip = "192.168.0.55", port = 445,
            ),
            Finding(
                id = "RISKY_SERVICE|192.168.0.63|554", kind = "RISKY_SERVICE", severity = "medium", points = 5,
                title = "Una cámara ofrece un stream sin cifrar",
                detail = "Camara-IP (192.168.0.63) tiene abierto RTSP (554), a veces sin autenticación.",
                action = "Revisa si de verdad necesitas ese servicio ahí.",
                ip = "192.168.0.63", port = 554,
                acknowledged = true, acknowledgedAt = ago(60 * 24),
            ),
        )
        val report = SecurityReport(generatedAt = ago(2), score = 90, findings = findings)
        val uiState = SecurityUiState(report = report, monitoredByAgent = true)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 950.dp) {
                SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/seguridad_con_hallazgo_a_proposito.png")
    }

    @Test
    fun seguridad_router_visto_desde_aqui() {
        val report = SecurityReport(
            generatedAt = ago(1),
            score = 92,
            findings = listOf(
                Finding(
                    id = "NO_INBOUND", kind = "NO_INBOUND", severity = "info", points = 0,
                    title = "Nada llega desde internet",
                    detail = "Tu router está detrás de otra red (CGNAT): nada de tu casa es alcanzable desde internet.",
                    action = "",
                ),
            ),
            router = null,
            outerRing = null,
        )
        val uiState = SecurityUiState(report = report, monitoredByAgent = false, lastRouterInfo = RouterInfo(ip = "192.168.0.4", externalIp = "192.168.100.2", upnp = true))
        val routerFromHere = RouterFromHereUi.Visible(
            externalIp = "192.168.100.2",
            openPorts = listOf(80, 3389),
            portScanState = PortScanState.Idle,
            canScanAllPorts = true,
            // El módem de ESTA red (la ONT), no el router principal (ver la revisión de la fase 5, punto 5).
            modemGatewayIp = "192.168.100.1",
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 950.dp) {
                SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, routerFromHere = routerFromHere, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/seguridad_router_visto_desde_aqui.png")
    }
}
