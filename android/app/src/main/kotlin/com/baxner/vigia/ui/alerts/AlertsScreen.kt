package com.baxner.vigia.ui.alerts

import android.text.format.DateFormat
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Devices
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.ReportProblem
import androidx.compose.material.icons.filled.Router
import androidx.compose.material.icons.filled.SettingsEthernet
import androidx.compose.material.icons.filled.SignalWifiOff
import androidx.compose.material.icons.filled.SignalWifiStatusbar4Bar
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.AlertSettingsDto
import com.baxner.vigia.data.agent.AlertSeverity
import com.baxner.vigia.data.agent.AlertType
import com.baxner.vigia.data.agent.isEnabled
import com.baxner.vigia.data.agent.severityOrUnknown
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.GlassSurface
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.permissions.hasNotificationPermission
import com.baxner.vigia.ui.permissions.rememberNotificationPermissionRequester
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.formatClockTime
import com.baxner.vigia.util.formatDateTime
import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId

/**
 * Envoltura con el [AlertsViewModel] real; ver [AlertsScreenContent] para la
 * versión sin estado (capturas, tests). Se llega acá desde la campanita del
 * Panel (ver `ui/agent/AgentCard.kt` y el encargo, punto 5).
 */
@Composable
fun AlertsScreen(onBack: () -> Unit, modifier: Modifier = Modifier) {
    val viewModel: AlertsViewModel = viewModel()
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var showSettings by rememberSaveable { mutableStateOf(false) }
    var hasNotifPermission by remember { mutableStateOf(hasNotificationPermission(context)) }
    val requestNotifPermission = rememberNotificationPermissionRequester { granted -> hasNotifPermission = granted }

    LaunchedEffect(Unit) { viewModel.onOpen() }
    // Al volver de Ajustes del sistema (o del propio diálogo del permiso) puede haber cambiado.
    LifecycleResumeEffect(Unit) {
        hasNotifPermission = hasNotificationPermission(context)
        onPauseOrDispose { }
    }

    AlertsScreenContent(
        uiState = uiState,
        showNotificationBanner = !hasNotifPermission,
        onRequestNotificationPermission = requestNotifPermission,
        onBack = onBack,
        onOpenSettings = { showSettings = true },
        modifier = modifier,
    )

    if (showSettings) {
        AlertSettingsSheet(
            settings = uiState.settings,
            onToggle = viewModel::setTypeEnabled,
            onDismiss = { showSettings = false },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AlertsScreenContent(
    uiState: AlertsUiState,
    onBack: () -> Unit,
    onOpenSettings: () -> Unit,
    modifier: Modifier = Modifier,
    showNotificationBanner: Boolean = false,
    onRequestNotificationPermission: () -> Unit = {},
    now: Instant = remember { Instant.now() },
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.alerts_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                actions = {
                    if (uiState.linked) {
                        IconButton(onClick = onOpenSettings) {
                            Icon(Icons.Default.Tune, contentDescription = stringResource(R.string.alerts_settings_title), tint = VigiaColors.TextPrimary)
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        if (!uiState.linked) {
            EmptyState(
                title = stringResource(R.string.alerts_empty_not_linked_title),
                subtitle = stringResource(R.string.alerts_empty_not_linked_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            return@Scaffold
        }

        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (showNotificationBanner) {
                item(key = "notif_banner") { NotificationPermissionBanner(onRequestNotificationPermission) }
            }
            if (uiState.alerts.isEmpty()) {
                item(key = "empty") {
                    EmptyState(
                        title = stringResource(R.string.alerts_empty_title),
                        subtitle = stringResource(R.string.alerts_empty_subtitle),
                    )
                }
            } else {
                items(uiState.alerts, key = { it.id }) { alert -> AlertRow(alert, now) }
            }
        }
    }
}

@Composable
private fun NotificationPermissionBanner(onRequest: () -> Unit) {
    SectionCard(title = stringResource(R.string.alerts_notif_permission_title)) {
        Text(
            stringResource(R.string.alerts_notif_permission_body),
            style = MaterialTheme.typography.bodyMedium,
            color = VigiaColors.TextSecondary,
        )
        Button(
            onClick = onRequest,
            colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
        ) {
            Text(stringResource(R.string.alerts_notif_permission_action))
        }
    }
}

@Composable
private fun AlertRow(alert: AlertDto, now: Instant) {
    val type = remember(alert.type) { AlertType.fromRaw(alert.type) }
    val (icon, tint) = alertVisuals(type, alert.severityOrUnknown())
    val unreadLabel = stringResource(R.string.alerts_unread_state)
    GlassSurface(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(14.dp)
                // Fusionada + estado "Sin leer" en texto (ver AGENTS.md §
                // accesibilidad): antes el punto cian era la única pista de
                // que una alerta no se había leído.
                .semantics(mergeDescendants = true) {
                    if (!alert.read) stateDescription = unreadLabel
                },
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                // FlowRow (no Row con weight): con letra grande, la hora en
                // mono ("10 Oct 2026 · 9:00 AM") se ensancha y aplastaba el
                // título hasta partir palabras a la mitad (ver AGENTS.md §
                // accesibilidad, "letra grande"). Si no caben los dos en una
                // línea, la hora baja a la siguiente en vez de exprimir el título.
                androidx.compose.foundation.layout.FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    Text(
                        alert.title,
                        style = MaterialTheme.typography.bodyMedium,
                        color = VigiaColors.TextPrimary,
                    )
                    Text(timeLabel(alert.at, now), style = VigiaMono.labelSmall, color = VigiaColors.TextTertiary)
                }
                Text(alert.body, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            }
            if (!alert.read) {
                Box(modifier = Modifier.padding(top = 4.dp).size(8.dp).background(VigiaColors.RadarCyan, CircleShape))
            }
        }
    }
}

/** Icono por tipo, color por severidad (ver el encargo, punto 5: "icono por tipo/severidad"). Un tipo sin reconocer cae en la campana genérica. */
private fun alertVisuals(type: AlertType?, severity: AlertSeverity): Pair<ImageVector, Color> {
    val icon = when (type) {
        AlertType.NEW_DEVICE -> Icons.Default.Devices
        AlertType.IMPORTANT_OFFLINE -> Icons.Default.CloudOff
        AlertType.IMPORTANT_ONLINE -> Icons.Default.CheckCircle
        AlertType.AGENT_BLIND -> Icons.Default.VisibilityOff
        AlertType.AGENT_RECOVERED -> Icons.Default.Visibility
        AlertType.ROUTER_CHANGED -> Icons.Default.Router
        AlertType.ARP_CONFLICT -> Icons.Default.ReportProblem
        AlertType.PORT_MAPPING_ADDED -> Icons.Default.SettingsEthernet
        AlertType.RISKY_SERVICE -> Icons.Default.Dns
        AlertType.INTERNET_DOWN -> Icons.Default.SignalWifiOff
        AlertType.INTERNET_RESTORED -> Icons.Default.SignalWifiStatusbar4Bar
        null -> Icons.Default.Notifications
    }
    val tint = when (severity) {
        AlertSeverity.HIGH -> VigiaColors.Risk
        AlertSeverity.MEDIUM -> VigiaColors.Amber
        AlertSeverity.LOW -> VigiaColors.RadarCyan
        AlertSeverity.UNKNOWN -> VigiaColors.TextTertiary
    }
    return icon to tint
}

/** Hora local (ver `util/TimeFormat.kt`, como pide el encargo): mismo día → solo hora; si no, fecha y hora. */
@Composable
private fun timeLabel(rfc3339: String, now: Instant): String {
    val is24Hour = DateFormat.is24HourFormat(LocalContext.current)
    val instant = runCatching { OffsetDateTime.parse(rfc3339).toInstant() }.getOrNull() ?: return rfc3339
    val zone = ZoneId.systemDefault()
    val sameDay = instant.atZone(zone).toLocalDate() == now.atZone(zone).toLocalDate()
    return if (sameDay) formatClockTime(rfc3339, is24Hour) else formatDateTime(instant, is24Hour)
}

@Composable
private fun AlertSettingsSheet(
    settings: AlertSettingsDto,
    onToggle: (AlertType, Boolean) -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.alerts_settings_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                AlertType.entries.forEach { type ->
                    SettingsToggleRow(
                        label = stringResource(type.labelRes()),
                        checked = settings.isEnabled(type),
                        onCheckedChange = { enabled -> onToggle(type, enabled) },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.alerts_settings_done), color = VigiaColors.RadarCyan) }
        },
        containerColor = VigiaColors.SurfaceElevated,
        titleContentColor = VigiaColors.TextPrimary,
        textContentColor = VigiaColors.TextSecondary,
    )
}

@Composable
private fun SettingsToggleRow(label: String, checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier.toggleable(value = checked, onValueChange = onCheckedChange, role = Role.Switch),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary, modifier = Modifier.weight(1f))
        Switch(
            checked = checked,
            onCheckedChange = null,
            colors = SwitchDefaults.colors(checkedThumbColor = VigiaColors.Background, checkedTrackColor = VigiaColors.RadarCyan),
        )
    }
}

/** No privado: también lo usa `ui/agent/WebhooksScreen.kt` para la lista de tipos a enviar. */
internal fun AlertType.labelRes(): Int = when (this) {
    AlertType.NEW_DEVICE -> R.string.alert_type_new_device
    AlertType.IMPORTANT_OFFLINE -> R.string.alert_type_important_offline
    AlertType.IMPORTANT_ONLINE -> R.string.alert_type_important_online
    AlertType.AGENT_BLIND -> R.string.alert_type_agent_blind
    AlertType.AGENT_RECOVERED -> R.string.alert_type_agent_recovered
    AlertType.ROUTER_CHANGED -> R.string.alert_type_router_changed
    AlertType.ARP_CONFLICT -> R.string.alert_type_arp_conflict
    AlertType.PORT_MAPPING_ADDED -> R.string.alert_type_port_mapping_added
    AlertType.RISKY_SERVICE -> R.string.alert_type_risky_service
    AlertType.INTERNET_DOWN -> R.string.alert_type_internet_down
    AlertType.INTERNET_RESTORED -> R.string.alert_type_internet_restored
}
