package com.baxner.vigia.ui.devices

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Star
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.model.DeviceType
import com.baxner.vigia.data.model.isSelf
import com.baxner.vigia.ui.components.ConfidenceRing
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.relativeTimeFrom
import java.time.Instant

/**
 * Envoltura fina sobre [DevicesScreenContent]: separadas para poder
 * capturar el contenido con datos de muestra sin un [ScanViewModel] real
 * (ver la misma nota en PanelScreen.kt).
 */
@Composable
fun DevicesScreen(
    scanViewModel: ScanViewModel,
    onDeviceClick: (id: Long) -> Unit,
    modifier: Modifier = Modifier,
) {
    val knownDevices by scanViewModel.knownDevices.collectAsStateWithLifecycle()
    val uiState by scanViewModel.uiState.collectAsStateWithLifecycle()
    val agentState by scanViewModel.agentPanelState.collectAsStateWithLifecycle()
    val viewingRemote = (agentState as? AgentPanelState.Linked)?.viewingRemote == true
    DevicesScreenContent(
        knownDevices = knownDevices,
        gateway = uiState.result?.gateway.orEmpty().ifEmpty {
            if (viewingRemote) "" else uiState.network?.gateway.orEmpty()
        },
        phoneIp = if (viewingRemote) null else uiState.network?.cidr?.substringBefore('/'),
        onDeviceClick = onDeviceClick,
        modifier = modifier,
    )
}

/**
 * Dispositivos sale de [knownDevices] (el historial de la red actual, ver
 * [ScanViewModel.knownDevices]), no del último escaneo: por eso sigue viendo
 * los equipos sin conexión. Dos secciones, en línea primero.
 */
