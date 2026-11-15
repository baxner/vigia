package com.baxner.vigia.ui.security

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.filled.WarningAmber
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.FindingSeverity
import com.baxner.vigia.data.agent.SecurityDevice
import com.baxner.vigia.data.agent.OuterRing
import com.baxner.vigia.data.agent.PortMapping
import com.baxner.vigia.data.agent.PortMappingSource
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.RouterReach
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.agent.reachOrUnknown
import com.baxner.vigia.data.agent.severityOrInfo
import com.baxner.vigia.data.agent.sourceOrUnknown
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.model.PortCatalog
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.GlassSurface
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.StatusChip
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.scan.PortScanState
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.RelativeTime
import com.baxner.vigia.util.relativeTimeFrom
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import kotlinx.coroutines.flow.flowOf

/** Estado de "Tu router visto desde aquí" (ver el encargo, punto 3) para [SecurityScreenContent]. Lo arma la envoltura con estado ([SecurityScreen]) a partir de [ScanViewModel]; aquí es solo datos, para poder capturarlo sin un ViewModel real. */
sealed interface RouterFromHereUi {
    /** El teléfono no está en la Wi-Fi de la ONT (o no hay un [RouterInfo] guardado todavía): la sección no se muestra. */
    data object NotApplicable : RouterFromHereUi

    data class Visible(
        val externalIp: String,
        /** `null` = todavía no hay un escaneo local de esta red (ver el encargo: "escanea esta red para ver..."); lista vacía = se escaneó y no hay puertos abiertos. */
        val openPorts: List<Int>?,
        val portScanState: PortScanState,
        /** `true` si ya hay un [KnownDevice] para [externalIp] al que pedirle el escaneo completo (ver [ScanViewModel.scanAllPorts]). */
        val canScanAllPorts: Boolean,
        /**
         * La puerta de enlace de ESTA red (la Wi-Fi de la ONT), no la del
         * router principal: si el `RouterInfo` del informe sale null, es porque este
         * módem no contesta UPnP/NAT-PMP — no el router de casa (ver la
         * revisión de la fase 5, punto 5). `null` si no se pudo leer.
         */
        val modemGatewayIp: String? = null,
    ) : RouterFromHereUi
}

/**
 * Envoltura con estado de la pantalla Seguridad (ver el encargo, punto 3):
 * conecta [SecurityViewModel] con la [scanViewModel] compartida (equipos
 * conocidos, red actual, y el escaneo completo de puertos para "Tu router
 * visto desde aquí" — reusa [ScanViewModel.scanAllPorts]/
 * [ScanViewModel.portScan], como pide el encargo). Se llega aquí desde la
 * tile "Seguridad" del Panel o desde la nueva ruta `security` (ver
 * `ui/navigation/VigiaNavHost.kt`).
 */
@Composable
fun SecurityScreen(
    scanViewModel: ScanViewModel,
    onBack: () -> Unit,
    onOpenDevice: (Long) -> Unit,
    modifier: Modifier = Modifier,
) {
    val viewModel: SecurityViewModel = viewModel()
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val scanUiState by scanViewModel.uiState.collectAsStateWithLifecycle()
    val knownDevices by scanViewModel.knownDevices.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { viewModel.onOpen(knownDevices, scanUiState.network) }
    // "y después de cada escaneo" (ver el encargo, punto 2): cualquier cambio en los
    // equipos conocidos —de un escaneo nuevo, local o del agente— vuelve a pedir el informe.
    LaunchedEffect(knownDevices) { viewModel.onDevicesChanged(knownDevices, scanUiState.network) }

    val lastRouterIp = uiState.lastRouterInfo?.externalIp
    val routerDevice = remember(knownDevices, lastRouterIp) {
        lastRouterIp?.let { ip -> knownDevices.firstOrNull { it.last.ip == ip } }
    }
    val routerDevicePorts = remember(routerDevice, scanUiState.result, lastRouterIp) {
        routerDevice?.last?.openPorts
            ?: lastRouterIp?.let { ip -> scanUiState.result?.devices?.firstOrNull { it.ip == ip }?.openPorts }
    }
    val portScanState by remember(routerDevice) {
        routerDevice?.let { scanViewModel.portScan(it.network, it.id) } ?: flowOf(PortScanState.Idle)
    }.collectAsStateWithLifecycle(initialValue = PortScanState.Idle)

    val routerFromHere = if (lastRouterIp == null || !viewModel.routerSeenFromHere(scanUiState.network?.cidr)) {
        RouterFromHereUi.NotApplicable
    } else {
        RouterFromHereUi.Visible(
            externalIp = lastRouterIp,
            openPorts = routerDevicePorts,
            portScanState = portScanState,
            canScanAllPorts = routerDevice != null,
            modemGatewayIp = scanUiState.network?.gateway,
        )
    }

    SecurityScreenContent(
        uiState = uiState,
        onBack = onBack,
        onOpenDevice = onOpenDevice,
        modifier = modifier,
        routerFromHere = routerFromHere,
        onScanRouterFromHere = { routerDevice?.let(scanViewModel::scanAllPorts) },
        onRunLocalScan = scanViewModel::startScan,
        onAcknowledge = viewModel::acknowledge,
        onUnacknowledge = viewModel::unacknowledge,
    )
}

