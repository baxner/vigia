package com.baxner.vigia.ui.devices

import android.content.ClipData
import android.os.Build
import android.text.format.DateFormat
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Label
import androidx.compose.material.icons.automirrored.filled.Login
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.automirrored.filled.Notes
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.EventAvailable
import androidx.compose.material.icons.filled.EventBusy
import androidx.compose.material.icons.filled.Factory
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Hub
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Sensors
import androidx.compose.material.icons.filled.SettingsEthernet
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.SwapHoriz
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.ClipEntry
import androidx.compose.ui.platform.LocalClipboard
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.history.DeviceEvent
import com.baxner.vigia.data.history.DeviceEventType
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.DeviceType
import com.baxner.vigia.data.model.PortCatalog
import com.baxner.vigia.data.model.Service
import com.baxner.vigia.ui.components.ConfidenceRing
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.components.vigiaGlow
import com.baxner.vigia.ui.scan.PortScanState
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.RelativeTime
import com.baxner.vigia.util.formatClockTime
import com.baxner.vigia.util.formatDateTime
import com.baxner.vigia.util.relativeTimeFrom
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import java.time.Instant

/**
 * Envoltura fina sobre [DeviceDetailScreenContent]: separadas para poder
 * capturar el contenido con datos de muestra sin un [ScanViewModel] real
 * (ver la misma nota en PanelScreen.kt). Por [id] estable, no por IP.
 */
@Composable
fun DeviceDetailScreen(
    id: Long,
    scanViewModel: ScanViewModel,
    onBack: () -> Unit,
    onEdit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val device by scanViewModel.device(id).collectAsStateWithLifecycle(initialValue = null)
    val events by scanViewModel.events(id).collectAsStateWithLifecycle(initialValue = emptyList())
    val agentState by scanViewModel.agentPanelState.collectAsStateWithLifecycle()
    val viewingRemote = (agentState as? AgentPanelState.Linked)?.viewingRemote == true
    // El escaneo completo de puertos vive en WorkManager, no en esta
    // pantalla: si se sale, sigue, y al volver aquí está el resultado.
    val network = device?.network
    val portScanFlow = remember(network, id) { network?.let { scanViewModel.portScan(it, id) } ?: flowOf(PortScanState.Idle) }
    val portScanState by portScanFlow.collectAsStateWithLifecycle(initialValue = PortScanState.Idle)

    DeviceDetailScreenContent(
        device = device,
        events = events,
        portScanState = portScanState.forDevice(device),
        onBack = onBack,
        onEdit = onEdit,
        onScanAllPorts = { device?.let { scanViewModel.scanAllPorts(it) } },
        onForget = { scanViewModel.forget(id) },
        canScanAllPorts = !viewingRemote,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceDetailScreenContent(
    device: KnownDevice?,
    events: List<DeviceEvent>,
    portScanState: PortScanState,
    onBack: () -> Unit,
    onEdit: () -> Unit,
    onScanAllPorts: () -> Unit,
    onForget: () -> Unit,
    modifier: Modifier = Modifier,
    canScanAllPorts: Boolean = true,
) {
    var menuExpanded by remember { mutableStateOf(false) }
    var confirmForget by remember { mutableStateOf(false) }
    val is24Hour = DateFormat.is24HourFormat(LocalContext.current)

    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(device?.displayName() ?: "") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                actions = {
                    if (device != null) {
                        IconButton(onClick = onEdit) {
                            Icon(Icons.Default.Edit, contentDescription = stringResource(R.string.detail_action_edit), tint = VigiaColors.TextPrimary)
                        }
                        Box {
                            IconButton(onClick = { menuExpanded = true }) {
                                Icon(Icons.Default.MoreVert, contentDescription = stringResource(R.string.a11y_more_options), tint = VigiaColors.TextPrimary)
                            }
                            DropdownMenu(expanded = menuExpanded, onDismissRequest = { menuExpanded = false }) {
                                DropdownMenuItem(
                                    text = { Text(stringResource(R.string.detail_action_forget), color = VigiaColors.Risk) },
                                    leadingIcon = { Icon(Icons.Default.Delete, contentDescription = null, tint = VigiaColors.Risk) },
                                    onClick = { menuExpanded = false; confirmForget = true },
                                )
                            }
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = VigiaColors.Background,
                    titleContentColor = VigiaColors.TextPrimary,
                ),
            )
        },
    ) { padding ->
        if (device == null) {
            EmptyState(
                title = stringResource(R.string.devices_empty),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            return@Scaffold
        }

        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "hero") { HeroHeader(device) }
            item(key = "status") { StatusSection(device, is24Hour) }
            item(key = "reason") { ReasonSection(device.last) }
            item(key = "network") { NetworkSection(device.last) }
            if (device.last.services.isNotEmpty()) {
                item(key = "services") { ServicesSection(device.last.services) }
            }
            item(key = "ports") {
                PortsSection(
                    device = device.last,
                    portScanState = portScanState,
                    onScanAllPorts = onScanAllPorts,
                    canScanAllPorts = canScanAllPorts,
                )
            }
            if (events.isNotEmpty()) {
                item(key = "timeline") { TimelineSection(events, is24Hour) }
            }
        }
    }

    if (confirmForget) {
        ForgetDeviceDialog(
            deviceName = device?.displayName().orEmpty(),
            onConfirm = {
                confirmForget = false
                onForget()
                onBack()
            },
            onDismiss = { confirmForget = false },
        )
    }
}

