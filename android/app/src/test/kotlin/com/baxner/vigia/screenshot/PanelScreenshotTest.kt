package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.panel.PanelScreenContent
import com.baxner.vigia.ui.permissions.rememberPermissionsState
import com.baxner.vigia.ui.scan.ScanStatus
import com.baxner.vigia.ui.scan.ScanUiState
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual del Panel (ver encargo "Radar Nocturno"): sin emulador,
 * Roborazzi + Robolectric capturan PanelScreenContent con los mismos datos
 * de muestra que el modo demo (DemoFixtures). Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 *
 * Los PNG quedan en app/build/outputs/roborazzi/ (build/, no se versiona).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class PanelScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun panel_con_resultado() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            val (permissionsState, _) = rememberPermissionsState()
            VigiaScreenshotFrame {
                PanelScreenContent(
                    uiState = ScanUiState(
                        network = DemoFixtures.currentNetwork,
                        status = ScanStatus.Idle,
                        result = DemoFixtures.scanResult,
                    ),
                    permissionsState = permissionsState,
                    onRequestPermissions = {},
                    onScan = {},
                    onCancel = {},
                    onOpenDevices = {},
                    demoMode = true,
                )
            }
        }
        // Deja asentar las animaciones de entrada (conteo, fundido del radar).
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/panel_con_resultado.png")
    }

    // Con informe de seguridad disponible (ver el encargo, punto 5): la tile de
    // en medio deja "Riesgos" y muestra el puntaje bajo "Seguridad".
    @Test
    fun panel_con_seguridad() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            val (permissionsState, _) = rememberPermissionsState()
            VigiaScreenshotFrame {
                PanelScreenContent(
                    uiState = ScanUiState(
                        network = DemoFixtures.currentNetwork,
                        status = ScanStatus.Idle,
                        result = DemoFixtures.scanResult,
                    ),
                    permissionsState = permissionsState,
                    onRequestPermissions = {},
                    onScan = {},
                    onCancel = {},
                    onOpenDevices = {},
                    demoMode = true,
                    securityScore = 55,
                    onOpenSecurity = {},
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/panel_con_seguridad.png")
    }

    @Test
    fun panel_escaneando() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            val (permissionsState, _) = rememberPermissionsState()
            VigiaScreenshotFrame {
                PanelScreenContent(
                    uiState = ScanUiState(
                        network = DemoFixtures.currentNetwork,
                        status = ScanStatus.Scanning(stage = "identify", done = 7, total = 12),
                        result = null,
                    ),
                    permissionsState = permissionsState,
                    onRequestPermissions = {},
                    onScan = {},
                    onCancel = {},
                    onOpenDevices = {},
                    demoMode = true,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/panel_escaneando.png")
    }

    /**
     * Fase 8 (ver el encargo, punto 2: "que quede claro qué se está
     * viendo"): la insignia de la cabecera cambia a "Tu casa, remoto" en vez
     * de "Vigilada 24/7" — con `demoMode = false` para que se dibuje la
     * tarjeta "Vigilancia 24/7" (el Panel la oculta en demo).
     */
    @Test
    fun panel_viendo_remoto() {
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            val (permissionsState, _) = rememberPermissionsState()
            VigiaScreenshotFrame {
                PanelScreenContent(
                    uiState = ScanUiState(network = DemoFixtures.currentNetwork, status = ScanStatus.Idle, result = null),
                    permissionsState = permissionsState,
                    onRequestPermissions = {},
                    onScan = {},
                    onCancel = {},
                    onOpenDevices = {},
                    demoMode = false,
                    agentState = AgentPanelState.Linked(
                        agentName = "Mac-Estudio",
                        reachable = true,
                        monitoringCurrentNetwork = false,
                        lastScanAt = null,
                        viewingRemote = true,
                        remoteAvailable = true,
                    ),
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/panel_viendo_remoto.png")
    }
}