/** Versión sin estado (capturas, tests): ver [SecurityScreen] para la envoltura real. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecurityScreenContent(
    uiState: SecurityUiState,
    onBack: () -> Unit,
    onOpenDevice: (Long) -> Unit,
    modifier: Modifier = Modifier,
    routerFromHere: RouterFromHereUi = RouterFromHereUi.NotApplicable,
    onScanRouterFromHere: () -> Unit = {},
    onRunLocalScan: () -> Unit = {},
    onAcknowledge: (String) -> Unit = {},
    onUnacknowledge: (String) -> Unit = {},
    now: Instant = Instant.now(),
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.security_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        val report = uiState.report
        when {
            report != null -> SecurityReportBody(
                report = report,
                padding = padding,
                onOpenDevice = onOpenDevice,
                routerFromHere = routerFromHere,
                onScanRouterFromHere = onScanRouterFromHere,
                onRunLocalScan = onRunLocalScan,
                onAcknowledge = onAcknowledge,
                onUnacknowledge = onUnacknowledge,
                now = now,
            )
            uiState.loading -> Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator(color = VigiaColors.RadarCyan)
            }
            uiState.monitoredByAgent && uiState.agentUnreachable -> EmptyState(
                title = stringResource(R.string.security_empty_agent_unreachable_title),
                subtitle = stringResource(R.string.security_empty_agent_unreachable_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            else -> EmptyState(
                title = stringResource(R.string.security_empty_no_data_title),
                subtitle = stringResource(R.string.security_empty_no_data_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
        }
    }
}

@Composable
private fun SecurityReportBody(
    report: SecurityReport,
    padding: PaddingValues,
    onOpenDevice: (Long) -> Unit,
    routerFromHere: RouterFromHereUi,
    onScanRouterFromHere: () -> Unit,
    onRunLocalScan: () -> Unit,
    onAcknowledge: (String) -> Unit,
    onUnacknowledge: (String) -> Unit,
    now: Instant,
) {
    // "Ya lo sé, es a propósito" (ver el contrato § "Hallazgos reconocidos
    // v1.3"): los reconocidos no entran en los grupos por severidad — van
    // aparte, al final (ver [AcknowledgedGroup]).
    val (acknowledged, active) = remember(report.findings) { report.findings.partition { it.acknowledged } }
    val bySeverity = remember(active) { active.groupBy { it.severityOrInfo() } }
    LazyColumn(
        modifier = Modifier.fillMaxSize().padding(padding),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { SecurityHeader(report, now) }

        findingGroups.forEach { (severity, titleRes) ->
            val findings = bySeverity[severity].orEmpty() +
                (if (severity == FindingSeverity.INFO) bySeverity[FindingSeverity.UNKNOWN].orEmpty() else emptyList())
            if (findings.isNotEmpty()) {
                item(key = "findings_$severity") {
                    FindingsGroup(title = stringResource(titleRes), findings = findings, onOpenDevice = onOpenDevice, onAcknowledge = onAcknowledge)
                }
            }
        }

        if (acknowledged.isNotEmpty()) {
            item(key = "findings_acknowledged") {
                AcknowledgedGroup(findings = acknowledged, onUnacknowledge = onUnacknowledge)
            }
        }

        item(key = "router") {
            RouterCard(
                router = report.router,
                modemGatewayIp = (routerFromHere as? RouterFromHereUi.Visible)?.modemGatewayIp,
            )
        }

        report.router?.let { router ->
            item(key = "mappings") { PortMappingsCard(router.portMappings, now) }
        }

        report.outerRing?.let { ring ->
            item(key = "outer_ring") { OuterRingCard(ring) }
        }

        if (routerFromHere is RouterFromHereUi.Visible) {
            item(key = "from_here") { RouterFromHereCard(routerFromHere, onScanRouterFromHere, onRunLocalScan) }
        }
    }
}

private val findingGroups = listOf(
    FindingSeverity.HIGH to R.string.security_findings_title_high,
    FindingSeverity.MEDIUM to R.string.security_findings_title_medium,
    FindingSeverity.LOW to R.string.security_findings_title_low,
    FindingSeverity.INFO to R.string.security_findings_title_info,
)

@Composable
private fun SecurityHeader(report: SecurityReport, now: Instant) {
    val actionable = remember(report.findings) {
        report.findings.count { it.severityOrInfo() != FindingSeverity.INFO && it.severityOrInfo() != FindingSeverity.UNKNOWN }
    }
    GlassSurface(modifier = Modifier.fillMaxWidth(), glow = report.score < 50) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(18.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            SecurityScoreBadge(report.score)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = when (actionable) {
                        0 -> stringResource(R.string.security_summary_none)
                        1 -> stringResource(R.string.security_summary_one)
                        else -> stringResource(R.string.security_summary_items, actionable)
                    },
                    style = MaterialTheme.typography.titleMedium,
                    color = VigiaColors.TextPrimary,
                )
                relativeTimeFromNow(report.generatedAt, now)?.let { relative ->
                    Text(
                        stringResource(R.string.security_header_updated, agoLabel(relative)),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.TextTertiary,
                    )
                }
            }
        }
    }
}

@Composable
private fun SecurityScoreBadge(score: Int, modifier: Modifier = Modifier) {
    val color = securityScoreColor(score)
    Box(
        modifier = modifier.size(84.dp).background(color.copy(alpha = 0.14f), CircleShape).border(2.dp, color, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Text(stringResource(R.string.security_score_format, score), style = VigiaMono.bodyMedium, color = color)
    }
}

private fun securityScoreColor(score: Int): Color = when {
    score >= 80 -> VigiaColors.Ok
    score >= 50 -> VigiaColors.Amber
    else -> VigiaColors.Risk
}

@Composable
private fun FindingsGroup(title: String, findings: List<Finding>, onOpenDevice: (Long) -> Unit, onAcknowledge: (String) -> Unit) {
    SectionCard(title = title) {
        findings.forEach { finding -> FindingRow(finding, onOpenDevice, onAcknowledge) }
    }
}

@Composable
private fun FindingRow(finding: Finding, onOpenDevice: (Long) -> Unit, onAcknowledge: (String) -> Unit) {
    var expanded by rememberSaveable(finding.id) { mutableStateOf(false) }
    val severity = finding.severityOrInfo()
    val expandedStateLabel = stringResource(R.string.security_finding_action_expanded)
    val collapsedStateLabel = stringResource(R.string.security_finding_action_collapsed)
    // "Ya lo sé, es a propósito" (ver el contrato § "Hallazgos reconocidos
    // v1.3") vive siempre en el desplegable: con eso, todo hallazgo activo
    // tiene algo que mostrar al expandir, aunque no traiga action ni equipo.
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            // Fusionada (ver AGENTS.md § accesibilidad): la severidad no se
            // ve solo por el color/icono — el icono lleva la descripción de
            // la severidad ("Alta prioridad"...) y se fusiona con título y
            // detalle en una sola lectura de TalkBack.
            modifier = Modifier.semantics(mergeDescendants = true) {},
        ) {
            Icon(findingIcon(severity), contentDescription = severityLabel(severity), tint = findingColor(severity), modifier = Modifier.size(20.dp))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(finding.title, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
                Text(finding.detail, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            }
        }
        Row(
            modifier = Modifier
                .padding(start = 30.dp)
                .minimumInteractiveComponentSize()
                .clip(RoundedCornerShape(8.dp))
                .clickable(role = Role.Button) { expanded = !expanded }
                .semantics {
                    stateDescription = if (expanded) {
                        expandedStateLabel
                    } else {
                        collapsedStateLabel
                    }
                },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                stringResource(R.string.security_finding_action_title),
                style = MaterialTheme.typography.labelSmall,
                color = VigiaColors.RadarCyan,
            )
            Icon(
                if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                contentDescription = null,
                tint = VigiaColors.RadarCyan,
                modifier = Modifier.size(16.dp),
            )
        }
        if (expanded) {
            Column(modifier = Modifier.padding(start = 30.dp, top = 2.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                if (finding.action.isNotBlank()) {
                    Text(finding.action, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextPrimary)
                }
                finding.deviceId?.let { id ->
                    TextButton(onClick = { onOpenDevice(id) }, contentPadding = PaddingValues(0.dp)) {
                        Text(stringResource(R.string.security_finding_open_device), color = VigiaColors.RadarCyan, style = MaterialTheme.typography.labelMedium)
                    }
                }
                TextButton(onClick = { onAcknowledge(finding.id) }, contentPadding = PaddingValues(0.dp)) {
                    Text(stringResource(R.string.security_finding_acknowledge), color = VigiaColors.TextSecondary, style = MaterialTheme.typography.labelMedium)
                }
            }
        }
    }
}

/** "A propósito" (ver el contrato § "Hallazgos reconocidos v1.3"): los reconocidos, al final, atenuados. */
@Composable
private fun AcknowledgedGroup(findings: List<Finding>, onUnacknowledge: (String) -> Unit) {
    SectionCard(title = stringResource(R.string.security_acknowledged_title)) {
        findings.forEach { finding -> AcknowledgedFindingRow(finding, onUnacknowledge) }
    }
}

