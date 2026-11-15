package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceEvent
import com.baxner.vigia.data.history.DeviceEventType
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.Identity
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.devices.DeviceDetailScreenContent
import com.baxner.vigia.ui.scan.PortScanState
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
import androidx.compose.ui.unit.dp

/**
 * Revisión visual del Detalle de dispositivo: la TV (identidad fuerte, 99%),
 * el PC (puertos de riesgo), un sin identificar (pista débil ssdp, 30%) y el
 * foco inteligente personalizado con cronología completa (los 4 tipos de
 * evento). Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class DeviceDetailScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val now: Instant = Instant.now()

    private fun knownDeviceByIp(ip: String, custom: DeviceCustomization = DeviceCustomization()): KnownDevice {
        val device = DemoFixtures.scanResult.devices.first { it.ip == ip }
        return KnownDevice(
            id = 1L,
            network = network,
            mac = device.mac.ifEmpty { null },
            lastIp = device.ip,
            firstSeen = now.minus(5, ChronoUnit.DAYS),
            lastSeen = now,
            online = true,
            onlineSince = now.minus(5, ChronoUnit.DAYS),
            last = device,
            custom = custom,
        )
    }

    private fun capture(
        device: KnownDevice,
        events: List<DeviceEvent>,
        fileName: String,
        tall: Boolean = false,
        portScanState: PortScanState = PortScanState.Idle,
        frameHeight: androidx.compose.ui.unit.Dp = if (tall) 1500.dp else 915.dp,
    ) {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = frameHeight) {
                DeviceDetailScreenContent(
                    device = device,
                    events = events,
                    portScanState = portScanState,
                    onBack = {},
                    onEdit = {},
                    onScanAllPorts = {},
                    onForget = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/$fileName.png")
    }

    @Test
    fun detalle_tv() {
        val device = knownDeviceByIp("192.168.0.35")
        capture(device, listOf(DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS))), "detalle_tv")
    }

    @Test
    fun detalle_pc() {
        val device = knownDeviceByIp("192.168.0.4")
        capture(device, listOf(DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS))), "detalle_pc")
    }

    // Resultado del escaneo completo (el que sigue aunque se salga del
    // detalle), con su hora, y los valores de red que se pueden copiar.
    @Test
    @Config(qualifiers = "+h2200dp") // pantalla alta: los puertos quedan muy abajo
    fun detalle_pc_escaneo_completo() {
        val device = knownDeviceByIp("192.168.0.4")
        capture(
            device,
            listOf(DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS))),
            "detalle_pc_escaneo_completo",
            frameHeight = 2200.dp,
            portScanState = PortScanState.Done(device.last.ip, listOf(135, 139, 445, 3389, 5040, 7680), now.minus(5, ChronoUnit.MINUTES)),
        )
    }

    @Test
    fun detalle_sin_identificar() {
        val device = knownDeviceByIp("192.168.0.67")
        capture(device, listOf(DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS))), "detalle_sin_identificar")
    }

    // Personalizado (nombre, tipo, ubicación, importante) con cronología
    // completa: los 4 tipos de evento, para revisar icono/color/texto de cada uno.
    @Test
    fun detalle_foco_inteligente() {
        val device = knownDeviceByIp(
            "192.168.0.76",
            custom = DeviceCustomization(name = "Foco de la sala", typeId = "smart_bulb", location = "Sala", notes = "Apagar antes de salir", important = true),
        )
        val events = listOf(
            DeviceEvent(device.id, DeviceEventType.CAME_ONLINE, now.minus(1, ChronoUnit.HOURS)),
            DeviceEvent(device.id, DeviceEventType.WENT_OFFLINE, now.minus(2, ChronoUnit.HOURS)),
            DeviceEvent(device.id, DeviceEventType.IP_CHANGED, now.minus(1, ChronoUnit.DAYS), "192.168.0.70 → 192.168.0.76"),
            DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS)),
        )
        capture(device, events, "detalle_foco_inteligente")
    }

    // Equipo mínimo (sin pistas, sin servicios, sin puertos) a propósito:
    // deja ver la Cronología completa, con los 4 tipos de evento y su
    // icono/color, sin pelear por espacio con las demás secciones.
    @Test
    fun detalle_cronologia() {
        val minimal = Device(
            ip = "192.168.0.99",
            mac = "aa:bb:cc:dd:ee:ff",
            identity = Identity(type = "iot", confidence = 60),
        )
        val device = KnownDevice(
            id = 1L,
            network = network,
            mac = minimal.mac,
            lastIp = minimal.ip,
            firstSeen = now.minus(5, ChronoUnit.DAYS),
            lastSeen = now,
            online = true,
            onlineSince = now.minus(1, ChronoUnit.HOURS),
            last = minimal,
        )
        val events = listOf(
            DeviceEvent(device.id, DeviceEventType.CAME_ONLINE, now.minus(1, ChronoUnit.HOURS)),
            DeviceEvent(device.id, DeviceEventType.WENT_OFFLINE, now.minus(2, ChronoUnit.HOURS)),
            DeviceEvent(device.id, DeviceEventType.IP_CHANGED, now.minus(1, ChronoUnit.DAYS), "192.168.0.70 → 192.168.0.99"),
            DeviceEvent(device.id, DeviceEventType.FIRST_SEEN, now.minus(5, ChronoUnit.DAYS)),
        )
        capture(device, events, "detalle_cronologia", tall = true)
    }
}
