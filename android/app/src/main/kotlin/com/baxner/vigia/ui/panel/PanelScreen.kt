package com.baxner.vigia.ui.panel

import android.text.format.DateFormat
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
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Router
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.agent.SharedPrefsAlertsStore
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.PortCatalog
import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.channelInfoForFrequencyMhz
import com.baxner.vigia.data.network.rssiToBars
import com.baxner.vigia.ui.agent.AgentCard
import com.baxner.vigia.ui.agent.AgentMonitoredBadge
import com.baxner.vigia.ui.agent.AgentRemoteBadge
import com.baxner.vigia.ui.alerts.AlertsViewModel
import com.baxner.vigia.ui.components.AnimatedCountText
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.SignalBars
import com.baxner.vigia.ui.components.StatTile
import com.baxner.vigia.ui.components.StatusChip
import com.baxner.vigia.ui.components.rememberReduceMotion
import com.baxner.vigia.ui.components.vigiaGlow
import com.baxner.vigia.ui.devices.effectiveCoarseType
import com.baxner.vigia.ui.devices.effectiveDisplayName
import com.baxner.vigia.ui.devices.icon
import com.baxner.vigia.ui.devices.label
import com.baxner.vigia.ui.internet.InternetPanelSummary
import com.baxner.vigia.ui.internet.InternetViewModel
import com.baxner.vigia.ui.internet.toPanelSummary
import com.baxner.vigia.ui.permissions.PermissionsState
import com.baxner.vigia.ui.permissions.hasNotificationPermission
import com.baxner.vigia.ui.permissions.rememberNotificationPermissionRequester
import com.baxner.vigia.ui.scan.ScanStatus
import com.baxner.vigia.ui.scan.ScanUiState
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.security.SecurityViewModel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.RelativeTime
import com.baxner.vigia.util.formatClockTime
import com.baxner.vigia.util.formatDurationNanos
import com.baxner.vigia.util.relativeTimeFrom
import kotlinx.coroutines.delay
import java.time.Instant

/**
 * Envoltura fina que conecta el [ScanViewModel] real con [PanelScreenContent].
 * Separados para poder previsualizar/capturar el contenido con datos de
 * muestra sin construir un ScanViewModel real (su repositorio carga el
 * motor nativo, que no corre en una prueba JVM/Robolectric).
 */