@Composable
private fun AcknowledgedFindingRow(finding: Finding, onUnacknowledge: (String) -> Unit) {
    val severity = finding.severityOrInfo()
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp).alpha(0.55f),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            modifier = Modifier.semantics(mergeDescendants = true) {},
        ) {
            Icon(findingIcon(severity), contentDescription = severityLabel(severity), tint = VigiaColors.TextTertiary, modifier = Modifier.size(20.dp))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(finding.title, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                Text(finding.detail, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            }
        }
        Row(
            modifier = Modifier.padding(start = 30.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (finding.points > 0) {
                Text(
                    pointsRestoredLabel(finding.points),
                    style = MaterialTheme.typography.labelSmall,
                    color = VigiaColors.TextTertiary,
                )
            }
            TextButton(onClick = { onUnacknowledge(finding.id) }, contentPadding = PaddingValues(0.dp)) {
                Text(stringResource(R.string.security_finding_undo_acknowledge), color = VigiaColors.RadarCyan, style = MaterialTheme.typography.labelMedium)
            }
        }
    }
}

@Composable
private fun pointsRestoredLabel(points: Int): String =
    if (points == 1) stringResource(R.string.security_finding_would_subtract_one) else stringResource(R.string.security_finding_would_subtract_other, points)

private fun findingIcon(severity: FindingSeverity): ImageVector = when (severity) {
    FindingSeverity.HIGH -> Icons.Default.Warning
    FindingSeverity.MEDIUM -> Icons.Default.WarningAmber
    FindingSeverity.LOW, FindingSeverity.INFO, FindingSeverity.UNKNOWN -> Icons.Default.Info
}

