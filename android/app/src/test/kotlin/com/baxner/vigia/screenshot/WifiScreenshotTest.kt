package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.test.core.app.ApplicationProvider
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

/**
 * Revisión visual de Wi-Fi con las 9 redes de muestra (la red "Casa" es un
 * mesh de 3 BSSID). A diferencia de Panel/Dispositivos/Detalle, WifiViewModel
 * no toca el motor nativo (solo WifiManager), así que aquí se usa el
 * ViewModel real en modo demo en vez de un *ScreenContent separado.
 * Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class WifiScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun wifi() {
        composeTestRule.mainClock.autoAdvance = false
        val wifiViewModel = WifiViewModel(ApplicationProvider.getApplicationContext())
        wifiViewModel.enableDemoMode()
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                WifiScreen(
                    wifiViewModel = wifiViewModel,
                    hasLocationPermission = true,
                    onRequestPermissions = {},
                    demoMode = true,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/wifi.png")
    }
}