@Composable
fun PanelScreen(
    scanViewModel: ScanViewModel,
    permissionsState: PermissionsState,
    onRequestPermissions: () -> Unit,
    onOpenDevices: () -> Unit,
    modifier: Modifier = Modifier,
    demoMode: Boolean = false,
    onOpenAgentPairing: () -> Unit = {},
    onOpenAgentSettings: () -> Unit = {},
    onOpenAlerts: () -> Unit = {},
    onOpenSecurity: () -> Unit = {},
    onOpenInternet: () -> Unit = {},
) {
    val uiState by scanViewModel.uiState.collectAsStateWithLifecycle()
    val knownDevices by scanViewModel.knownDevices.collectAsStateWithLifecycle()
    val agentState by scanViewModel.agentPanelState.collectAsStateWithLifecycle()
    // Informe de seguridad para la tile (ver el encargo, punto 5): independiente
    // del ScanViewModel, mismo motivo que AlertsViewModel arriba — solo lee el
    // caché compartido de SecurityRepository y lo refresca después de cada
    // escaneo (nunca al abrir el Panel: la Wi-Fi solo se sondea al abrir Seguridad).
    val securityViewModel: SecurityViewModel = viewModel()
    val securityUiState by securityViewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(knownDevices) {
        if (!demoMode) securityViewModel.onDevicesChanged(knownDevices, scanViewModel.uiState.value.network)
    }
    // Fila "Internet · 18 ms · bien" (fase 7, ver docs/plan.md § "Fase 7",
    // y `ui/agent/AgentCard.kt"): se refresca al entrar a "Red monitoreada"
    // y cada ~30 s mientras el Panel está visible (ver el encargo, punto 3).
    val internetViewModel: InternetViewModel = viewModel()
    val internetUiState by internetViewModel.uiState.collectAsStateWithLifecycle()
    val monitoringForInternet = (agentState as? AgentPanelState.Linked)?.let {
        it.monitoringCurrentNetwork || it.viewingRemote
    } == true
    LaunchedEffect(monitoringForInternet) {
        if (demoMode || !monitoringForInternet) return@LaunchedEffect
        while (true) {
            internetViewModel.refreshStatus()
            delay(30_000)
        }
    }
    // Reloj de la tarjeta del agente: "hace X min" avanza solo, y si el
    // agente deja de escanear se nota en vez de quedarse en "hace un momento".
    val now by produceState(Instant.now()) {
        while (true) {
            delay(30_000)
            value = Instant.now()
        }
    }
    // Independiente del ScanViewModel a propósito (ver AlertsRepository): el
    // contador de no leídas no depende de que esta red sea "Red monitoreada".
    val alertsViewModel: AlertsViewModel = viewModel()
    val unreadAlerts by alertsViewModel.uiState.collectAsStateWithLifecycle()
    // Permiso de notificaciones: se pide una sola vez, la primera vez que el
    // Panel muestra al agente vigilando esta red (el momento en que se
    // entiende para qué); después queda el aviso en la pantalla Alertas.
    val context = LocalContext.current
    val requestNotifications = rememberNotificationPermissionRequester()
    val monitoring = (agentState as? AgentPanelState.Linked)?.monitoringCurrentNetwork == true
    LaunchedEffect(monitoring) {
        if (!monitoring || demoMode || hasNotificationPermission(context)) return@LaunchedEffect
        val store = SharedPrefsAlertsStore(context)
        if (store.notificationPermissionAsked()) return@LaunchedEffect
        store.setNotificationPermissionAsked()
        requestNotifications()
    }
    // Solo los equipos que de verdad tienen algo personalizado: el nombre y
    // el tipo elegidos a mano mandan sobre los del motor también en el
    // radar y las tarjetas del Panel, no solo en Dispositivos y Detalle.
    val customByIp = remember(knownDevices) {
        knownDevices
            .filter { it.custom.name != null || it.custom.typeId != null }
            .associate { it.lastIp to it.custom }
    }
    // Los mismos equipos "en línea" que la lista de Dispositivos (historial
    // local o del agente): si no, el Panel contaba el último escaneo del
    // teléfono y la lista lo del agente, y no coincidían.
    val onlineDevices = remember(knownDevices) { knownDevices.filter { it.online }.map { it.last } }
    PanelScreenContent(
        uiState = uiState,
        onlineDevices = onlineDevices,
        permissionsState = permissionsState,
        onRequestPermissions = onRequestPermissions,
        onScan = scanViewModel::startScan,
        onCancel = scanViewModel::cancelScan,
        onOpenDevices = onOpenDevices,
        modifier = modifier,
        demoMode = demoMode,
        customByIp = customByIp,
        agentState = agentState,
        onOpenAgentPairing = onOpenAgentPairing,
        onOpenAgentSettings = onOpenAgentSettings,
        now = now,
        unreadAlerts = unreadAlerts.unreadCount,
        onOpenAlerts = onOpenAlerts,
        securityScore = securityUiState.report?.score,
        onOpenSecurity = onOpenSecurity,
        internetSummary = internetUiState.status?.toPanelSummary(),
        onOpenInternet = onOpenInternet,
    )
}