private fun findingColor(severity: FindingSeverity): Color = when (severity) {
    FindingSeverity.HIGH -> VigiaColors.Risk
    FindingSeverity.MEDIUM -> VigiaColors.Amber
    FindingSeverity.LOW -> VigiaColors.RadarCyan
    FindingSeverity.INFO, FindingSeverity.UNKNOWN -> VigiaColors.TextTertiary
}

/**
 * La severidad en texto, para TalkBack (ver AGENTS.md § accesibilidad): hoy
 * se distingue solo por el color/icono del hallazgo, así que el icono lleva
 * esta descripción — los mismos textos que ya usan los encabezados de grupo
 * ("Alta prioridad"...), para no inventar un vocabulario aparte.
 */
@Composable
private fun severityLabel(severity: FindingSeverity): String = when (severity) {
    FindingSeverity.HIGH -> stringResource(R.string.security_findings_title_high)
    FindingSeverity.MEDIUM -> stringResource(R.string.security_findings_title_medium)
    FindingSeverity.LOW -> stringResource(R.string.security_findings_title_low)
    FindingSeverity.INFO, FindingSeverity.UNKNOWN -> stringResource(R.string.security_findings_title_info)
}

/**
 * [modemGatewayIp] no nulo significa que el teléfono está en "Tu router
 * visto desde aquí" (la Wi-Fi de la ONT): ahí, el `RouterInfo` que se
 * consultó es el módem de ESA red, no el router de casa (ver la revisión de
 * la fase 5, punto 5) — "El router no contestó…" sería contradictorio justo
 * al lado de esa tarjeta, así que el mensaje aclara de cuál router se habla.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun RouterCard(router: RouterInfo?, modemGatewayIp: String? = null) {
    SectionCard(title = stringResource(R.string.security_router_title)) {
        if (router == null) {
            val message = if (modemGatewayIp != null) {
                stringResource(R.string.security_router_modem_unavailable, modemGatewayIp)
            } else {
                stringResource(R.string.security_router_unavailable)
            }
            Text(message, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
            return@SectionCard
        }
        InfoRow(label = stringResource(R.string.security_router_ip), value = router.ip)
        router.mac?.let { InfoRow(label = stringResource(R.string.security_router_mac), value = it) }
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            StatusChip(
                text = "${stringResource(R.string.security_router_upnp)} · ${activeLabel(router.upnp)}",
                dotColor = if (router.upnp) VigiaColors.Amber else VigiaColors.TextTertiary,
            )
            StatusChip(
                text = "${stringResource(R.string.security_router_natpmp)} · ${activeLabel(router.natpmp)}",
                dotColor = if (router.natpmp) VigiaColors.Amber else VigiaColors.TextTertiary,
            )
        }
        router.externalIp?.let { InfoRow(label = stringResource(R.string.security_router_external_ip), value = it) }
        Text(reachExplanation(router), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
    }
}

@Composable
private fun activeLabel(active: Boolean): String =
    stringResource(if (active) R.string.security_router_active else R.string.security_router_inactive)

@Composable
private fun reachExplanation(router: RouterInfo): String = when (router.reachOrUnknown()) {
    RouterReach.OUTER_RING -> stringResource(R.string.security_router_reach_outer_ring, subnetGuessLabel(router.externalIp))
    RouterReach.INTERNET -> stringResource(R.string.security_router_reach_internet)
    RouterReach.UNKNOWN -> stringResource(R.string.security_router_reach_unknown)
}

/** "192.168.100.2" -> "192.168.100.x" (ver el encargo, punto 3: así redacta el ejemplo del `reach` "outer_ring"). */
private fun subnetGuessLabel(ip: String?): String {
    val parts = ip?.split('.') ?: return "?"
    return if (parts.size == 4) "${parts[0]}.${parts[1]}.${parts[2]}.x" else ip
}

