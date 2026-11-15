package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.agent.WebhookDto
import com.baxner.vigia.ui.agent.AddWebhookFormFields
import com.baxner.vigia.ui.agent.AddWebhookFormState
import com.baxner.vigia.ui.agent.WebhooksScreenContent
import com.baxner.vigia.ui.agent.WebhooksUiState
import com.baxner.vigia.ui.components.SectionCard
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Revisión visual de "Webhooks" en Ajustes del agente (ver el encargo de la
 * fase 8, punto 2: lista, vacía y el formulario). Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class WebhooksScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private fun sampleWebhooks(): List<WebhookDto> = listOf(
        WebhookDto(
            id = 3, url = "http://homeassistant.local:8123/api/webhook/vigia",
            types = listOf("NEW_DEVICE", "INTERNET_DOWN"), lastStatus = 200,
        ),
        WebhookDto(id = 4, url = "https://ejemplo.com/webhooks/vigia", types = emptyList(), lastError = "timeout"),
    )

    @Test
    fun con_webhooks() {
        val uiState = WebhooksUiState(linked = true, webhooks = sampleWebhooks())
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame {
                WebhooksScreenContent(uiState = uiState, onBack = {}, onAddClick = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/webhooks_con_webhooks.png")
    }

    @Test
    fun sin_webhooks_vinculado() {
        val uiState = WebhooksUiState(linked = true, webhooks = emptyList())
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 320.dp) {
                WebhooksScreenContent(uiState = uiState, onBack = {}, onAddClick = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/webhooks_sin_webhooks.png")
    }

    @Test
    fun sin_agente_vinculado() {
        val uiState = WebhooksUiState(linked = false)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 260.dp) {
                WebhooksScreenContent(uiState = uiState, onBack = {}, onAddClick = {})
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/webhooks_sin_agente.png")
    }

    @Test
    fun formulario_agregar() {
        // Los campos del formulario, sin el `AlertDialog` que los envuelve
        // en la app real (ver `AddWebhookFormFields` en `WebhooksScreen.kt`):
        // un `AlertDialog` abre su propia ventana con una animación de
        // entrada que Robolectric + Roborazzi (GraphicsMode.NATIVE) nunca
        // asienta (AppNotIdleException) — por eso tampoco se capturan los
        // otros `AlertDialog` de la app (Alertas, Ajustes del agente). El
        // contenido es el mismo; solo cambia el marco alrededor.
        val form = AddWebhookFormState(
            url = "https://homeassistant.local:8123/api/webhook/vigia",
            selectedTypes = setOf("NEW_DEVICE", "INTERNET_DOWN"),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 700.dp) {
                SectionCard(title = "Nuevo webhook", modifier = Modifier.padding(16.dp)) {
                    AddWebhookFormFields(
                        form = form,
                        onUrlChange = {},
                        onSecretChange = {},
                        onToggleType = { _, _ -> },
                    )
                }
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/webhooks_formulario.png")
    }
}
