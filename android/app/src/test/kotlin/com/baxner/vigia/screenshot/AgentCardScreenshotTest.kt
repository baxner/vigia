package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.agent.InternetState
import com.baxner.vigia.ui.agent.AgentCard
import com.baxner.vigia.ui.internet.InternetPanelSummary
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import java.time.Instant
import java.time.temporal.ChronoUnit
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de la tarjeta "Vigilancia 24/7" (ver el encargo, punto 6):
 * sin agente (invita a vincular) y vinculado y vigilando la red actual.
 * Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class AgentCardScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun sin_vincular() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AgentCard(
                    state = AgentPanelState.NotLinked,
                    onLinkClick = {},
                    onManageClick = {},
                    modifier = androidx.compose.ui.Modifier.padding(16.dp),
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_card_sin_vincular.png")
    }

    @Test
    fun vigilando() {
        val now = Instant.parse("2026-10-06T22:10:00Z")
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AgentCard(
                    state = AgentPanelState.Linked(
                        agentName = "Mac-Estudio",
                        reachable = true,
                        monitoringCurrentNetwork = true,
                        lastScanAt = now.minus(2, ChronoUnit.MINUTES),
                    ),
                    onLinkClick = {},
                    onManageClick = {},
                    modifier = androidx.compose.ui.Modifier.padding(16.dp),
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_card_vigilando.png")
    }

    /** Fase 8 (ver el encargo: "todas las pantallas fuera de casa por Tailscale"): viendo la casa remoto. */
    @Test
    fun viendo_remoto() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AgentCard(
                    state = AgentPanelState.Linked(
                        agentName = "Mac-Estudio",
                        reachable = true,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                        viewingRemote = true,
                        remoteAvailable = true,
                    ),
                    onLinkClick = {},
                    onManageClick = {},
                    modifier = androidx.compose.ui.Modifier.padding(16.dp),
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_card_viendo_remoto.png")
    }

    /** Fase 8: fuera de casa y ni la LAN del vínculo ni Tailscale contestaron (ver el encargo, punto 4). */
    @Test
    fun fuera_de_casa_inalcanzable() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AgentCard(
                    state = AgentPanelState.Linked(
                        agentName = "Mac-Estudio",
                        reachable = false,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                    ),
                    onLinkClick = {},
                    onManageClick = {},
                    modifier = androidx.compose.ui.Modifier.padding(16.dp),
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_card_fuera_de_casa_inalcanzable.png")
    }

    /** Fase 7 (ver `docs/plan.md` § "Fase 7"): acceso a la pantalla Internet desde la tarjeta "Vigilancia 24/7" del Panel. */
    @Test
    fun vigilando_con_internet() {
        val now = Instant.parse("2026-10-11T22:10:00Z")
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 300.dp) {
                AgentCard(
                    state = AgentPanelState.Linked(
                        agentName = "Mac-Estudio",
                        reachable = true,
                        monitoringCurrentNetwork = true,
                        lastScanAt = now.minus(2, ChronoUnit.MINUTES),
                    ),
                    onLinkClick = {},
                    onManageClick = {},
                    modifier = androidx.compose.ui.Modifier.padding(16.dp),
                    now = now,
                    internetSummary = InternetPanelSummary(state = InternetState.UP, latencyMs = 18),
                    onOpenInternet = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/agent_card_vigilando_con_internet.png")
    }
}
