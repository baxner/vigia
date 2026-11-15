package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import androidx.test.core.app.ApplicationProvider
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.AlertSettingsDto
import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.InternetNowDto
import com.baxner.vigia.data.agent.InternetSeriesPointDto
import com.baxner.vigia.data.agent.InternetSeriesRange
import com.baxner.vigia.data.agent.InternetSettingsDto
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.OutageDto
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.agent.SpeedTestDto
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.alerts.AlertsScreenContent
import com.baxner.vigia.ui.alerts.AlertsUiState
import com.baxner.vigia.ui.devices.DevicesScreenContent
import com.baxner.vigia.ui.internet.InternetScreenContent
import com.baxner.vigia.ui.internet.InternetUiState
import com.baxner.vigia.ui.panel.PanelScreenContent
import com.baxner.vigia.ui.permissions.rememberPermissionsState
import com.baxner.vigia.ui.scan.ScanStatus
import com.baxner.vigia.ui.scan.ScanUiState
import com.baxner.vigia.ui.security.SecurityScreenContent
import com.baxner.vigia.ui.security.SecurityUiState
import com.baxner.vigia.ui.wifi.WifiScreen
import com.baxner.vigia.ui.wifi.WifiViewModel
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.Instant
import java.time.temporal.ChronoUnit

