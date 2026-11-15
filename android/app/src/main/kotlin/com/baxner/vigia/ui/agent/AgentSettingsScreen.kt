package com.baxner.vigia.ui.agent

import android.app.Application
import androidx.compose.foundation.clickable
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.permissions.rememberNotificationPermissionRequester
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono

/**
 * Ajustes mínimos del agente vinculado (ver el encargo, punto 5): nombre,
 * dirección y desvincular. Se llega acá tocando la tarjeta "Vigilancia
 * 24/7" ya vinculada.
 */
@Composable
fun AgentSettingsScreen(onDone: () -> Unit, modifier: Modifier = Modifier, onOpenWebhooks: () -> Unit = {}) {
    val context = LocalContext.current
    val agentController = remember { AgentModule.controller(context.applicationContext as Application) }
    val agentName = remember { agentController.linkedAgentName() ?: "" }
    val hostAndPort = remember { agentController.linkedHostAndPort() }
    var liveWatchEnabled by remember { mutableStateOf(agentController.isLiveWatchEnabled()) }
    // "Ver tu casa remoto" (ver el encargo, fase 8): StateFlow, no un valor
    // leído una sola vez — si cambia desde otro lado (no debería, pero por
    // si acaso) la pantalla no se queda desactualizada.
    val remoteViewPreferred by agentController.remoteViewPreferred.collectAsStateWithLifecycle()
    val requestNotificationPermission = rememberNotificationPermissionRequester()

    AgentSettingsScreenContent(
        agentName = agentName,
        hostAndPort = hostAndPort,
        onBack = onDone,
        onUnlink = {
            agentController.forget()
            onDone()
        },
        liveWatchEnabled = liveWatchEnabled,
        onLiveWatchToggle = { enabled ->
            liveWatchEnabled = enabled
            agentController.setLiveWatchEnabled(enabled)
            // "al activar alertas" (ver ui/permissions/NotificationPermission.kt):
            // sin este permiso la notificación fija ni se vería, aunque el
            // servicio sí corra — mejor pedirlo ya mismo.
            if (enabled) requestNotificationPermission()
        },
        remoteViewPreferred = remoteViewPreferred,
        onRemoteViewToggle = agentController::setRemoteViewPreferred,
        onOpenWebhooks = onOpenWebhooks,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AgentSettingsScreenContent(
    agentName: String,
    hostAndPort: Pair<String, Int>?,
    onBack: () -> Unit,
    onUnlink: () -> Unit,
    modifier: Modifier = Modifier,
    liveWatchEnabled: Boolean = false,
    onLiveWatchToggle: (Boolean) -> Unit = {},
    remoteViewPreferred: Boolean = true,
    onRemoteViewToggle: (Boolean) -> Unit = {},
    onOpenWebhooks: () -> Unit = {},
) {
    var showConfirm by remember { mutableStateOf(false) }

    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.agent_settings_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            SectionCard(title = agentName) {
                if (hostAndPort != null) {
                    Text("${hostAndPort.first}:${hostAndPort.second}", style = VigiaMono.body, color = VigiaColors.TextSecondary)
                }
            }
            SectionCard(title = stringResource(R.string.agent_settings_live_watch_title)) {
                Row(
                    modifier = Modifier.fillMaxWidth().toggleable(value = liveWatchEnabled, onValueChange = onLiveWatchToggle, role = Role.Switch),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        stringResource(R.string.agent_settings_live_watch_subtitle),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.TextSecondary,
                        modifier = Modifier.weight(1f).padding(end = 12.dp),
                    )
                    Switch(
                        checked = liveWatchEnabled,
                        onCheckedChange = null,
                        colors = SwitchDefaults.colors(checkedTrackColor = VigiaColors.RadarCyan),
                    )
                }
            }
            SectionCard(title = stringResource(R.string.agent_settings_remote_view_title)) {
                Row(
                    modifier = Modifier.fillMaxWidth().toggleable(value = remoteViewPreferred, onValueChange = onRemoteViewToggle, role = Role.Switch),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        stringResource(R.string.agent_settings_remote_view_subtitle),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.TextSecondary,
                        modifier = Modifier.weight(1f).padding(end = 12.dp),
                    )
                    Switch(
                        checked = remoteViewPreferred,
                        onCheckedChange = null,
                        colors = SwitchDefaults.colors(checkedTrackColor = VigiaColors.RadarCyan),
                    )
                }
            }
            SectionCard(title = stringResource(R.string.agent_settings_webhooks_button)) {
                Row(
                    modifier = Modifier.fillMaxWidth().clickable(onClick = onOpenWebhooks).semantics(mergeDescendants = true) {},
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        stringResource(R.string.agent_settings_webhooks_subtitle),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.TextSecondary,
                        modifier = Modifier.weight(1f).padding(end = 12.dp),
                    )
                    Icon(Icons.Default.ChevronRight, contentDescription = null, tint = VigiaColors.TextTertiary)
                }
            }
            OutlinedButton(
                onClick = { showConfirm = true },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = VigiaColors.Risk),
            ) {
                Text(stringResource(R.string.agent_settings_unlink))
            }
        }
    }

    if (showConfirm) {
        AlertDialog(
            onDismissRequest = { showConfirm = false },
            title = { Text(stringResource(R.string.agent_settings_unlink_confirm_title)) },
            text = { Text(stringResource(R.string.agent_settings_unlink_confirm_message, agentName)) },
            confirmButton = {
                TextButton(onClick = { showConfirm = false; onUnlink() }) {
                    Text(stringResource(R.string.agent_settings_unlink), color = VigiaColors.Risk)
                }
            },
            dismissButton = {
                TextButton(onClick = { showConfirm = false }) { Text(stringResource(R.string.agent_settings_unlink_cancel)) }
            },
            containerColor = VigiaColors.SurfaceElevated,
            titleContentColor = VigiaColors.TextPrimary,
            textContentColor = VigiaColors.TextSecondary,
        )
    }
}
