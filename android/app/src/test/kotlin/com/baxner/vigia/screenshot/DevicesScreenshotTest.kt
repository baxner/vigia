package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.Identity
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.devices.DevicesScreenContent
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
 * Revisión visual de Dispositivos: las dos secciones (en línea/sin conexión),
 * la insignia "Nuevo", el icono de "Importante" y un equipo con nombre/tipo
 * personalizado (el mismo caso del modo demo: "Foco de la sala"). Regenerar
 * con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class DevicesScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun dispositivos() {
        val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
        val now = Instant.now()
        var nextId = 1L

        // Solo unos cuantos en línea (no los 12 del modo demo): con la altura
        // fija de las capturas, un viewport lleno de solo "en línea" nunca
        // deja ver "sin conexión" debajo. Estos 5 ya cubren "Este teléfono",
        // identificado con marca, "Nuevo", personalizado+importante y sin
        // identificar.
        val onlineIps = listOf("192.168.0.23", "192.168.0.48", "192.168.0.52", "192.168.0.76", "192.168.0.88")
        val online = DemoFixtures.scanResult.devices.filter { it.ip in onlineIps }.map { device ->
            KnownDevice(
                id = nextId++,
                network = network,
                mac = device.mac.ifEmpty { null },
                lastIp = device.ip,
                firstSeen = now.minus(3, ChronoUnit.DAYS),
                lastSeen = now,
                online = true,
                onlineSince = now.minus(3, ChronoUnit.DAYS),
                last = device,
            )
        }.toMutableList()
        check(online.size == onlineIps.size) { "faltan fixtures en línea" }

        // Personalizado: el motor lo detectó como "iot"/Tuya, el usuario lo
        // renombró, le puso ubicación y lo marcó importante.
        val smartPlugIndex = online.indexOfFirst { it.lastIp == "192.168.0.76" }
        online[smartPlugIndex] = online[smartPlugIndex].copy(
            custom = DeviceCustomization(name = "Foco de la sala", typeId = "smart_bulb", location = "Sala", important = true),
        )

        // "Nuevo": visto por primera vez hace 10 minutos (< 24 h).
        val echoIndex = online.indexOfFirst { it.lastIp == "192.168.0.52" }
        val recent = now.minus(10, ChronoUnit.MINUTES)
        online[echoIndex] = online[echoIndex].copy(firstSeen = recent, onlineSince = recent)

        val oldPrinter = Device(
            ip = "192.168.0.15",
            mac = "02:00:00:10:30:05",
            vendor = "HP Inc.",
            hostname = "",
            identity = Identity(type = "printer", brand = "HP", model = "LaserJet P1102", confidence = 88, reason = "arp; puerto 9100"),
            openPorts = listOf(9100),
            sources = listOf("arp"),
        )
        val unknown3 = DemoFixtures.scanResult.devices.first { it.ip == "192.168.0.142" }
        val offline = listOf(
            KnownDevice(
                id = nextId++,
                network = network,
                mac = oldPrinter.mac,
                lastIp = oldPrinter.ip,
                firstSeen = now.minus(10, ChronoUnit.DAYS),
                lastSeen = now.minus(1, ChronoUnit.DAYS),
                online = false,
                onlineSince = null,
                last = oldPrinter,
            ),
            KnownDevice(
                id = nextId++,
                network = network,
                mac = unknown3.mac.ifEmpty { null },
                lastIp = unknown3.ip,
                firstSeen = now.minus(6, ChronoUnit.DAYS),
                lastSeen = now.minus(3, ChronoUnit.HOURS),
                online = false,
                onlineSince = null,
                last = unknown3,
            ),
        )

        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                DevicesScreenContent(
                    knownDevices = online + offline,
                    gateway = "192.168.0.1",
                    onDeviceClick = {},
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/dispositivos.png")
    }
}