@Composable
private fun PortMappingsCard(mappings: List<PortMapping>, now: Instant) {
    SectionCard(title = stringResource(R.string.security_mappings_title)) {
        if (mappings.isEmpty()) {
            Text(stringResource(R.string.security_mappings_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            mappings.forEach { mapping -> PortMappingRow(mapping, now) }
        }
    }
}

@Composable
private fun PortMappingRow(mapping: PortMapping, now: Instant) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        // La regla en su propia línea completa (ver la revisión de la fase 5,
        // punto 8: con la etiqueta al lado se partía en dos por el ancho de
        // "Pedida por UPnP"); la etiqueta de origen va debajo, no al lado.
        Text(
            stringResource(R.string.security_mapping_route, "${mapping.protocol} ${mapping.externalPort}", "${mapping.internalIp}:${mapping.internalPort}"),
            style = VigiaMono.body,
            color = VigiaColors.TextPrimary,
        )
        val sourceRes = if (mapping.sourceOrUnknown() == PortMappingSource.PROBE) {
            R.string.security_mapping_source_probe
        } else {
            R.string.security_mapping_source_upnp
        }
        StatusChip(text = stringResource(sourceRes), dotColor = VigiaColors.RadarCyan)
        Text(PortCatalog.localizedNameFor(mapping.internalPort), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
        mapping.description.takeIf { it.isNotBlank() }?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
        }
        mapping.firstSeen?.let { firstSeen ->
            relativeTimeFromNow(firstSeen, now)?.let { relative ->
                Text(stringResource(R.string.security_mapping_since, agoLabel(relative)), style = VigiaMono.labelSmall, color = VigiaColors.TextTertiary)
            }
        }
    }
}

