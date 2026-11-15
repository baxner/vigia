package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.ui.devices.DeviceTypePickerContent
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual del Selector de tipo: catálogo por categorías, buscable,
 * con "Automático" fijo arriba y el tipo ya elegido resaltado en cian con
 * su check. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class DeviceTypePickerScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun selector_de_tipo() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                DeviceTypePickerContent(selectedId = "smart_bulb", onSelect = {}, onClose = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/selector_de_tipo.png")
    }
}
