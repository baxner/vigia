package com.baxner.vigia.ui.agent

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.SignalWifiStatusbar4Bar
import androidx.compose.material.icons.filled.WarningAmber
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.agent.InternetState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.internet.InternetPanelSummary
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.util.relativeTimeFrom
import java.time.Instant

/**
 * Tarjeta "Vigilancia 24/7" del Panel (ver el encargo, punto 5): sin agente,
 * invita a vincular; vinculado, muestra si esta red la vigila el agente
 * ahora mismo y hace cuánto escaneó. Tocarla (ya vinculado) abre los ajustes
 * mínimos (ver [com.baxner.vigia.ui.agent.AgentSettingsScreen]).
 */
@Composable
fun AgentCard(
    state: AgentPanelState,
    onLinkClick: () -> Unit,
    onManageClick: () -> Unit,
    modifier: Modifier = Modifier,
    now: Instant = Instant.now(),
    /** No leídas (ver el encargo, punto 5: "contador de no leídas... en la tarjeta de Vigilancia 24/7"). 0 = sin campanita con número, pero la campana sigue ahí para abrir Alertas. */
    unreadAlerts: Int = 0,
    onOpenAlerts: () -> Unit = {},
    /**
     * "Internet · 18 ms · bien" (fase 7, ver `docs/plan.md`): `null` sin
     * dato todavía (p. ej. el agente contestó pero no mandó `/internet`, o
     * esta red no la vigila — ver [onOpenInternet], que solo se dibuja con
     * un resumen en mano).
     */
    internetSummary: InternetPanelSummary? = null,
    onOpenInternet: () -> Unit = {},
) {
    SectionCard(title = stringResource(R.string.agent_card_title), modifier = modifier) {
        when (state) {
            AgentPanelState.NotLinked -> NotLinkedContent(onLinkClick)
            is AgentPanelState.Linked -> LinkedContent(state, onManageClick, now, unreadAlerts, onOpenAlerts, internetSummary, onOpenInternet)
        }
    }
}

@Composable
private fun NotLinkedContent(onLinkClick: () -> Unit) {
    Text(
        stringResource(R.string.agent_card_not_linked_hint),
        style = MaterialTheme.typography.bodyMedium,
        color = VigiaColors.TextSecondary,
    )
    Button(
        onClick = onLinkClick,
        colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
        shape = MaterialTheme.shapes.large,
    ) {
        Text(stringResource(R.string.agent_card_link_button))
    }
}

@Composable
private fun LinkedContent(
    state: AgentPanelState.Linked,
    onManageClick: () -> Unit,
    now: Instant,
    unreadAlerts: Int,
    onOpenAlerts: () -> Unit,
    internetSummary: InternetPanelSummary?,
    onOpenInternet: () -> Unit,
) {
    val (icon, tint) = when {
        !state.reachable -> Icons.Default.CloudOff to VigiaColors.TextTertiary
        // Viendo remoto (ver el encargo, fase 8): tan "protegido" como
        // vigilando en casa, mismo ícono y color — la diferencia la dice el
        // texto (ver [statusLine]), no el ícono.
        state.viewingRemote -> Icons.Default.Shield to VigiaColors.RadarCyan
        !state.monitoringCurrentNetwork -> Icons.Default.WarningAmber to VigiaColors.Amber
        state.warning != null -> Icons.Default.WarningAmber to VigiaColors.Amber
        else -> Icons.Default.Shield to VigiaColors.RadarCyan
    }
    Row(
        // mergeDescendants (ver AGENTS.md § accesibilidad): fusiona el
        // estado y el nombre del agente en una sola lectura. La campanita
        // ([AlertsBell]) tiene su propio clic y Compose la deja como parada
        // aparte en vez de tragársela — no hace falta excluirla a mano.
        modifier = Modifier.fillMaxWidth().clickable(onClick = onManageClick).semantics(mergeDescendants = true) {},
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Surface(shape = CircleShape, color = tint.copy(alpha = 0.16f)) {
            Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.padding(8.dp).size(20.dp))
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(statusLine(state, now), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
            Text(state.agentName, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            state.warning?.let { warning ->
                Text(warning, style = MaterialTheme.typography.bodySmall, color = VigiaColors.Amber)
            }
        }
        AlertsBell(unreadCount = unreadAlerts, onClick = onOpenAlerts)
        Icon(Icons.Default.ChevronRight, contentDescription = null, tint = VigiaColors.TextTertiary)
    }
    // "Internet · 18 ms · bien" (fase 7, ver docs/plan.md § "Fase 7"):
    // acceso a la pantalla Internet desde esta misma tarjeta, solo si esta
    // red es la que vigila el agente (si no, no hay de dónde medir).
    if ((state.monitoringCurrentNetwork || state.viewingRemote) && internetSummary != null) {
        HorizontalDivider(color = VigiaColors.Outline, modifier = Modifier.padding(vertical = 10.dp))
        InternetRow(internetSummary, onOpenInternet)
    }
}

@Composable
private fun InternetRow(summary: InternetPanelSummary, onClick: () -> Unit) {
    val (tint, stateLabel) = when (summary.state) {
        InternetState.UP -> VigiaColors.Ok to stringResource(R.string.internet_panel_state_up)
        InternetState.DEGRADED -> VigiaColors.Amber to stringResource(R.string.internet_panel_state_degraded)
        InternetState.DOWN -> VigiaColors.Risk to stringResource(R.string.internet_panel_state_down)
        InternetState.UNKNOWN -> VigiaColors.TextTertiary to stringResource(R.string.internet_panel_state_unknown)
    }
    val text = summary.latencyMs?.let { ms ->
        stringResource(R.string.internet_panel_row_latency, ms.toString(), stateLabel)
    } ?: stringResource(R.string.internet_panel_row_plain, stateLabel)
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).semantics(mergeDescendants = true) {},
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(Icons.Default.SignalWifiStatusbar4Bar, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary, modifier = Modifier.weight(1f))
        Icon(Icons.Default.ChevronRight, contentDescription = null, tint = VigiaColors.TextTertiary, modifier = Modifier.size(18.dp))
    }
}