@Composable
private fun ForgetDeviceDialog(deviceName: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.detail_forget_confirm_title)) },
        text = { Text(stringResource(R.string.detail_forget_confirm_message, deviceName)) },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(stringResource(R.string.detail_action_forget), color = VigiaColors.Risk)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.detail_forget_cancel)) }
        },
        containerColor = VigiaColors.SurfaceElevated,
        titleContentColor = VigiaColors.TextPrimary,
        textContentColor = VigiaColors.TextSecondary,
    )
}

/** Cabecera héroe: icono grande con brillo, nombre, "Marca Modelo", chip de SO y anillo de confianza. */
@Composable
private fun HeroHeader(device: KnownDevice) {
    val accent = device.statusColor()
    val name = device.displayName()
    val last = device.last
    val brandModel = listOf(last.identity.brand, last.identity.model).filter { it.isNotBlank() }.joinToString(" ")
        // Sin esto, un equipo sin hostname propio repite "Marca Modelo" dos veces
        // (el nombre YA es "Marca Modelo" cuando nameOrNull() cae a esa regla).
        .takeUnless { it.isBlank() || it.equals(name, ignoreCase = true) }
        .orEmpty()

    SectionCard(title = "", glow = true) {
        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
            Box(
                modifier = Modifier
                    .size(64.dp)
                    .vigiaGlow(color = accent, alpha = 0.3f)
                    .background(accent.copy(alpha = 0.16f), RoundedCornerShape(16.dp)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(device.icon(), contentDescription = device.typeLabel(), tint = accent, modifier = Modifier.size(32.dp))
            }
            Column(modifier = Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        device.typeLabel().uppercase(),
                        style = MaterialTheme.typography.labelSmall,
                        color = VigiaColors.TextTertiary,
                    )
                    if (device.custom.important) {
                        Icon(
                            Icons.Default.Star,
                            contentDescription = stringResource(R.string.devices_badge_important),
                            tint = VigiaColors.Amber,
                            modifier = Modifier.size(13.dp),
                        )
                    }
                }
                Text(
                    name,
                    style = MaterialTheme.typography.headlineSmall,
                    color = VigiaColors.TextPrimary,
                )
                if (brandModel.isNotBlank()) {
                    Text(brandModel, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                }
                if (last.identity.os.isNotBlank()) {
                    Spacer(Modifier.height(6.dp))
                    OsChip(last.identity.os)
                }
            }
            // Sin tipo, o con el tipo elegido a mano, el % del motor no dice nada.
            if (device.coarseType() != DeviceType.UNKNOWN && device.custom.typeId == null) {
                ConfidenceRing(confidence = last.identity.confidence, size = 52.dp, strokeWidth = 4.dp)
            }
        }
    }
}

