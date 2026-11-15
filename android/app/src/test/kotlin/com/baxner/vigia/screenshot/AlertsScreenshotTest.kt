package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.AlertSettingsDto
import com.baxner.vigia.ui.alerts.AlertsScreenContent
import com.baxner.vigia.ui.alerts.AlertsUiState
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
 * Revisión visual de la pantalla Alertas (ver el encargo, punto 6): la
 * lista con los 5 tipos del contrato (icono por tipo/severidad, no
 * leídas marcadas) y el aviso de permiso de notificaciones, y los dos
 * estados vacíos (sin alertas todavía / sin agente vinculado). No se
 * capturó la notificación de Android en sí — Roborazzi renderiza el árbol
 * de Compose de la app, no la bandeja del sistema — por eso estas capturas
 * de la lista, como permite el encargo ("si no, de la lista"). Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class AlertsScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now = Instant.parse("2026-10-07T20:00:00Z")

    private fun sampleAlerts(): List<AlertDto> = listOf(
        AlertDto(
            id = 61, type = "NEW_DEVICE", severity = "high", at = now.minus(5, ChronoUnit.MINUTES).toString(),
            title = "Equipo nuevo en tu red", body = "Telefono-Invitado (192.168.0.24) se conectó por primera vez.",
            deviceId = 31, network = "k", read = false,
        ),
        AlertDto(
            id = 60, type = "IMPORTANT_OFFLINE", severity = "medium", at = now.minus(2, ChronoUnit.HOURS).toString(),
            title = "Equipo importante desconectado", body = "Impresora HP se desconectó.",
            deviceId = 12, network = "k", read = false,
        ),
        AlertDto(
            id = 59, type = "IMPORTANT_ONLINE", severity = "low", at = now.minus(1, ChronoUnit.DAYS).toString(),
            title = "Equipo importante conectado", body = "Cámara del jardín volvió a conectarse.",
            deviceId = 8, network = "k", read = true,
        ),
        AlertDto(
            id = 58, type = "AGENT_BLIND", severity = "medium", at = now.minus(2, ChronoUnit.DAYS).toString(),
            title = "El agente dejó de ver la red", body = "Mac-Estudio no detecta equipos activos desde hace 10 min.",
            deviceId = null, network = "k", read = true,
        ),
        AlertDto(
            id = 57, type = "AGENT_RECOVERED", severity = "low", at = now.minus(3, ChronoUnit.DAYS).toString(),
            title = "El agente volvió a ver la red", body = "Mac-Estudio ya detecta equipos de nuevo.",
            deviceId = null, network = "k", read = true,
        ),
    )

    @Test
    fun con_alertas() {
        val uiState = AlertsUiState(linked = true, alerts = sampleAlerts(), unreadCount = 2, settings = AlertSettingsDto())
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                AlertsScreenContent(
                    uiState = uiState,
                    onBack = {},
                    onOpenSettings = {},
                    showNotificationBanner = true,
                    now = now,
                )
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/alertas_con_alertas.png")
    }

    @Test
    fun sin_alertas_vinculado() {
        val uiState = AlertsUiState(linked = true, alerts = emptyList(), unreadCount = 0, settings = AlertSettingsDto())
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AlertsScreenContent(uiState = uiState, onBack = {}, onOpenSettings = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/alertas_sin_alertas.png")
    }

    @Test
    fun sin_agente_vinculado() {
        val uiState = AlertsUiState(linked = false)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                AlertsScreenContent(uiState = uiState, onBack = {}, onOpenSettings = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/alertas_sin_agente.png")
    }
}