@Composable
private fun OuterRingCard(ring: OuterRing) {
    SectionCard(title = stringResource(R.string.security_outer_ring_title)) {
        if (ring.devices.isEmpty()) {
            Text(stringResource(R.string.security_outer_ring_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            // Plural correcto (ver AGENTS.md § accesibilidad): "1 equipos" se
            // leía mal con TalkBack (y a la vista) antes de este cambio.
            Text(
                pluralStringResource(R.plurals.security_outer_ring_count_plural, ring.devices.size, ring.devices.size),
                style = MaterialTheme.typography.bodyMedium,
                color = VigiaColors.TextSecondary,
            )
            ring.devices.take(MaxOuterRingRows).forEach { device -> OuterRingDeviceRow(device) }
            val remaining = ring.devices.size - MaxOuterRingRows
            if (remaining > 0) {
                Text(stringResource(R.string.common_and_more, remaining), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            }
        }
    }
}

private const val MaxOuterRingRows = 6

@Composable
private fun OuterRingDeviceRow(device: SecurityDevice) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(device.device.ip, style = VigiaMono.body, color = VigiaColors.TextPrimary)
        val name = device.name.ifBlank { device.device.vendor }.ifBlank { stringResource(R.string.devices_unidentified) }
        Text(name, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary, modifier = Modifier.weight(1f))
        if (device.device.openPorts.isNotEmpty()) {
            Text(device.device.openPorts.sorted().joinToString(", "), style = VigiaMono.labelSmall, color = VigiaColors.TextTertiary)
        }
    }
}

@Composable
private fun RouterFromHereCard(
    state: RouterFromHereUi.Visible,
    onScanAllPorts: () -> Unit,
    onRunLocalScan: () -> Unit,
) {
    SectionCard(title = stringResource(R.string.security_from_here_title)) {
        Text(stringResource(R.string.security_from_here_hint), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
        InfoRow(label = stringResource(R.string.security_router_external_ip), value = state.externalIp)

        when {
            state.openPorts == null -> {
                Text(stringResource(R.string.security_from_here_need_scan), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                Button(onClick = onRunLocalScan, colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background)) {
                    Text(stringResource(R.string.security_from_here_scan_now))
                }
            }
            state.openPorts.isEmpty() -> Text(stringResource(R.string.security_from_here_no_ports), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
            else -> state.openPorts.sorted().forEach { port ->
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("$port", style = VigiaMono.bodyMedium, color = if (PortCatalog.isRisky(port)) VigiaColors.Risk else VigiaColors.TextPrimary)
                    Text(PortCatalog.localizedNameFor(port), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                }
            }
        }

        if (state.canScanAllPorts) {
            when (val scan = state.portScanState) {
                is PortScanState.Scanning -> Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = VigiaColors.RadarCyan)
                    Text(stringResource(R.string.security_from_here_scanning), color = VigiaColors.TextSecondary)
                }
                is PortScanState.Done -> Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    OutlinedButton(onClick = onScanAllPorts) { Text(stringResource(R.string.security_from_here_scan_all_ports)) }
                    if (scan.ports.isNotEmpty()) {
                        Text(scan.ports.sorted().joinToString(", "), style = VigiaMono.labelSmall, color = VigiaColors.TextSecondary)
                    }
                }
                else -> OutlinedButton(onClick = onScanAllPorts) { Text(stringResource(R.string.security_from_here_scan_all_ports)) }
            }
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(label, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
        Spacer(Modifier.weight(1f))
        Text(value, style = VigiaMono.body, color = VigiaColors.TextSecondary)
    }
}

/** [relativeTimeFrom] con un reloj fijo (ver el encargo, "Pruebas": "Reloj fijo en las pruebas"), convertido una sola vez por fila en vez de que cada [java.time.OffsetDateTime.now] interno difiera entre capturas. */
private fun relativeTimeFromNow(rfc3339: String, now: Instant): RelativeTime? =
    relativeTimeFrom(rfc3339, OffsetDateTime.ofInstant(now, ZoneOffset.UTC))