/** Campanita con contador de no leídas (ver el encargo, punto 5). Clickable propio: tocarla abre Alertas sin disparar [onManageClick] de la fila completa. */
@Composable
private fun AlertsBell(unreadCount: Int, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .minimumInteractiveComponentSize()
            .clickable(onClick = onClick)
            .padding(4.dp),
        contentAlignment = Alignment.Center,
    ) {
        Icon(Icons.Default.Notifications, contentDescription = stringResource(R.string.alerts_title), tint = VigiaColors.TextSecondary)
        if (unreadCount > 0) {
            Surface(
                shape = CircleShape,
                color = VigiaColors.Risk,
                modifier = Modifier.align(Alignment.TopEnd).size(16.dp),
            ) {
                Box(contentAlignment = Alignment.Center) {
                    Text(
                        if (unreadCount > 9) "9+" else "$unreadCount",
                        style = BadgeTextStyle,
                        color = VigiaColors.TextPrimary,
                    )
                }
            }
        }
    }
}

private val BadgeTextStyle = TextStyle(fontSize = 9.sp, fontWeight = FontWeight.Bold)

@Composable
private fun statusLine(state: AgentPanelState.Linked, now: Instant): String = when {
    // Fuera de casa y ni la LAN ni Tailscale contestaron (ver el encargo,
    // fase 8, punto 4): mensaje específico, no el genérico de abajo —
    // "revisa que Tailscale esté encendido" es lo primero que hay que mirar.
    !state.reachable && !state.monitoringCurrentNetwork -> stringResource(R.string.agent_card_status_away_unreachable, state.agentName)
    !state.reachable -> stringResource(R.string.agent_card_status_unreachable, state.agentName)
    state.viewingRemote -> stringResource(R.string.agent_card_status_remote)
    !state.monitoringCurrentNetwork -> stringResource(R.string.agent_card_status_other_network, state.agentName)
    state.lastScanAt == null -> stringResource(R.string.agent_card_status_monitoring_never)
    else -> stringResource(R.string.agent_card_status_monitoring_relative, agoLabel(relativeTimeFrom(state.lastScanAt, now)))
}

/** Insignia chica de la cabecera del Panel cuando esta red sí está "Vigilada 24/7" (ver el encargo, punto 5). */
@Composable
fun AgentMonitoredBadge(modifier: Modifier = Modifier) {
    Surface(modifier = modifier, shape = RoundedCornerShape(50), color = VigiaColors.RadarCyan.copy(alpha = 0.16f)) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(Icons.Default.Shield, contentDescription = null, tint = VigiaColors.RadarCyan, modifier = Modifier.size(14.dp))
            Text(stringResource(R.string.agent_header_badge), style = MaterialTheme.typography.labelSmall, color = VigiaColors.RadarCyan)
        }
    }
}

/**
 * Insignia chica de la cabecera del Panel cuando lo que se ve ahí (radar,
 * tiles, lista) es la casa remoto por Tailscale, no la red actual del
 * teléfono (ver el encargo, fase 8, punto 2: "que quede claro qué se está
 * viendo"). Mismo estilo que [AgentMonitoredBadge] — es, en el fondo, el
 * mismo tipo de vigilancia, solo que de lejos — con un ámbar en vez del
 * cian para no confundirla con "Vigilada 24/7" a simple vista.
 */
@Composable
fun AgentRemoteBadge(modifier: Modifier = Modifier) {
    Surface(modifier = modifier, shape = RoundedCornerShape(50), color = VigiaColors.Amber.copy(alpha = 0.16f)) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(Icons.Default.Shield, contentDescription = null, tint = VigiaColors.Amber, modifier = Modifier.size(14.dp))
            Text(stringResource(R.string.agent_header_badge_remote), style = MaterialTheme.typography.labelSmall, color = VigiaColors.Amber)
        }
    }
}
