package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.ui.agent.AgentSettingsScreenContent
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de los ajustes del agente (ver `docs/plan.md` § "Fase
 * 5.1" y la fase 8): el interruptor "Avisos al instante" apagado (default) y
 * prendido, y "Ver tu casa remoto" (fase 8) en sus dos estados, con su
 * explicación. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class AgentSettingsScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun avisos_al_instante_apagado() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 640.dp) {
                AgentSettingsScreenContent(
                    agentName = "Mac-Estudio",
                    hostAndPort = "192.168.0.41" to 47283,
                    onBack = {},
                    onUnlink = {},
                    liveWatchEnabled = false,
                    onLiveWatchToggle = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_settings_avisos_al_instante_apagado.png")
    }

    @Test
    fun avisos_al_instante_prendido() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 640.dp) {
                AgentSettingsScreenContent(
                    agentName = "Mac-Estudio",
                    hostAndPort = "192.168.0.41" to 47283,
                    onBack = {},
                    onUnlink = {},
                    liveWatchEnabled = true,
                    onLiveWatchToggle = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_settings_avisos_al_instante_prendido.png")
    }

    /** Fase 8 (ver el encargo, punto 2: el selector): "Ver tu casa remoto" prendido (default). */
    @Test
    fun ver_remoto_prendido() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 640.dp) {
                AgentSettingsScreenContent(
                    agentName = "Mac-Estudio",
                    hostAndPort = "192.168.0.41" to 47283,
                    onBack = {},
                    onUnlink = {},
                    remoteViewPreferred = true,
                    onRemoteViewToggle = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_settings_ver_remoto_prendido.png")
    }

    /** Fase 8: "Ver tu casa remoto" apagado — el usuario prefiere quedarse con la red actual estando fuera de casa. */
    @Test
    fun ver_remoto_apagado() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 640.dp) {
                AgentSettingsScreenContent(
                    agentName = "Mac-Estudio",
                    hostAndPort = "192.168.0.41" to 47283,
                    onBack = {},
                    onUnlink = {},
                    remoteViewPreferred = false,
                    onRemoteViewToggle = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_settings_ver_remoto_apagado.png")
    }
}