/**
 * Pruebas de screenshot con fontScale aumentado (1.5x, el ajuste "grande" de
 * Android: Ajustes → Pantalla → Tamaño de letra) para Panel, Dispositivos,
 * Detalle de dispositivo, Seguridad, Internet, Alertas y Wi-Fi (accesibilidad):
 * verifica que la letra de verdad crece (antes, un intento
 * anterior escalaba [androidx.compose.material3.LocalTextStyle], que el texto
 * de la app no usa — salió una captura idéntica a la normal) y que nada queda
 * cortado, encimado o fuera de su tarjeta con letra más grande.
 *
 * Las capturas con letra normal de estas pantallas ya existen en sus propios
 * *ScreenshotTest.kt (PanelScreenshotTest, DevicesScreenshotTest...); estas
 * son solo la comparación con fontScale = 1.5f.
 *
 * Regenerar con:
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class AccessibilityScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now = Instant.parse("2026-10-11T15:00:00Z")
    private fun ago(minutes: Long) = now.minus(minutes, ChronoUnit.MINUTES).toString()
    private fun agoHours(hours: Long) = now.minus(hours, ChronoUnit.HOURS).toString()

    @Test
    fun panel_letra_grande() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            val (permissionsState, _) = rememberPermissionsState()
            VigiaScreenshotFrame(height = 1400.dp, fontScale = 1.5f) {
                PanelScreenContent(
                    uiState = ScanUiState(
                        network = DemoFixtures.currentNetwork,
                        status = ScanStatus.Idle,
                        result = DemoFixtures.scanResult,
                    ),
                    permissionsState = permissionsState,
                    onRequestPermissions = {},
                    onScan = {},
                    onCancel = {},
                    onOpenDevices = {},
                    demoMode = true,
                    securityScore = 72,
                    onOpenSecurity = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/panel_letra_grande.png")
    }

    @Test
    fun dispositivos_letra_grande() {
        val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
        val devices = DemoFixtures.scanResult.devices.take(3).mapIndexed { idx, device ->
            val custom = when (idx) {
                0 -> DeviceCustomization(name = "TV de la sala", typeId = "television", location = "Sala", important = true)
                1 -> DeviceCustomization()
                else -> DeviceCustomization(name = "Teléfono", location = "Dormitorio")
            }
            KnownDevice(
                id = (idx + 1).toLong(),
                network = network,
                mac = device.mac.ifEmpty { null },
                lastIp = device.ip,
                firstSeen = now.minus(3, ChronoUnit.DAYS),
                lastSeen = now,
                online = true,
                onlineSince = now.minus(3, ChronoUnit.DAYS),
                last = device,
                custom = custom,
            )
        }

        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1300.dp, fontScale = 1.5f) {
                DevicesScreenContent(
                    knownDevices = devices,
                    gateway = "192.168.0.1",
                    onDeviceClick = {},
                    phoneIp = "192.168.0.50",
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/dispositivos_letra_grande.png")
    }

    @Test
    fun detalle_letra_grande() {
        val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
        val device = DemoFixtures.scanResult.devices.first { it.ip == "192.168.0.76" }
        val knownDevice = KnownDevice(
            id = 1L,
            network = network,
            mac = device.mac.ifEmpty { null },
            lastIp = device.ip,
            firstSeen = now.minus(5, ChronoUnit.DAYS),
            lastSeen = now,
            online = true,
            onlineSince = now.minus(5, ChronoUnit.DAYS),
            last = device,
            custom = DeviceCustomization(
                name = "Foco de la sala",
                typeId = "smart_bulb",
                location = "Sala",
                notes = "Apagar antes de salir",
                important = true,
            ),
        )

        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1700.dp, fontScale = 1.5f) {
                com.baxner.vigia.ui.devices.DeviceDetailScreenContent(
                    device = knownDevice,
                    events = emptyList(),
                    portScanState = com.baxner.vigia.ui.scan.PortScanState.Idle,
                    onBack = {},
                    onEdit = {},
                    onScanAllPorts = {},
                    onForget = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/detalle_letra_grande.png")
    }

    @Test
    fun seguridad_letra_grande() {
        val findings = listOf(
            Finding(
                id = "RISKY_SERVICE|192.168.0.55|445", kind = "RISKY_SERVICE", severity = "high", points = 10,
                title = "Una computadora comparte archivos con toda la red",
                detail = "PC-Escritorio (192.168.0.55) tiene abierto SMB (445).",
                action = "Si no compartes carpetas desde esa computadora, desactiva el uso compartido.",
                deviceId = 55L, ip = "192.168.0.55", port = 445,
            ),
            Finding(
                id = "WIFI_WPS", kind = "WIFI_WPS", severity = "medium", points = 5,
                title = "WPS activado en tu Wi-Fi",
                detail = "WPS permite entrar con un PIN de 8 dígitos, más fácil de forzar que la contraseña.",
                action = "Apaga WPS en el router.",
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
        val report = SecurityReport(
            generatedAt = ago(2),
            score = 68,
            findings = findings,
            router = RouterInfo(ip = "192.168.0.4", mac = "02:00:00:10:20:01", upnp = true, natpmp = false, externalIp = "192.168.100.2", reach = "outer_ring"),
        )
        val uiState = SecurityUiState(report = report, monitoredByAgent = true)

        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1700.dp, fontScale = 1.5f) {
                SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/seguridad_letra_grande.png")
    }

    @Test
    @Config(qualifiers = "+h2200dp") // ver InternetScreenshotTest/SecurityScreenshotTest: lienzo más alto que el default necesita agrandar también el qualifier del dispositivo.
    fun internet_letra_grande() {
        val status = InternetStatusDto(
            state = "up",
            since = agoHours(6),
            now = InternetNowDto(routerMs = 3.1, modemMs = 4.0, internetMs = 18.2, dnsMs = 12.0, lossPct = 0.0),
            ipv6 = "ok",
            outages = listOf(OutageDto(start = ago(60 * 20), end = ago(60 * 20 - 3), durationSec = 180, blame = "isp")),
            speedTests = listOf(SpeedTestDto(at = ago(60 * 3), downMbps = 95.4, upMbps = 18.2, latencyMs = 11.0, loadedLatencyMs = 34.0)),
        )
        val series = (0 until 48).map { i ->
            InternetSeriesPointDto(
                at = ago((47 - i) * 30L),
                routerMs = 3.0 + (i % 5),
                modemMs = 4.0 + (i % 4),
                internetMs = 16.0 + (i % 10) * 1.5,
                dnsMs = 10.0,
                lossPct = 0.0,
            )
        }
        val uiState = InternetUiState(
            linked = true,
            monitoring = true,
            status = status,
            series = series,
            seriesRange = InternetSeriesRange.HOURS_24,
            settings = InternetSettingsDto(speedTestsPerDay = 2),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 2200.dp, fontScale = 1.5f) {
                InternetScreenContent(uiState = uiState, onBack = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/internet_letra_grande.png")
    }

    @Test
    fun alertas_letra_grande() {
        val alerts = listOf(
            AlertDto(
                id = 61, type = "NEW_DEVICE", severity = "high", at = now.minus(5, ChronoUnit.MINUTES).toString(),
                title = "Equipo nuevo en tu red", body = "Telefono-Invitado (192.168.0.24) se conectó por primera vez.",
                deviceId = 31, network = "k", read = false,
            ),
            AlertDto(
                id = 60, type = "IMPORTANT_OFFLINE", severity = "medium", at = now.minus(2, ChronoUnit.HOURS).toString(),
                title = "Equipo importante desconectado", body = "Impresora HP se desconectó.",
                deviceId = 12, network = "k", read = false,
            ),
            AlertDto(
                id = 59, type = "IMPORTANT_ONLINE", severity = "low", at = now.minus(1, ChronoUnit.DAYS).toString(),
                title = "Equipo importante conectado", body = "Cámara del jardín volvió a conectarse.",
                deviceId = 8, network = "k", read = true,
            ),
        )
        val uiState = AlertsUiState(linked = true, alerts = alerts, unreadCount = 2, settings = AlertSettingsDto())
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1400.dp, fontScale = 1.5f) {
                AlertsScreenContent(uiState = uiState, onBack = {}, onOpenSettings = {}, showNotificationBanner = true, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/alertas_letra_grande.png")
    }

    @Test
    @Config(qualifiers = "+h2800dp") // la lista de redes (WifiNetworkRow) con sus badges necesita bajar bastante más que el analizador de canales.
    fun wifi_letra_grande() {
        composeTestRule.mainClock.autoAdvance = false
        val wifiViewModel = WifiViewModel(ApplicationProvider.getApplicationContext())
        wifiViewModel.enableDemoMode()
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 2800.dp, fontScale = 1.5f) {
                WifiScreen(
                    wifiViewModel = wifiViewModel,
                    hasLocationPermission = true,
                    onRequestPermissions = {},
                    demoMode = true,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/wifi_letra_grande.png")
    }
}
