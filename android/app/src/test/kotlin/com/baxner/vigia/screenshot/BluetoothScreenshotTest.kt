package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.radar.BleDevice
import com.baxner.vigia.data.radar.SignalSourceState
import com.baxner.vigia.ui.bluetooth.BluetoothScreenContent
import com.baxner.vigia.ui.bluetooth.BluetoothUiState
import androidx.compose.ui.unit.dp
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import java.time.Instant
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de la pestaña Bluetooth (fase 6, ver docs/plan.md): varios
 * equipos (uno sin nombre emparentado con la Tira Govee de la red, uno con
 * dirección aleatoria) y la pantalla sin permiso. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class BluetoothScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now = Instant.parse("2026-10-08T15:00:00Z")
    private fun ago(seconds: Long) = now.minusSeconds(seconds).toEpochMilli()

    /** Sin nombre anunciado: se identifica solo por parentesco de MAC con la Tira Govee de la red. */
    private val siblingDevice = BleDevice(
        address = "02:00:00:12:34:12",
        name = null,
        brand = null,
        rssi = -58,
        txPowerDbm = null,
        lastSeenMillis = ago(5),
        randomAddress = false,
    )
    private val randomAddressDevice = BleDevice(
        address = "02:00:00:dd:ee:0f",
        name = "Galaxy Buds2",
        brand = "Samsung",
        rssi = -72,
        txPowerDbm = -59,
        lastSeenMillis = ago(2),
        randomAddress = true,
    )
    private val keyboardDevice = BleDevice(
        address = "02:00:00:20:11:02",
        name = "Teclado K380",
        brand = "Logitech",
        rssi = -66,
        txPowerDbm = 4,
        lastSeenMillis = ago(30),
        randomAddress = false,
    )
    private val weakRandomDevice = BleDevice(
        address = "02:00:00:9a:bb:cd",
        name = null,
        brand = null,
        rssi = -85,
        txPowerDbm = null,
        lastSeenMillis = ago(120),
        randomAddress = true,
    )

    private val knownNetworkDevices = listOf("02:00:00:12:34:10" to "Tira Govee")

    @Test
    fun bluetooth_con_equipos() {
        val uiState = BluetoothUiState(
            devices = listOf(siblingDevice, randomAddressDevice, keyboardDevice, weakRandomDevice),
            state = SignalSourceState.OK,
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                BluetoothScreenContent(
                    uiState = uiState,
                    hasPermission = true,
                    onRequestPermission = {},
                    knownDevices = knownNetworkDevices,
                    onToggleOnlyNamed = {},
                    onHunt = {},
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/bluetooth_con_equipos.png")
    }

    @Test
    fun bluetooth_sin_permiso() {
        val uiState = BluetoothUiState(devices = emptyList(), state = SignalSourceState.NO_PERMISSION)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 450.dp) {
                BluetoothScreenContent(
                    uiState = uiState,
                    hasPermission = false,
                    onRequestPermission = {},
                    knownDevices = emptyList(),
                    onToggleOnlyNamed = {},
                    onHunt = {},
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/bluetooth_sin_permiso.png")
    }
}
