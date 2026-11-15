package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.radar.BearingConfidence
import com.baxner.vigia.data.radar.BearingEstimate
import com.baxner.vigia.data.radar.DistanceRange
import com.baxner.vigia.data.radar.SignalSourceState
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.data.radar.Trend
import com.baxner.vigia.ui.radar.HuntScreenContent
import com.baxner.vigia.ui.radar.HuntUiState
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de "Buscar señal" (fase 6, ver docs/plan.md): a media
 * vuelta guiada (Bluetooth), con rumbo ya estimado y chip RTT (Wi-Fi), y el
 * aviso de limitación de escaneos. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class HuntScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val bleTarget = SignalTarget.Ble(
        address = "02:00:00:12:34:10", name = "Tira", brand = "Govee", txPowerDbm = 0, label = "Tira Govee",
    )
    private val wifiTarget = SignalTarget.WifiAp(
        bssid = "02:00:00:10:20:01", ssid = "Router-Lab", frequencyMhz = 5180,
        rttCapable = true, connected = false, label = "Router-Lab",
    )

    @Test
    @Config(qualifiers = "+h1400dp") // pantalla alta: detector + radar + vuelta guiada no caben en un teléfono normal
    fun buscar_senal_media_vuelta_bluetooth() {
        val uiState = HuntUiState(
            target = bleTarget,
            sourceState = SignalSourceState.OK,
            smoothedRssi = -58.0,
            trend = Trend.CLOSER,
            distance = DistanceRange(minMeters = 1.2, typicalMeters = 2.1, maxMeters = 3.8),
            headingDeg = 160f,
            guidedTurnActive = true,
            // Bluetooth: minPerSector = 5. Los primeros 4 sectores ya se llenaron, el resto va a medias.
            sectorCounts = listOf(5, 5, 5, 5, 3, 1, 0, 0),
            bearing = null,
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1400.dp) {
                HuntScreenContent(uiState = uiState, onBack = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/buscar_senal_media_vuelta_bluetooth.png")
    }

    @Test
    @Config(qualifiers = "+h1400dp")
    fun buscar_senal_rumbo_estimado_rtt_wifi() {
        val uiState = HuntUiState(
            target = wifiTarget,
            sourceState = SignalSourceState.OK,
            smoothedRssi = -52.0,
            trend = Trend.STEADY,
            distance = DistanceRange(minMeters = 1.0, typicalMeters = 1.8, maxMeters = 3.0),
            rttMeters = 1.8,
            headingDeg = 15f,
            guidedTurnActive = false,
            // Wi-Fi: minPerSector = 2, la vuelta ya se completó en los 8 sectores.
            sectorCounts = listOf(3, 2, 2, 4, 2, 3, 2, 2),
            bearing = BearingEstimate(bearingDeg = 70.0, swingDb = 9.0, confidence = BearingConfidence.HIGH),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1400.dp) {
                HuntScreenContent(uiState = uiState, onBack = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/buscar_senal_rumbo_estimado_rtt_wifi.png")
    }

    @Test
    @Config(qualifiers = "+h1400dp")
    fun buscar_senal_aviso_limite_escaneos() {
        val uiState = HuntUiState(
            target = wifiTarget.copy(rttCapable = false),
            sourceState = SignalSourceState.SLOW_THROTTLED,
            smoothedRssi = -68.0,
            trend = Trend.STEADY,
            distance = DistanceRange(minMeters = 2.0, typicalMeters = 4.0, maxMeters = 7.5),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1400.dp) {
                HuntScreenContent(uiState = uiState, onBack = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/buscar_senal_aviso_limite_escaneos.png")
    }
}
