package com.baxner.vigia.ui.accessibility

import androidx.activity.ComponentActivity
import androidx.compose.material3.Surface
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.InternetNowDto
import com.baxner.vigia.data.agent.InternetSeriesPointDto
import com.baxner.vigia.data.agent.InternetSeriesRange
import com.baxner.vigia.data.agent.InternetSettingsDto
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.demo.DemoFixtures
import com.baxner.vigia.ui.devices.DeviceDetailScreenContent
import com.baxner.vigia.ui.devices.DevicesScreenContent
import com.baxner.vigia.ui.internet.InternetScreenContent
import com.baxner.vigia.ui.internet.InternetUiState
import com.baxner.vigia.ui.scan.PortScanState
import com.baxner.vigia.ui.security.SecurityScreenContent
import com.baxner.vigia.ui.security.SecurityUiState
import com.baxner.vigia.ui.theme.VigiaTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.Instant
import java.time.temporal.ChronoUnit

/**
 * Pruebas de semántica para accesibilidad de TalkBack (ver AGENTS.md §
 * accesibilidad), reemplazo del "solo se dibuja" anterior: comprueban de
 * verdad lo que anuncia TalkBack, no solo que la pantalla no truene.
 *
 *  - Un botón de solo ícono (Volver, en Detalle de dispositivo) tiene la
 *    acción como descripción, no `null`.
 *  - Una fila de dispositivo se lee fusionada (nombre + IP) con el estado
 *    "en línea"/"sin conexión" en [SemanticsProperties.StateDescription],
 *    no solo por color.
 *  - La severidad de un hallazgo de Seguridad está en texto (la descripción
 *    del icono), no solo en su color.
 *  - La descripción de la gráfica de latencia de Internet dice el rango que
 *    se está viendo (24 h o 7 días), no "última hora" fijo.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = "es")
class SemanticAccessibilityTest {
    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private val now: Instant = Instant.parse("2026-10-11T15:00:00Z")
    private fun ago(minutes: Long) = now.minus(minutes, ChronoUnit.MINUTES).toString()

    /** Botón de solo ícono: "Volver" en la barra de Detalle, no `contentDescription = null`. */
    @Test
    fun backButton_hasActionDescription() {
        composeTestRule.setContent {
            VigiaTheme {
                Surface {
                    DeviceDetailScreenContent(
                        device = null,
                        events = emptyList(),
                        portScanState = PortScanState.Idle,
                        onBack = {},
                        onEdit = {},
                        onScanAllPorts = {},
                        onForget = {},
                    )
                }
            }
        }

        composeTestRule.onNodeWithContentDescription("Volver").assertHasClickAction()
    }

    /** Fila de dispositivo fusionada: nombre + IP en un solo nodo, con el estado "En línea" (no solo por color). */
    @Test
    fun deviceRow_isMergedWithOnlineStateDescription() {
        val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
        val device = DemoFixtures.scanResult.devices.first()
        val onlineDevice = KnownDevice(
            id = 1L,
            network = network,
            mac = device.mac.ifEmpty { null },
            lastIp = device.ip,
            firstSeen = now.minus(1, ChronoUnit.DAYS),
            lastSeen = now,
            online = true,
            onlineSince = now.minus(1, ChronoUnit.DAYS),
            last = device,
        )

        composeTestRule.setContent {
            VigiaTheme {
                Surface {
                    DevicesScreenContent(
                        knownDevices = listOf(onlineDevice),
                        gateway = "192.168.0.1",
                        onDeviceClick = {},
                        phoneIp = "192.168.0.50",
                        now = now,
                    )
                }
            }
        }

        // Un solo nodo clickable con el estado "En línea" en stateDescription...
        val row = composeTestRule.onNode(
            hasClickAction() and SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "En línea"),
        )
        row.assertHasClickAction()
        // ...y que de verdad fusiona el texto de la fila (la IP), no solo el nombre.
        row.assertTextContains(device.ip, substring = true)
    }

    /** La severidad de un hallazgo de Seguridad está en texto, no solo en el color/icono. */
    @Test
    fun securityFinding_severityIsInText() {
        val finding = Finding(
            id = "RISKY_SERVICE|192.168.0.55|445",
            kind = "RISKY_SERVICE",
            severity = "high",
            points = 10,
            title = "Una computadora comparte archivos con toda la red",
            detail = "PC-Escritorio (192.168.0.55) tiene abierto SMB (445).",
        )
        val report = SecurityReport(generatedAt = ago(2), score = 70, findings = listOf(finding))
        val uiState = SecurityUiState(report = report, monitoredByAgent = true)

        composeTestRule.setContent {
            VigiaTheme {
                Surface {
                    SecurityScreenContent(uiState = uiState, onBack = {}, onOpenDevice = {}, now = now)
                }
            }
        }

        // "Alta prioridad" es tanto el título del grupo como la descripción del
        // icono de severidad de este hallazgo (ver SecurityScreen.severityLabel).
        composeTestRule.onNodeWithContentDescription("Alta prioridad").assertExists()
    }

    /** La gráfica de latencia dice el rango que se está viendo: 24 h primero, luego 7 días. */
    @Test
    fun latencyChart_descriptionMatchesSelectedRange() {
        val series = (0 until 10).map { i ->
            InternetSeriesPointDto(at = ago((9 - i) * 30L), routerMs = 3.0, modemMs = 4.0, internetMs = 16.0 + i, dnsMs = 10.0, lossPct = 0.0)
        }
        val status = InternetStatusDto(
            state = "up",
            now = InternetNowDto(routerMs = 3.0, modemMs = 4.0, internetMs = 18.0, dnsMs = 10.0, lossPct = 0.0),
            ipv6 = "ok",
        )
        val baseState = InternetUiState(
            linked = true,
            monitoring = true,
            status = status,
            series = series,
            settings = InternetSettingsDto(speedTestsPerDay = 2),
        )

        // Un solo `setContent` (no se puede llamar dos veces por prueba): el
        // rango vive en un `mutableStateOf` que la prueba cambia después,
        // igual que lo haría el selector real al tocar el chip.
        lateinit var range: androidx.compose.runtime.MutableState<InternetSeriesRange>
        composeTestRule.setContent {
            range = androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(InternetSeriesRange.HOURS_24) }
            VigiaTheme {
                Surface {
                    InternetScreenContent(uiState = baseState.copy(seriesRange = range.value), onBack = {}, now = now)
                }
            }
        }
        composeTestRule.onNode(hasContentDescriptionContaining("últimas 24 horas")).assertExists()

        range.value = InternetSeriesRange.DAYS_7
        composeTestRule.waitForIdle()
        composeTestRule.onNode(hasContentDescriptionContaining("últimos 7 días")).assertExists()
    }

    /** `onNodeWithContentDescription` exige igualdad exacta; esta pantalla arma la descripción con mínimos/máximos que no vale la pena fijar a mano en la prueba. */
    private fun hasContentDescriptionContaining(substring: String): SemanticsMatcher =
        SemanticsMatcher("ContentDescription contains '$substring'") { node ->
            node.config.getOrNull(SemanticsProperties.ContentDescription)?.any { it.contains(substring) } == true
        }
}