@Composable
fun PanelScreenContent(
    uiState: ScanUiState,
    permissionsState: PermissionsState,
    onRequestPermissions: () -> Unit,
    onScan: () -> Unit,
    onCancel: () -> Unit,
    onOpenDevices: () -> Unit,
    modifier: Modifier = Modifier,
    /** Equipos en línea según el historial; vacío = se usa el último escaneo (demo, capturas). */
    onlineDevices: List<Device> = emptyList(),
    demoMode: Boolean = false,
    customByIp: Map<String, DeviceCustomization> = emptyMap(),
    agentState: AgentPanelState = AgentPanelState.NotLinked,
    onOpenAgentPairing: () -> Unit = {},
    onOpenAgentSettings: () -> Unit = {},
    now: Instant = Instant.now(),
    unreadAlerts: Int = 0,
    onOpenAlerts: () -> Unit = {},
    /** Puntaje del último informe de seguridad cacheado; `null` = sin informe, la tile de en medio se queda como "Riesgos" (ver el encargo, punto 5). */
    securityScore: Int? = null,
    onOpenSecurity: () -> Unit = {},
    /** Resumen de internet para la fila de la tarjeta del agente (fase 7); `null` = sin dato todavía, la fila no se dibuja. */
    internetSummary: InternetPanelSummary? = null,
    onOpenInternet: () -> Unit = {},
) {
    val scanning = uiState.status is ScanStatus.Scanning
    val devices = onlineDevices.ifEmpty { uiState.result?.devices.orEmpty() }
    val monitored = (agentState as? AgentPanelState.Linked)?.monitoringCurrentNetwork == true
    // Viendo la casa remoto por Tailscale (ver el encargo, fase 8): el radar,
    // las tiles y la lista de abajo ya no son el escaneo de ESTA red (ver
    // ScanViewModel, que cambia de repositorio solo) — el encabezado tiene
    // que decirlo, si no el nombre de la red actual (SSID/CIDR) engaña.
    val viewingRemote = (agentState as? AgentPanelState.Linked)?.viewingRemote == true

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (!demoMode && !permissionsState.allGranted) {
            item(key = "permissions") {
                PermissionsCard(permissionsState = permissionsState, onRequest = onRequestPermissions)
            }
        }

        item(key = "header") {
            NetworkHeader(network = uiState.network, status = uiState.status, result = uiState.result, monitored = monitored, viewingRemote = viewingRemote)
        }

        item(key = "radar") {
            RadarHero(
                devices = devices,
                scanning = scanning,
                onClick = onOpenDevices,
                reduceMotion = rememberReduceMotion(),
                modifier = Modifier.padding(top = 4.dp),
                customByIp = customByIp,
            )
        }

        item(key = "tiles") {
            StatTilesRow(
                devices = devices,
                network = uiState.network,
                securityScore = securityScore,
                onOpenSecurity = onOpenSecurity,
            )
        }

        if (!demoMode) {
            item(key = "agent") {
                AgentCard(
                    state = agentState,
                    onLinkClick = onOpenAgentPairing,
                    onManageClick = onOpenAgentSettings,
                    now = now,
                    unreadAlerts = unreadAlerts,
                    onOpenAlerts = onOpenAlerts,
                    internetSummary = internetSummary,
                    onOpenInternet = onOpenInternet,
                )
            }
        }

        item(key = "scan_action") {
            ScanActionCard(
                status = uiState.status,
                canScan = uiState.network?.hasUsableNetwork == true || viewingRemote,
                onScan = onScan,
                onCancel = onCancel,
            )
        }

        uiState.errorMessage?.let { message ->
            item(key = "error") {
                SectionCard(title = stringResource(R.string.panel_scan_error_title)) {
                    Text(stringResource(R.string.panel_scan_error, message), color = VigiaColors.Risk)
                }
            }
        }

        if (devices.isNotEmpty()) {
            item(key = "attention") { AttentionCard(devices, customByIp) }
            item(key = "by_type") { ByTypeCard(devices, customByIp) }
            if (!viewingRemote) {
                item(key = "network_card") { NetworkDetailCard(network = uiState.network) }
            }
        }
        uiState.result?.let { result ->
            item(key = "last_scan") { LastScanFooter(result) }
        }
    }
}

