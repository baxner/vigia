package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.devices.DeviceEditScreenContent
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
 * Revisión visual de Editar: nombre, tipo (con el icono ya elegido), notas,
 * ubicación (con las sugerencias rápidas) e "Importante" encendido, sobre
 * un equipo ya personalizado (el mismo caso "Foco de la sala"). Regenerar
 * con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class DeviceEditScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun editar() {
        val now = Instant.now()
        val device = DemoFixtures.scanResult.devices.first { it.ip == "192.168.0.76" }
        val knownDevice = KnownDevice(
            id = 1L,
            network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24"),
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
            VigiaScreenshotFrame {
                DeviceEditScreenContent(device = knownDevice, onSave = {}, onCancel = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/editar.png")
    }
}