@Composable
private fun OsChip(os: String) {
    Surface(shape = RoundedCornerShape(50), color = VigiaColors.SurfaceElevated) {
        Text(
            os,
            style = MaterialTheme.typography.labelMedium,
            color = VigiaColors.ElectricBlue,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

/**
 * "En línea desde"/"Sin conexión desde", "Visto por primera vez", ubicación
 * y notas (si las hay) e "Importante" (si lo es): el resumen de historial y
 * personalización que no cabe en la cabecera.
 */
@Composable
private fun StatusSection(device: KnownDevice, is24Hour: Boolean) {
    SectionCard(title = stringResource(R.string.detail_status_title)) {
        val sinceText = if (device.online) {
            stringResource(R.string.detail_online_since, formatDateTime(device.onlineSince ?: device.lastSeen, is24Hour))
        } else {
            stringResource(R.string.detail_offline_since, formatDateTime(device.lastSeen, is24Hour))
        }
        InfoLine(
            icon = if (device.online) Icons.Default.EventAvailable else Icons.Default.EventBusy,
            iconTint = if (device.online) VigiaColors.Ok else VigiaColors.TextSecondary,
            text = sinceText,
        )
        InfoLine(
            icon = Icons.Default.History,
            iconTint = VigiaColors.TextSecondary,
            text = stringResource(R.string.detail_first_seen_at, formatDateTime(device.firstSeen, is24Hour)),
        )
        device.custom.location?.takeUnless { it.isBlank() }?.let {
            InfoLine(icon = Icons.Default.LocationOn, iconTint = VigiaColors.ElectricBlue, text = it)
        }
        device.custom.notes?.takeUnless { it.isBlank() }?.let {
            InfoLine(icon = Icons.AutoMirrored.Filled.Notes, iconTint = VigiaColors.TextSecondary, text = it)
        }
        if (device.custom.important) {
            InfoLine(icon = Icons.Default.Star, iconTint = VigiaColors.Amber, text = stringResource(R.string.edit_important_hint))
        }
    }
}

@Composable
private fun InfoLine(icon: ImageVector, iconTint: Color, text: String) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        Icon(icon, contentDescription = null, tint = iconTint, modifier = Modifier.size(18.dp))
        Text(text, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
    }
}

/** Una pista de [Identity.reason], ya traducida a icono + encabezado + detalle. */
private data class Clue(val icon: ImageVector, val label: String, val detail: String)

/**
 * Traduce una pista cruda del motor ("mdns _airplay._tcp", "puerto 554",
 * "oui TP-Link", "hostname=PC-Escritorio", "equipo que escanea"...)
 * a un icono y un texto legible, por el prefijo. "api"/"http" quedan
 * cubiertos por si el motor empieza a mandarlos (identificación por web).
 */
@Composable
private fun parseClue(raw: String): Clue {
    val text = raw.trim()
    return when {
        text.equals("equipo que escanea", ignoreCase = true) ->
            Clue(Icons.Default.PhoneAndroid, stringResource(R.string.detail_clue_self), stringResource(R.string.detail_clue_self_detail))

        text.startsWith("mdns", ignoreCase = true) -> {
            var d = text.substring(4).trim().removePrefix("model=")
            if (d.startsWith("_")) d = d.removePrefix("_").removeSuffix("._tcp").removeSuffix("._udp")
            Clue(Icons.Default.Podcasts, stringResource(R.string.detail_clue_bonjour), d.ifBlank { stringResource(R.string.detail_clue_bonjour_generic) })
        }

        text.startsWith("ssdp", ignoreCase = true) -> {
            val d = text.substring(4).trim()
            Clue(Icons.Default.Hub, stringResource(R.string.detail_clue_upnp), d.ifBlank { stringResource(R.string.detail_clue_upnp_generic) })
        }

        text.startsWith("puertos", ignoreCase = true) || text.startsWith("puerto", ignoreCase = true) -> {
            val num = text.substringAfter(' ', "").trim()
            val port = num.toIntOrNull()
            val detail = if (port != null) "$port · ${PortCatalog.localizedNameFor(port)}" else num
            Clue(Icons.Default.SettingsEthernet, stringResource(R.string.detail_clue_port), detail)
        }

        text.startsWith("tuya", ignoreCase = true) ->
            Clue(Icons.Default.Sensors, stringResource(R.string.detail_clue_tuya), stringResource(R.string.detail_clue_tuya_detail))

        text.startsWith("oui", ignoreCase = true) ->
            Clue(Icons.Default.Factory, stringResource(R.string.detail_clue_vendor), text.substringAfter(' ', text).trim())

        text.startsWith("hostname", ignoreCase = true) ->
            Clue(Icons.AutoMirrored.Filled.Label, stringResource(R.string.detail_clue_hostname), text.substringAfter('=', text).trim())

        text.startsWith("api", ignoreCase = true) || text.startsWith("http", ignoreCase = true) ->
            Clue(Icons.Default.Language, stringResource(R.string.detail_clue_web), text.substringAfter(' ', text).trim())

        else -> Clue(Icons.Default.Info, stringResource(R.string.detail_clue_generic), text)
    }
}

/**
 * "Por qué creemos que es esto" como línea de tiempo: cada pista de
 * [Identity.reason] (separadas por "; ") con su icono y, entre ellas, el
 * trazo que las conecta. Es la pieza de transparencia que distingue a Vigía
 * de una caja negra como Fing.
 */
@Composable
private fun ReasonSection(device: Device) {
    val reason = device.identity.reason
    val clueTexts = remember(reason) { reason.split("; ").filter { it.isNotBlank() } }
    SectionCard(title = stringResource(R.string.detail_why_title)) {
        if (clueTexts.isEmpty()) {
            Text(stringResource(R.string.detail_why_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            clueTexts.forEachIndexed { index, raw ->
                val clue = parseClue(raw)
                TimelineRow(icon = clue.icon, label = clue.label, detail = clue.detail, isLast = index == clueTexts.lastIndex)
            }
        }
    }
}

/** Una fila de la línea de tiempo: icono en círculo + trazo, encabezado y detalle. Comparte diseño "Por qué creemos que es esto" y "Cronología". */
@Composable
private fun TimelineRow(icon: ImageVector, label: String, detail: String, isLast: Boolean, iconTint: Color = VigiaColors.RadarCyan) {
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Box(
                modifier = Modifier
                    .size(28.dp)
                    .background(VigiaColors.SurfaceElevated, CircleShape),
                contentAlignment = Alignment.Center,
            ) {
                Icon(icon, contentDescription = null, tint = iconTint, modifier = Modifier.size(15.dp))
            }
            if (!isLast) {
                Box(modifier = Modifier.width(1.dp).height(22.dp).background(VigiaColors.Outline))
            }
        }
        Column(modifier = Modifier.padding(bottom = if (isLast) 0.dp else 10.dp, top = 3.dp)) {
            Text(label, style = MaterialTheme.typography.titleSmall, color = VigiaColors.TextPrimary)
            if (detail.isNotBlank()) {
                Text(detail, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            }
        }
    }
}

/**
 * Cronología: todos los eventos del equipo (más reciente arriba), con icono
 * y color por tipo y la fecha y hora local (12/24 h como el resto de la app).
 */
@Composable
private fun TimelineSection(events: List<DeviceEvent>, is24Hour: Boolean) {
    SectionCard(title = stringResource(R.string.detail_timeline_title)) {
        events.forEachIndexed { index, event ->
            val (icon, tint, label) = eventVisual(event.type)
            val detail = buildString {
                event.detail?.takeUnless { it.isBlank() }?.let { append(it); append(" · ") }
                append(formatDateTime(event.at, is24Hour))
            }
            TimelineRow(icon = icon, label = label, detail = detail, isLast = index == events.lastIndex, iconTint = tint)
        }
    }
}

@Composable
private fun eventVisual(type: DeviceEventType): Triple<ImageVector, Color, String> = when (type) {
    DeviceEventType.FIRST_SEEN -> Triple(Icons.Default.AddCircle, VigiaColors.RadarCyan, stringResource(R.string.detail_event_first_seen))
    DeviceEventType.CAME_ONLINE -> Triple(Icons.AutoMirrored.Filled.Login, VigiaColors.Ok, stringResource(R.string.detail_event_came_online))
    DeviceEventType.WENT_OFFLINE -> Triple(Icons.AutoMirrored.Filled.Logout, VigiaColors.Amber, stringResource(R.string.detail_event_went_offline))
    DeviceEventType.IP_CHANGED -> Triple(Icons.Default.SwapHoriz, VigiaColors.ElectricBlue, stringResource(R.string.detail_event_ip_changed))
}

/** Datos de red en JetBrains Mono: IP, MAC e IPv6 son identificadores, no palabras. */
@Composable
private fun NetworkSection(device: Device) {
    SectionCard(title = stringResource(R.string.detail_network_title)) {
        LabeledValue("IP", device.ip, mono = true, copyable = true)
        LabeledValue(
            stringResource(R.string.detail_mac),
            device.mac.ifBlank { stringResource(R.string.detail_mac_hidden) },
            mono = device.mac.isNotBlank(),
            copyable = device.mac.isNotBlank(),
        )
        if (device.vendor.isNotBlank()) {
            LabeledValue(stringResource(R.string.detail_vendor), device.vendor)
        }
        if (device.hostname.isNotBlank()) {
            LabeledValue(stringResource(R.string.detail_hostname), device.hostname, mono = true, copyable = true)
        }
        if (device.ipv6.isNotEmpty()) {
            LabeledValue("IPv6", device.ipv6.joinToString(), mono = true)
        }
        if (device.sources.isNotEmpty()) {
            LabeledValue(stringResource(R.string.detail_sources), device.sources.map { sourceLabel(it) }.joinToString())
        }
        if (device.lastSeen.isNotBlank()) {
            Text(
                stringResource(R.string.detail_last_seen, formatClockTime(device.lastSeen, DateFormat.is24HourFormat(LocalContext.current))),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextTertiary,
            )
        }
    }
}

@Composable
private fun LabeledValue(label: String, value: String, mono: Boolean = false, copyable: Boolean = false) {
    val clipboard = LocalClipboard.current
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val copiedText = stringResource(R.string.detail_copied, value)
    Row(
        modifier = if (copyable) {
            Modifier
                .defaultMinSize(minHeight = 48.dp)
                .clickable(onClickLabel = stringResource(R.string.detail_copy_action, label)) {
                    scope.launch { clipboard.setClipEntry(ClipEntry(ClipData.newPlainText(label, value))) }
                    // Android 13+ ya muestra su propio aviso al copiar: no duplicarlo.
                    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
                        Toast.makeText(context, copiedText, Toast.LENGTH_SHORT).show()
                    }
                }
        } else {
            Modifier
        },
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        Text(
            value,
            style = if (mono) VigiaMono.body else MaterialTheme.typography.bodyMedium,
            color = VigiaColors.TextPrimary,
            // Sin llenar: el icono de copiar queda pegado al valor y no se sale si el texto es largo.
            modifier = Modifier.weight(1f, fill = false),
        )
        if (copyable) {
            Icon(
                Icons.Default.ContentCopy,
                contentDescription = null,
                tint = VigiaColors.TextTertiary,
                modifier = Modifier.size(14.dp),
            )
        }
    }
}

@Composable
private fun ServicesSection(services: List<Service>) {
    SectionCard(title = stringResource(R.string.detail_services_title)) {
        services.forEach { service ->
            Column {
                Text("${service.proto} · ${service.name}", style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
                if (service.info.isNotBlank()) {
                    Text(service.info, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                }
            }
        }
    }
}

/**
 * Un resultado anterior a que este equipo apareciera no puede ser suyo: es
 * de otro que tuvo el mismo id (el agente reusa ids al olvidar equipos).
 */
private fun PortScanState.forDevice(device: KnownDevice?): PortScanState {
    val finishedAt = (this as? PortScanState.Done)?.finishedAt ?: return this
    return if (device != null && finishedAt.isBefore(device.firstSeen)) PortScanState.Idle else this
}

@Composable
private fun PortsSection(
    device: Device,
    portScanState: PortScanState,
    onScanAllPorts: () -> Unit,
    canScanAllPorts: Boolean = true,
) {
    SectionCard(title = stringResource(R.string.detail_ports_title)) {
        if (device.openPorts.isEmpty()) {
            Text(stringResource(R.string.detail_no_ports), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            PortChips(device.openPorts)
        }

        when (portScanState) {
            PortScanState.Scanning -> {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = VigiaColors.RadarCyan)
                    Text(stringResource(R.string.detail_scanning_ports), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                }
            }
            is PortScanState.Done -> {
                Text(fullScanLabel(portScanState.finishedAt), style = MaterialTheme.typography.labelMedium, color = VigiaColors.TextSecondary)
                if (portScanState.ip.isNotBlank() && portScanState.ip != device.ip) {
                    Text(stringResource(R.string.detail_full_scan_other_ip, portScanState.ip), style = MaterialTheme.typography.bodySmall, color = VigiaColors.Amber)
                }
                if (portScanState.ports.isEmpty()) {
                    Text(stringResource(R.string.detail_full_scan_no_ports), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                } else {
                    PortChips(portScanState.ports)
                }
            }
            is PortScanState.Failed -> {
                val msg = portScanState.message.ifBlank { stringResource(R.string.error_unknown) }
                Text(stringResource(R.string.detail_full_scan_error, msg), style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
            }
            PortScanState.Idle -> Unit
        }

        Spacer(Modifier.size(4.dp))
        val isScanning = portScanState is PortScanState.Scanning
        if (canScanAllPorts) {
            OutlinedButton(onClick = onScanAllPorts, enabled = !isScanning, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.detail_scan_all_ports))
            }
        } else {
            Text(
                stringResource(R.string.detail_scan_all_ports_remote_disabled),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextTertiary,
            )
        }
    }
}

/** "Escaneo completo · hace 5 min" o, si ya pasó más de una hora, con fecha y hora. */
@Composable
private fun fullScanLabel(finishedAt: Instant?): String {
    if (finishedAt == null) return stringResource(R.string.detail_full_scan_title)
    val relative = relativeTimeFrom(finishedAt)
    // Una hora o más (Hours o Days: ver la revisión de la fase 5) se lee mejor
    // con fecha y hora que "hace 30 h"/"hace 2 d" para un escaneo puntual.
    val whenText = if (relative is RelativeTime.Hours || relative is RelativeTime.Days) {
        formatDateTime(finishedAt, DateFormat.is24HourFormat(LocalContext.current))
    } else {
        agoLabel(relative)
    }
    return stringResource(R.string.detail_full_scan_done, whenText)
}

/** Puertos como chips: los de riesgo en rojo con icono de advertencia. */
@Composable
private fun PortChips(ports: List<Int>) {
    androidx.compose.foundation.layout.FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        ports.sorted().forEach { port -> PortChip(port) }
    }
}

@Composable
private fun PortChip(port: Int) {
    val risky = PortCatalog.isRisky(port)
    val color = if (risky) VigiaColors.Risk else VigiaColors.TextSecondary
    Surface(
        shape = RoundedCornerShape(50),
        color = if (risky) VigiaColors.Risk.copy(alpha = 0.12f) else VigiaColors.SurfaceElevated,
        border = androidx.compose.foundation.BorderStroke(1.dp, if (risky) VigiaColors.Risk.copy(alpha = 0.5f) else VigiaColors.Outline),
        // Fusionado: el riesgo hoy solo se ve por color/icono
        // (accesibilidad) — el icono lleva el estado en texto.
        modifier = Modifier.semantics(mergeDescendants = true) {},
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            if (risky) {
                Icon(Icons.Default.Warning, contentDescription = stringResource(R.string.devices_port_risky), tint = color, modifier = Modifier.size(14.dp))
            }
            Text("$port · ${PortCatalog.localizedNameFor(port)}", style = VigiaMono.label, color = color)
        }
    }
}