/** SSID grande, chip de estado ("Escaneando… etapa" o "hace 2 min"), la insignia de Red monitoreada o de casa remota (ver el encargo, punto 5 y fase 8) e IP/router en mono. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun NetworkHeader(network: CurrentNetwork?, status: ScanStatus, result: ScanResult?, monitored: Boolean = false, viewingRemote: Boolean = false) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            text = network?.ssid ?: network?.cidr?.substringBefore('/') ?: stringResource(R.string.panel_not_connected),
            style = MaterialTheme.typography.headlineMedium,
            color = VigiaColors.TextPrimary,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
            itemVerticalAlignment = Alignment.CenterVertically,
        ) {
            StatusChip(
                text = statusChipText(status, result),
                dotColor = if (status is ScanStatus.Scanning) VigiaColors.RadarCyan else VigiaColors.TextSecondary,
            )
            // Nunca los dos a la vez (ver AgentPanelState.Linked.viewingRemote):
            // viendo remoto, esta red no es la que vigila el agente.
            when {
                viewingRemote -> AgentRemoteBadge()
                monitored -> AgentMonitoredBadge()
            }
        }
        if (network?.hasUsableNetwork == true) {
            val ip = network.cidr?.substringBefore('/') ?: "?"
            val gateway = network.gateway
            val text = buildString {
                append(stringResource(R.string.panel_ip_label)); append(' '); append(ip)
                if (!gateway.isNullOrBlank()) {
                    append("   ·   "); append(stringResource(R.string.panel_gateway)); append(' '); append(gateway)
                }
            }
            Text(text, style = VigiaMono.label, color = VigiaColors.TextSecondary)
        }
        if (network?.vpnActive == true) {
            Text(
                stringResource(R.string.panel_vpn_note),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextTertiary,
            )
        }
    }
}

@Composable
private fun statusChipText(status: ScanStatus, result: ScanResult?): String = when (status) {
    is ScanStatus.Scanning -> stageLabel(status)
    ScanStatus.Idle -> {
        val relative = result?.let { relativeTimeFrom(it.startedAt) }
        when (relative) {
            null -> stringResource(R.string.panel_status_never_scanned)
            RelativeTime.JustNow -> stringResource(R.string.panel_time_now)
            is RelativeTime.Seconds -> stringResource(
                R.string.panel_status_relative,
                stringResource(R.string.panel_time_seconds, relative.value.toInt()),
            )
            is RelativeTime.Minutes -> stringResource(
                R.string.panel_status_relative,
                stringResource(R.string.panel_time_minutes, relative.value.toInt()),
            )
            is RelativeTime.Hours -> stringResource(
                R.string.panel_status_relative,
                stringResource(R.string.panel_time_hours, relative.value.toInt()),
            )
            is RelativeTime.Days -> stringResource(
                R.string.panel_status_relative,
                stringResource(R.string.panel_time_days, relative.value.toInt()),
            )
        }
    }
}

@Composable
private fun PermissionsCard(permissionsState: PermissionsState, onRequest: () -> Unit) {
    SectionCard(title = stringRes(R.string.permissions_title)) {
        Text(stringRes(R.string.permissions_location_reason), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        if (!permissionsState.hasNearbyWifiDevices) {
            Text(stringRes(R.string.permissions_nearby_reason), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        }
        Text(
            stringRes(R.string.permissions_partial_hint),
            style = MaterialTheme.typography.bodySmall,
            color = VigiaColors.TextTertiary,
        )
        Button(onClick = onRequest, colors = vigiaButtonColors()) { Text(stringRes(R.string.permissions_grant)) }
    }
}

/**
 * Equipos / Riesgos-o-Seguridad / Señal: las tres tiles debajo del radar.
 * Con [securityScore] disponible (ver el encargo, punto 5), la tile de en
 * medio deja de contar puertos de riesgo y muestra ese puntaje bajo la
 * etiqueta "Seguridad"; tocarla abre la pantalla Seguridad
 * ([onOpenSecurity]). Sin puntaje, se queda exactamente como estaba
 * (contando [PortCatalog.riskyAmong], sin reaccionar a toques).
 */
