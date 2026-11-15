package com.baxner.vigia.screenshot

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import com.baxner.vigia.data.agent.InternetNowDto
import com.baxner.vigia.data.agent.InternetSeriesPointDto
import com.baxner.vigia.data.agent.InternetSeriesRange
import com.baxner.vigia.data.agent.InternetSettingsDto
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.OutageDto
import com.baxner.vigia.data.agent.SpeedTestDto
import com.baxner.vigia.ui.internet.InternetScreenContent
import com.baxner.vigia.ui.internet.InternetUiState
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
 * Revisión visual de la pantalla Internet (fase 7, ver `docs/plan.md` §
 * "Fase 7"): funcionando bien con su gráfica de 24 h, con una caída en
 * curso, y con IPv6 roto. Regenerar con:
 *
 *   cd android && ./gradlew :app:testDebugUnitTest
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.MediumPhone)
class InternetScreenshotTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now = Instant.parse("2026-10-11T15:00:00Z")
    private fun ago(minutes: Long) = now.minus(minutes, ChronoUnit.MINUTES).toString()
    private fun agoHours(hours: Long) = now.minus(hours, ChronoUnit.HOURS).toString()

    /** 24 h de la serie por minuto, con un hueco de 10 min a mitad de camino (ver el encargo: "huecos donde no hay dato"). */
    private fun seriesWithGap(): List<InternetSeriesPointDto> =
        (0 until 48).map { i ->
            val minutesAgo = (47 - i) * 30L
            val gapped = i in 20..22
            InternetSeriesPointDto(
                at = ago(minutesAgo),
                routerMs = if (gapped) null else 3.0 + (i % 5),
                modemMs = if (gapped) null else 4.0 + (i % 4),
                internetMs = if (gapped) null else 16.0 + (i % 10) * 1.5,
                dnsMs = if (gapped) null else 10.0,
                lossPct = 0.0,
            )
        }

    @Test
    fun internet_funcionando_bien() {
        val status = InternetStatusDto(
            state = "up",
            since = agoHours(6),
            now = InternetNowDto(routerMs = 3.1, modemMs = 4.0, internetMs = 18.2, dnsMs = 12.0, lossPct = 0.0),
            ipv6 = "ok",
            outages = listOf(
                OutageDto(start = ago(60 * 20), end = ago(60 * 20 - 3), durationSec = 180, blame = "isp"),
            ),
            speedTests = listOf(
                SpeedTestDto(at = ago(60 * 3), downMbps = 95.4, upMbps = 18.2, latencyMs = 11.0, loadedLatencyMs = 34.0),
                SpeedTestDto(at = ago(60 * 15), downMbps = 93.1, upMbps = 17.8, latencyMs = 12.0, loadedLatencyMs = 36.0),
            ),
        )
        val uiState = InternetUiState(
            linked = true,
            monitoring = true,
            status = status,
            series = seriesWithGap(),
            seriesRange = InternetSeriesRange.HOURS_24,
            settings = InternetSettingsDto(speedTestsPerDay = 2),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1500.dp) {
                InternetScreenContent(uiState = uiState, onBack = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/internet_funcionando_bien.png")
    }

    @Test
    fun internet_caida_en_curso() {
        val status = InternetStatusDto(
            state = "down",
            since = ago(12),
            now = InternetNowDto(routerMs = 2.8, modemMs = 3.9, internetMs = null, dnsMs = null, lossPct = 100.0),
            ipv6 = "ok",
            outages = listOf(
                OutageDto(start = ago(12), end = null, durationSec = 0, blame = "isp"),
                OutageDto(start = ago(60 * 24 + 20), end = ago(60 * 24), durationSec = 20 * 60, blame = "modem"),
                OutageDto(start = ago(60 * 48 + 5), end = ago(60 * 48 + 2), durationSec = 3 * 60, blame = "lan"),
            ),
            speedTests = emptyList(),
        )
        val uiState = InternetUiState(
            linked = true,
            monitoring = true,
            status = status,
            series = emptyList(),
            seriesRange = InternetSeriesRange.HOURS_24,
            settings = InternetSettingsDto(speedTestsPerDay = 2),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1300.dp) {
                InternetScreenContent(uiState = uiState, onBack = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/internet_caida_en_curso.png")
    }

    @Test
    // El contenido (cabecera + gráfica + caídas + velocidad + nota de IPv6)
    // no cabe en un teléfono normal (mismo patrón que `SecurityScreenshotTest`,
    // "seguridad_con_agente"): hay que agrandar el qualifier de alto del
    // dispositivo con `+h`, no solo el Box de [VigiaScreenshotFrame] — ese
    // Box se mide dentro de la ventana real de la prueba, así que no puede
    // pedir más alto del que el dispositivo simulado tiene.
    @Config(qualifiers = "+h1700dp")
    fun internet_ipv6_roto() {
        val status = InternetStatusDto(
            state = "up",
            since = agoHours(10),
            now = InternetNowDto(routerMs = 2.5, modemMs = 3.5, internetMs = 15.0, dnsMs = 9.0, lossPct = 0.0),
            ipv6 = "broken",
            outages = emptyList(),
            speedTests = listOf(SpeedTestDto(at = ago(60 * 5), downMbps = 88.0, upMbps = 16.0, latencyMs = 10.0, loadedLatencyMs = 30.0)),
        )
        val uiState = InternetUiState(
            linked = true,
            monitoring = true,
            status = status,
            series = seriesWithGap(),
            seriesRange = InternetSeriesRange.HOURS_24,
            settings = InternetSettingsDto(speedTestsPerDay = 2),
        )
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 1650.dp) {
                InternetScreenContent(uiState = uiState, onBack = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/internet_ipv6_roto.png")
    }

    @Test
    fun internet_sin_agente() {
        val uiState = InternetUiState(linked = false, monitoring = false)
        composeTestRule.mainClock.autoAdvance = false
        composeTestRule.setContent {
            VigiaScreenshotFrame(height = 400.dp) {
                InternetScreenContent(uiState = uiState, onBack = {}, now = now)
            }
        }
        composeTestRule.mainClock.advanceTimeBy(1_000)
        composeTestRule.onRoot().captureRoboImage("build/outputs/roborazzi/internet_sin_agente.png")
    }
}
