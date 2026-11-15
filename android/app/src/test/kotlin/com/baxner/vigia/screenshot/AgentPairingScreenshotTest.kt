package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.agent.DiscoveredAgent
import com.baxner.vigia.ui.agent.AgentPairingScreenContent
import com.baxner.vigia.ui.agent.PairingStep
import com.baxner.vigia.ui.agent.PairingTarget
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de la pantalla de vinculación (ver el encargo, puntos 5 y
 * 6): la lista de agentes encontrados por mDNS y el campo de 6 dígitos
 * grande. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class AgentPairingScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun elegir_agente() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                AgentPairingScreenContent(
                    step = PairingStep.ChooseAgent(
                        discovered = listOf(DiscoveredAgent(id = "agent-1", name = "Mac-Estudio", host = "192.168.0.48", port = 7341)),
                        searching = true,
                    ),
                    onBack = {},
                    onRescan = {},
                    onSelectAgent = {},
                    onToggleManualEntry = {},
                    onManualHostChange = {},
                    onManualPortChange = {},
                    onContinueManual = {},
                    onCodeChange = {},
                    onSubmitCode = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/pairing_elegir_agente.png")
    }

    @Test
    fun codigo() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                AgentPairingScreenContent(
                    step = PairingStep.EnterCode(
                        target = PairingTarget(name = "Mac-Estudio", host = "192.168.0.48", port = 7341),
                        code = "4829",
                    ),
                    onBack = {},
                    onRescan = {},
                    onSelectAgent = {},
                    onToggleManualEntry = {},
                    onManualHostChange = {},
                    onManualPortChange = {},
                    onContinueManual = {},
                    onCodeChange = {},
                    onSubmitCode = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/pairing_codigo.png")
    }
}