@Composable
private fun StatTilesRow(
    devices: List<Device>,
    network: CurrentNetwork?,
    securityScore: Int? = null,
    onOpenSecurity: () -> Unit = {},
) {
    val deviceCount = devices.size
    val riskyCount = devices.sumOf { PortCatalog.riskyAmong(it.openPorts).size }

    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        StatTile(
            label = stringResource(R.string.panel_tile_devices),
            modifier = Modifier.weight(1f),
            value = { AnimatedCountText(deviceCount, style = MaterialTheme.typography.headlineSmall, color = VigiaColors.RadarCyan) },
        )
        if (securityScore != null) {
            StatTile(
                label = stringResource(R.string.panel_tile_security),
                modifier = Modifier.weight(1f),
                onClick = onOpenSecurity,
                value = {
                    AnimatedCountText(
                        securityScore,
                        style = MaterialTheme.typography.headlineSmall,
                        color = securityScoreTileColor(securityScore),
                    )
                },
            )
        } else {
            StatTile(
                label = stringResource(R.string.panel_tile_risks),
                modifier = Modifier.weight(1f),
                value = {
                    AnimatedCountText(
                        riskyCount,
                        style = MaterialTheme.typography.headlineSmall,
                        color = if (riskyCount > 0) VigiaColors.Risk else VigiaColors.RadarCyan,
                    )
                },
            )
        }
        StatTile(
            label = stringResource(R.string.panel_tile_signal),
            modifier = Modifier.weight(1f),
            value = {
                val rssi = network?.rssi
                if (rssi != null) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        SignalBars(level = rssiToBars(rssi))
                        Text("$rssi dBm", style = VigiaMono.labelSmall, color = VigiaColors.TextSecondary)
                    }
                } else {
                    Text("—", style = MaterialTheme.typography.headlineSmall, color = VigiaColors.TextTertiary)
                }
            },
        )
    }
}

/** Mismos 3 cortes que la cabecera de Seguridad (ver `ui/security/SecurityScreen.kt`, `securityScoreColor`): verde ≥ 80, ámbar 50–79, rojo < 50. Duplicado a propósito — 3 líneas, no vale la pena acoplar los dos archivos por esto. */
private fun securityScoreTileColor(score: Int): androidx.compose.ui.graphics.Color = when {
    score >= 80 -> VigiaColors.Ok
    score >= 50 -> VigiaColors.Amber
    else -> VigiaColors.Risk
}

@Composable
private fun ScanActionCard(
    status: ScanStatus,
    canScan: Boolean,
    onScan: () -> Unit,
    onCancel: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        when (status) {
            is ScanStatus.Idle -> {
                Button(
                    onClick = onScan,
                    enabled = canScan,
                    modifier = Modifier.fillMaxWidth().height(52.dp).vigiaGlow(alpha = if (canScan) 0.28f else 0f),
                    colors = vigiaButtonColors(),
                    shape = MaterialTheme.shapes.large,
                ) {
                    Text(stringRes(R.string.panel_scan_button), fontWeight = FontWeight.SemiBold)
                }
            }
            is ScanStatus.Scanning -> {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
                        color = VigiaColors.RadarCyan,
                        trackColor = VigiaColors.Outline,
                    )
                    Text(stageLabel(status), color = VigiaColors.TextPrimary)
                }
                if (status.total > 0) {
                    LinearProgressIndicator(
                        progress = { (status.done.toFloat() / status.total.toFloat()).coerceIn(0f, 1f) },
                        modifier = Modifier.fillMaxWidth().height(4.dp),
                        color = VigiaColors.RadarCyan,
                        trackColor = VigiaColors.Outline,
                    )
                } else {
                    LinearProgressIndicator(
                        modifier = Modifier.fillMaxWidth().height(4.dp),
                        color = VigiaColors.RadarCyan,
                        trackColor = VigiaColors.Outline,
                    )
                }
                OutlinedButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) {
                    Text(stringRes(R.string.panel_cancel))
                }
            }
        }
    }
}

@Composable
private fun vigiaButtonColors() = ButtonDefaults.buttonColors(
    containerColor = VigiaColors.RadarCyan,
    contentColor = VigiaColors.Background,
    disabledContainerColor = VigiaColors.Outline,
    disabledContentColor = VigiaColors.TextTertiary,
)