@Composable
fun DevicesScreenContent(
    knownDevices: List<KnownDevice>,
    gateway: String,
    onDeviceClick: (id: Long) -> Unit,
    modifier: Modifier = Modifier,
    /** IP de este teléfono en la red: así se le pone "Este teléfono" también en modo agente. */
    phoneIp: String? = null,
    now: Instant = remember { Instant.now() },
) {
    var query by rememberSaveable { mutableStateOf("") }
    var selectedTypeRaw by rememberSaveable { mutableStateOf<String?>(null) }

    val baseline = remember(knownDevices) { baselineOf(knownDevices) }
    val typesPresent = remember(knownDevices) {
        knownDevices.map { it.coarseType() }.distinct()
    }
    val filtered = remember(knownDevices, query, selectedTypeRaw) {
        knownDevices.filter { kd ->
            val matchesType = selectedTypeRaw == null || kd.coarseType().name == selectedTypeRaw
            val matchesQuery = query.isBlank() ||
                listOf(kd.displayName(), kd.lastIp, kd.last.vendor, kd.last.identity.brand, kd.last.identity.model, kd.custom.location.orEmpty())
                    .any { it.contains(query, ignoreCase = true) }
            matchesType && matchesQuery
        }
    }
    val online = remember(filtered) { filtered.filter { it.online }.sortedBy { it.displayName().lowercase() } }
    val offline = remember(filtered) { filtered.filterNot { it.online }.sortedByDescending { it.lastSeen } }

    Column(modifier = modifier.fillMaxSize()) {
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
            placeholder = { Text(stringResource(R.string.devices_search_hint), color = VigiaColors.TextTertiary) },
            leadingIcon = { Icon(Icons.Default.Search, contentDescription = null, tint = VigiaColors.TextSecondary) },
            singleLine = true,
            shape = RoundedCornerShape(16.dp),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = VigiaColors.RadarCyan,
                unfocusedBorderColor = VigiaColors.Outline,
                focusedTextColor = VigiaColors.TextPrimary,
                unfocusedTextColor = VigiaColors.TextPrimary,
                cursorColor = VigiaColors.RadarCyan,
                focusedContainerColor = VigiaColors.Surface,
                unfocusedContainerColor = VigiaColors.Surface,
            ),
        )

        if (typesPresent.isNotEmpty()) {
            LazyRow(
                contentPadding = PaddingValues(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                item(key = "__all__") {
                    VigiaFilterChip(
                        selected = selectedTypeRaw == null,
                        onClick = { selectedTypeRaw = null },
                        label = stringResource(R.string.devices_filter_all),
                    )
                }
                items(typesPresent, key = { it.name }) { type ->
                    VigiaFilterChip(
                        selected = selectedTypeRaw == type.name,
                        onClick = { selectedTypeRaw = if (selectedTypeRaw == type.name) null else type.name },
                        label = type.label(),
                        icon = type.icon(),
                    )
                }
            }
        }

        when {
            knownDevices.isEmpty() -> EmptyState(
                title = stringResource(R.string.devices_empty),
                modifier = Modifier.fillMaxSize(),
            )
            filtered.isEmpty() -> EmptyState(
                title = stringResource(R.string.devices_empty_filtered),
                modifier = Modifier.fillMaxSize(),
            )
            else -> LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (online.isNotEmpty()) {
                    item(key = "__header_online__") {
                        SectionHeader(stringResource(R.string.devices_section_online, online.size))
                    }
                    items(online, key = { "on_${it.id}" }) { device ->
                        DeviceRow(
                            baseline = baseline,
                            device = device,
                            isGateway = device.lastIp == gateway,
                            isThisPhone = phoneIp != null && device.lastIp == phoneIp,
                            now = now,
                            onClick = { onDeviceClick(device.id) },
                            modifier = Modifier.animateItem(),
                        )
                    }
                }
                if (offline.isNotEmpty()) {
                    item(key = "__header_offline__") {
                        SectionHeader(stringResource(R.string.devices_section_offline, offline.size))
                    }
                    items(offline, key = { "off_${it.id}" }) { device ->
                        DeviceRow(
                            baseline = baseline,
                            device = device,
                            isGateway = false,
                            isThisPhone = phoneIp != null && device.lastIp == phoneIp,
                            now = now,
                            onClick = { onDeviceClick(device.id) },
                            modifier = Modifier.animateItem(),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SectionHeader(text: String) {
    Text(
        text = text.uppercase(),
        style = MaterialTheme.typography.labelMedium,
        color = VigiaColors.TextTertiary,
        modifier = Modifier.padding(start = 4.dp, top = 4.dp, bottom = 2.dp),
    )
}

@Composable
private fun VigiaFilterChip(
    selected: Boolean,
    onClick: () -> Unit,
    label: String,
    icon: androidx.compose.ui.graphics.vector.ImageVector? = null,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(label) },
        leadingIcon = icon?.let { { Icon(it, contentDescription = null) } },
        shape = RoundedCornerShape(50),
        colors = FilterChipDefaults.filterChipColors(
            containerColor = VigiaColors.Surface,
            labelColor = VigiaColors.TextSecondary,
            iconColor = VigiaColors.TextSecondary,
            selectedContainerColor = VigiaColors.RadarCyan.copy(alpha = 0.18f),
            selectedLabelColor = VigiaColors.RadarCyan,
            selectedLeadingIconColor = VigiaColors.RadarCyan,
        ),
        border = FilterChipDefaults.filterChipBorder(
            enabled = true,
            selected = selected,
            borderColor = VigiaColors.Outline,
            selectedBorderColor = VigiaColors.RadarCyan.copy(alpha = 0.5f),
        ),
    )
}

/**
 * Un equipo conocido como fila-tarjeta: icono teñido por tipo/estado (el
 * elegido a mano manda sobre el del motor, ver [coarseType]), nombre con
 * insignias ("Nuevo", "Este teléfono", "Router") y el icono de "Importante",
 * "Tipo · Marca" debajo, IP en mono y confianza (o "visto hace X" si está
 * sin conexión) a la derecha. Los sin conexión salen atenuados.
 */
@Composable
private fun DeviceRow(
    baseline: Instant?,
    device: KnownDevice,
    isGateway: Boolean,
    isThisPhone: Boolean,
    now: Instant,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val type = device.coarseType()
    val custom = device.custom
    // Sin nombre propio: el tipo ("Cámara") o "Sin identificar"; la IP ya va a la derecha.
    val name = device.nameOrNull()
        ?: if (type != DeviceType.UNKNOWN || custom.typeId != null) device.typeLabel() else stringResource(R.string.devices_unidentified)
    val muted = type == DeviceType.UNKNOWN && custom.typeId == null
    val accent = if (muted) VigiaColors.TextTertiary else device.statusColor()
    val textColor = if (muted) VigiaColors.TextSecondary else VigiaColors.TextPrimary
    val rowAlpha = if (device.online) 1f else 0.55f
    // Antes esto llamaba a R.string.devices_section_online/offline, que trae
    // "(%1$d)" (es para el encabezado de sección, no para una fila) y
    // TalkBack lo leía literal como "En línea (%1$d)": dos strings aparte,
    // sin el conteo, solo para el estado de ESTA fila.
    val onlineStatus = if (device.online) stringResource(R.string.devices_state_online) else stringResource(R.string.devices_state_offline)

    Row(
        modifier = modifier
            .fillMaxWidth()
            .alpha(rowAlpha)
            .clickable(onClick = onClick)
            .background(VigiaColors.Surface, RoundedCornerShape(16.dp))
            .border(1.dp, VigiaColors.Outline, RoundedCornerShape(16.dp))
            .padding(12.dp)
            // mergeDescendants: que TalkBack lea la fila completa de un
            // jalón ("TV de la sala, 192.168.0.118, en línea") en vez de
            // nombre, IP y badges como paradas sueltas.
            .semantics(mergeDescendants = true) {
                stateDescription = onlineStatus
            },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .background(accent.copy(alpha = if (muted) 0.10f else 0.16f), RoundedCornerShape(12.dp)),
            contentAlignment = Alignment.Center,
        ) {
            // Decorativo: con la fila fusionada, el nombre/tipo ya se lee en
            // el texto de abajo — repetir aquí duplicaría "Televisión" dos veces.
            Icon(device.icon(), contentDescription = null, tint = accent)
        }

        Column(modifier = Modifier.weight(1f)) {
            // FlowRow (no Row): con letra grande, la IP/anillo de confianza
            // de la derecha (accesibilidad, "letra grande")
            // ya ocupan más espacio y no dejaban casi nada para el nombre —
            // un Row con `weight` lo aplastaba hasta dejarlo en "…" porque
            // la insignia (ancho fijo) se medía primero. Con FlowRow, si no
            // cabe todo en una línea, la insignia baja a la siguiente y el
            // nombre se queda con el ancho completo.
            androidx.compose.foundation.layout.FlowRow(
                verticalArrangement = Arrangement.spacedBy(2.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Text(
                    name,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    style = MaterialTheme.typography.titleSmall,
                    color = textColor,
                )
                if (custom.important) {
                    Icon(
                        Icons.Default.Star,
                        contentDescription = stringResource(R.string.devices_badge_important),
                        tint = VigiaColors.Amber,
                        modifier = Modifier.size(14.dp),
                    )
                }
                when {
                    // "Este teléfono" por IP: en modo agente, `self` es el equipo del agente (el servidor).
                    isThisPhone -> Badge(stringResource(R.string.devices_badge_self))
                    device.last.isSelf -> Badge(stringResource(R.string.devices_badge_agent))
                    isGateway -> Badge(stringResource(R.string.devices_badge_gateway))
                    device.isNew(now, baseline) -> Badge(stringResource(R.string.devices_badge_new))
                }
            }
            val parts = buildList {
                if (device.nameOrNull() != null) add(device.typeLabel()) // si no, el tipo ya es el título
                val brand = device.last.identity.brand.ifBlank { device.last.vendor }
                if (brand.isNotBlank() && !name.contains(brand, ignoreCase = true)) add(brand)
                custom.location?.takeUnless { it.isBlank() }?.let { add(it) }
            }
            if (parts.isNotEmpty()) {
                Text(
                    parts.joinToString(" · "),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    style = MaterialTheme.typography.bodySmall,
                    color = VigiaColors.TextSecondary,
                )
            }
        }

        Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(device.lastIp, style = VigiaMono.label, color = VigiaColors.TextSecondary)
            if (device.online) {
                // Tipo elegido a mano (o por la app, para este teléfono): el % del motor no aplica.
                if (type != DeviceType.UNKNOWN && custom.typeId == null) {
                    ConfidenceRing(confidence = device.last.identity.confidence, size = 30.dp, strokeWidth = 2.5.dp)
                }
            } else {
                Text(
                    stringResource(R.string.devices_seen_relative, agoLabel(relativeTimeFrom(device.lastSeen, now))),
                    style = MaterialTheme.typography.labelSmall,
                    color = VigiaColors.TextTertiary,
                )
            }
        }
    }
}

@Composable
private fun Badge(text: String) {
    Surface(shape = RoundedCornerShape(50), color = VigiaColors.SurfaceElevated) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            color = VigiaColors.RadarCyan,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}