/** "Identificando dispositivos 7/22", a partir de la etapa que reporta el motor. */
@Composable
private fun stageLabel(status: ScanStatus.Scanning): String {
    val name = when (status.stage) {
        "sweep" -> stringRes(R.string.panel_stage_sweep)
        "names" -> stringRes(R.string.panel_stage_names)
        "identify" -> stringRes(R.string.panel_stage_identify)
        "agent" -> stringRes(R.string.panel_stage_agent)
        else -> stringRes(R.string.panel_scanning)
    }
    return if (status.stage == "identify" && status.total > 0) "$name ${status.done}/${status.total}" else name
}

@Composable
private fun AttentionCard(devices: List<Device>, customByIp: Map<String, DeviceCustomization> = emptyMap()) {
    data class Finding(val device: Device, val port: Int, val serviceName: String, val reason: String)

    val findings = devices.flatMap { device ->
        PortCatalog.riskyAmong(device.openPorts).map { risky ->
            Finding(device, risky.port, risky.serviceName, stringRes(risky.reasonRes))
        }
    }

    SectionCard(title = stringRes(R.string.panel_attention_title), glow = findings.isNotEmpty()) {
        if (findings.isEmpty()) {
            Text(stringRes(R.string.panel_risky_ports_none), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            findings.forEach { finding ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Icon(Icons.Default.Warning, contentDescription = null, tint = VigiaColors.Risk, modifier = Modifier.size(20.dp))
                    Column {
                        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            Text(finding.device.effectiveDisplayName(customByIp), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
                            Text("· ${finding.port} (${finding.serviceName})", style = VigiaMono.label, color = VigiaColors.Risk)
                        }
                        Text(finding.reason, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                    }
                }
            }
        }
    }
}

@Composable
private fun ByTypeCard(devices: List<Device>, customByIp: Map<String, DeviceCustomization> = emptyMap()) {
    SectionCard(title = stringRes(R.string.panel_by_type_title)) {
        val counts = devices
            .groupingBy { it.effectiveCoarseType(customByIp) }
            .eachCount()
            .entries
            .sortedByDescending { it.value }

        if (counts.isEmpty()) {
            Text(stringRes(R.string.devices_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        }
        counts.forEach { (type, count) ->
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(type.icon(), contentDescription = null, tint = VigiaColors.RadarCyan, modifier = Modifier.size(20.dp))
                Text(type.label(), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
                Spacer(Modifier.weight(1f))
                Text("$count", style = VigiaMono.bodyMedium, color = VigiaColors.TextSecondary)
            }
        }
    }
}

@Composable
private fun NetworkDetailCard(network: CurrentNetwork?) {
    if (network == null || !network.isWifi) return
    val freq = network.frequencyMhz ?: return
    val channelInfo = channelInfoForFrequencyMhz(freq)

    SectionCard(title = stringRes(R.string.panel_wifi_summary)) {
        InfoRow(
            icon = Icons.Default.Router,
            label = stringRes(R.string.panel_network_band_label),
            value = stringResource(R.string.panel_network_band_value, channelInfo.band.label, channelInfo.channel),
        )
        network.linkSpeedMbps?.let { speed ->
            InfoRow(label = stringRes(R.string.panel_network_speed_label), value = stringResource(R.string.panel_network_speed_value, speed), mono = true)
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String, icon: ImageVector? = null, mono: Boolean = false) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        if (icon != null) {
            Icon(icon, contentDescription = null, tint = VigiaColors.RadarCyan, modifier = Modifier.size(18.dp))
        }
        Text(label, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
        Spacer(Modifier.weight(1f))
        Text(value, style = if (mono) VigiaMono.body else MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
    }
}

@Composable
private fun LastScanFooter(result: ScanResult) {
    Text(
        text = stringRes(
            R.string.panel_last_scan,
            "${formatClockTime(result.startedAt, DateFormat.is24HourFormat(LocalContext.current))} · ${formatDurationNanos(result.duration)}",
        ),
        style = VigiaMono.label,
        color = VigiaColors.TextTertiary,
        modifier = Modifier.padding(horizontal = 4.dp).wrapContentWidth(),
    )
}

@Composable
private fun stringRes(id: Int): String = androidx.compose.ui.res.stringResource(id)

@Composable
private fun stringRes(id: Int, arg: String): String = androidx.compose.ui.res.stringResource(id, arg)
