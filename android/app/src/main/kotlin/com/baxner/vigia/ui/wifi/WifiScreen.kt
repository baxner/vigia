package com.baxner.vigia.ui.wifi

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.WifiBand
import com.baxner.vigia.data.network.WifiNetworkInfo
import com.baxner.vigia.data.network.WifiSecurity
import com.baxner.vigia.data.network.WifiStandard
import com.baxner.vigia.data.network.apCountForSsid
import com.baxner.vigia.data.network.ipFromCidr
import com.baxner.vigia.data.network.rssiToBars
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.SignalBars
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import kotlin.math.exp

@Composable
fun WifiScreen(
    wifiViewModel: WifiViewModel,
    hasLocationPermission: Boolean,
    onRequestPermissions: () -> Unit,
    modifier: Modifier = Modifier,
    demoMode: Boolean = false,
    // "Buscar señal" (fase 6, ver docs/plan.md): sin acción por defecto para no
    // romper las pruebas/capturas que todavía no conectan la navegación.
    onHunt: (SignalTarget.WifiAp) -> Unit = {},
) {
    val uiState by wifiViewModel.uiState.collectAsStateWithLifecycle()

    // Escanea al entrar (y en cuanto se conceda el permiso), no al abrir la app.
    androidx.compose.runtime.LaunchedEffect(hasLocationPermission, demoMode) {
        if (!demoMode && hasLocationPermission) wifiViewModel.rescanIfStale()
    }

    val connected = remember(uiState.networks) { uiState.networks.firstOrNull { it.isConnected } }
    val meshApCount = remember(uiState.networks, connected) {
        apCountForSsid(uiState.networks, connected?.ssid ?: "")
    }

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "hero") { ConnectedHero(connected, uiState.currentNetwork, meshApCount) }

        item(key = "header") {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(stringResource(R.string.wifi_networks_title), style = MaterialTheme.typography.titleLarge, color = VigiaColors.TextPrimary)
                if (uiState.isScanning) {
                    CircularProgressIndicator(modifier = Modifier.size(22.dp), strokeWidth = 2.dp, color = VigiaColors.RadarCyan)
                } else {
                    IconButton(onClick = wifiViewModel::rescan) {
                        Icon(Icons.Default.Refresh, contentDescription = stringResource(R.string.wifi_rescan), tint = VigiaColors.TextSecondary)
                    }
                }
            }
        }

        if (!demoMode && !hasLocationPermission) {
            item(key = "permission") {
                SectionCard(title = stringResource(R.string.permissions_title)) {
                    Text(stringResource(R.string.wifi_needs_location), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                    Button(
                        onClick = onRequestPermissions,
                        colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
                    ) { Text(stringResource(R.string.permissions_grant)) }
                }
            }
        }

        if (uiState.throttled) {
            item(key = "throttled") {
                Text(stringResource(R.string.wifi_throttled), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            }
        }

        if (uiState.scanThrottleEnabled) {
            item(key = "scanThrottleEnabled") {
                Text(stringResource(R.string.wifi_scan_throttle_enabled), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            }
        }

        uiState.errorMessage?.let { message ->
            item(key = "error") {
                Text(stringResource(R.string.wifi_error_prefix, message), color = VigiaColors.Risk)
            }
        }

        if (uiState.networks.isNotEmpty()) {
            item(key = "chart") {
                SectionCard(title = stringResource(R.string.wifi_channel_chart_title)) {
                    ChannelAnalyzer(uiState.networks)
                }
            }
        }

        if (uiState.networks.isEmpty()) {
            if (!uiState.isScanning) {
                item(key = "empty") {
                    EmptyState(title = stringResource(R.string.wifi_empty), modifier = Modifier.fillMaxWidth().padding(top = 32.dp))
                }
            }
        } else {
            items(uiState.networks, key = { it.bssid.ifBlank { "${it.ssid}-${it.frequencyMhz}" } }) { network ->
                val isYourNetwork = connected != null && connected.ssid.isNotBlank() &&
                    network.ssid == connected.ssid && !network.isConnected
                WifiNetworkRow(network, isYourNetwork = isYourNetwork, onHunt = onHunt, modifier = Modifier.animateItem())
            }
        }
    }
}

/**
 * Héroe de la red conectada: SSID, BSSID+marca, banda/canal/centro/ancho,
 * velocidad de enlace e IP (de [CurrentNetwork], que [WifiNetworkInfo] no
 * tiene), distancia estimada, aviso de red mesh y advertencias de seguridad.
 */
@Composable
private fun ConnectedHero(connected: WifiNetworkInfo?, currentNetwork: CurrentNetwork?, meshApCount: Int) {
    SectionCard(title = "", glow = connected != null) {
        if (connected == null) {
            Text(stringResource(R.string.wifi_hero_no_connection), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
            return@SectionCard
        }
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(
                connected.ssid.ifBlank { stringResource(R.string.wifi_hidden_ssid) },
                style = MaterialTheme.typography.headlineSmall,
                color = VigiaColors.TextPrimary,
            )
            val vendorSuffix = if (connected.vendor.isNotBlank()) " · ${connected.vendor}" else ""
            Text(
                connected.bssid.ifBlank { "—" } + vendorSuffix,
                style = VigiaMono.body,
                color = VigiaColors.TextSecondary,
            )
            val standardSuffix = connected.wifiStandard?.let { "   ·   ${it.label}" } ?: ""
            Text(
                "${channelSummary(connected)}$standardSuffix",
                style = VigiaMono.label,
                color = VigiaColors.TextSecondary,
            )
            val linkSpeedText = currentNetwork?.linkSpeedMbps?.let { "$it Mbps" }
            val ipText = ipFromCidr(currentNetwork?.cidr)
            if (linkSpeedText != null || ipText != null) {
                Text(
                    listOfNotNull(linkSpeedText, ipText).joinToString("   ·   "),
                    style = VigiaMono.label,
                    color = VigiaColors.TextSecondary,
                )
            }
            Text(
                stringResource(R.string.wifi_hero_distance, connected.distanceLabel),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextSecondary,
            )
            if (meshApCount > 1) {
                Text(
                    stringResource(R.string.wifi_hero_mesh, meshApCount),
                    style = MaterialTheme.typography.bodySmall,
                    color = VigiaColors.ElectricBlue,
                )
            }
            SignalMeter(rssi = connected.rssiDbm)
            HeroSecurityWarnings(connected)
        }
    }
}

/**
 * "Red abierta"/"WEP o WPA" en rojo/ámbar y "WPS activo" en ámbar, si
 * aplican: la seguridad de TU red merece destacarse en el héroe, no solo en
 * la insignia chica de la lista.
 */
@Composable
private fun HeroSecurityWarnings(connected: WifiNetworkInfo) {
    val isWeakSecurity = connected.security == WifiSecurity.WEP || connected.security == WifiSecurity.WPA
    if (connected.security != WifiSecurity.OPEN && !isWeakSecurity && !connected.hasWps) return
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when {
            connected.security == WifiSecurity.OPEN ->
                Text(stringResource(R.string.wifi_hero_warning_open), style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
            isWeakSecurity ->
                Text(
                    stringResource(R.string.wifi_hero_warning_weak, connected.security.label),
                    style = MaterialTheme.typography.bodySmall,
                    color = VigiaColors.Amber,
                )
        }
        if (connected.hasWps) {
            Text(stringResource(R.string.wifi_hero_warning_wps), style = MaterialTheme.typography.bodySmall, color = VigiaColors.Amber)
        }
    }
}

/** Medidor de señal: una barra de degradado (rojo→ámbar→cian) más las barritas y el dBm. */
@Composable
private fun SignalMeter(rssi: Int) {
    val fraction = ((rssi + 90).coerceIn(0, 60) / 60f).coerceIn(0.04f, 1f)
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(8.dp)
                .background(VigiaColors.Outline, RoundedCornerShape(4.dp)),
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(fraction)
                    .height(8.dp)
                    .background(
                        Brush.horizontalGradient(listOf(VigiaColors.Risk, VigiaColors.Amber, VigiaColors.RadarCyan)),
                        RoundedCornerShape(4.dp),
                    ),
            )
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            SignalBars(level = rssiToBars(rssi))
            Text("$rssi dBm", style = VigiaMono.body, color = VigiaColors.TextSecondary)
        }
    }
}

/** Generaciones Wi-Fi que se destacan con insignia en la lista (no 4/5: harían ruido en cada fila). */
private val wifiSixOrNewer = setOf(WifiStandard.WIFI_6, WifiStandard.WIFI_6E, WifiStandard.WIFI_7)

/** "5 GHz · canal 48 · centro 50 · 160 MHz" (sin "· centro N" en 20 MHz, donde el canal ya es su propio centro). */
@Composable
private fun channelSummary(network: WifiNetworkInfo): String {
    val span = network.channelSpan
    return if (span != null) {
        stringResource(
            R.string.wifi_channel_summary_with_center,
            network.band.label,
            network.channel,
            span.centerChannel,
            network.channelWidthLabel,
        )
    } else {
        stringResource(
            R.string.wifi_channel_summary,
            network.band.label,
            network.channel,
            network.channelWidthLabel,
        )
    }
}

@Composable
private fun WifiNetworkRow(
    network: WifiNetworkInfo,
    isYourNetwork: Boolean,
    modifier: Modifier = Modifier,
    onHunt: (SignalTarget.WifiAp) -> Unit = {},
) {
    val displayName = network.ssid.ifBlank { stringResource(R.string.wifi_hidden_ssid) }
    Row(
        modifier = modifier
            .fillMaxWidth()
            .background(VigiaColors.Surface, RoundedCornerShape(16.dp))
            .border(1.dp, VigiaColors.Outline, RoundedCornerShape(16.dp))
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SignalBars(level = rssiToBars(network.rssiDbm))
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            // FlowRow, no Row con weight (ver AGENTS.md § accesibilidad,
            // "letra grande"): con la columna de la derecha (distancia/dBm/
            // "Buscar") creciendo por la letra grande, un Row aplastaba el
            // nombre de la red hasta dejarlo casi invisible.
            androidx.compose.foundation.layout.FlowRow(
                verticalArrangement = Arrangement.spacedBy(2.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Text(
                    displayName,
                    style = MaterialTheme.typography.titleSmall,
                    color = VigiaColors.TextPrimary,
                    maxLines = 1,
                )
                if (network.isConnected) {
                    Surface(shape = RoundedCornerShape(50), color = VigiaColors.RadarCyan.copy(alpha = 0.18f)) {
                        Text(
                            stringResource(R.string.wifi_connected_badge),
                            style = MaterialTheme.typography.labelSmall,
                            color = VigiaColors.RadarCyan,
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
                        )
                    }
                } else if (isYourNetwork) {
                    YourNetworkBadge()
                }
            }
            val vendorSuffix = if (network.vendor.isNotBlank()) " · ${network.vendor}" else ""
            Text(
                network.bssid.ifBlank { "—" } + vendorSuffix,
                style = VigiaMono.labelSmall,
                color = VigiaColors.TextTertiary,
            )
            // FlowRow: con varias insignias (WPA2, Wi-Fi 6, WPS, RTT) y poco
            // ancho, un Row las aplastaba letra por letra ("W"/"P"/"S" en
            // columna) en vez de bajar la insignia completa a otra línea.
            androidx.compose.foundation.layout.FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                SecurityBadge(network.security)
                network.wifiStandard?.takeIf { it in wifiSixOrNewer }?.let { StandardBadge(it) }
                if (network.hasWps) WpsBadge()
                if (network.rttResponder) RttBadge()
            }
            Text(
                channelSummary(network),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextSecondary,
            )
        }
        Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(network.distanceLabel, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
                Text("${network.rssiDbm} dBm", style = VigiaMono.label, color = VigiaColors.TextSecondary)
            }
            OutlinedButton(onClick = {
                onHunt(
                    SignalTarget.WifiAp(
                        bssid = network.bssid,
                        ssid = network.ssid,
                        frequencyMhz = network.frequencyMhz,
                        rttCapable = network.rttResponder,
                        connected = network.isConnected,
                        label = displayName,
                    ),
                )
            }) { Text(stringResource(R.string.radar_search_button)) }
        }
    }
}

/** Insignia para redes con el mismo SSID que la conectada, pero otro BSSID (repetidor/mesh). */
@Composable
private fun YourNetworkBadge() {
    Surface(
        shape = RoundedCornerShape(50),
        color = VigiaColors.RadarCyan.copy(alpha = 0.10f),
        border = BorderStroke(1.dp, VigiaColors.RadarCyan.copy(alpha = 0.4f)),
    ) {
        Text(
            stringResource(R.string.wifi_your_network_badge),
            style = MaterialTheme.typography.labelSmall,
            color = VigiaColors.RadarCyan,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}

/** Insignia de generación Wi-Fi (solo se llama para Wi-Fi 6/6E/7, ver [wifiSixOrNewer]). */
@Composable
private fun StandardBadge(standard: WifiStandard) {
    Surface(
        shape = RoundedCornerShape(50),
        color = VigiaColors.ElectricBlue.copy(alpha = 0.14f),
        border = BorderStroke(1.dp, VigiaColors.ElectricBlue.copy(alpha = 0.55f)),
    ) {
        Text(
            standard.label,
            style = MaterialTheme.typography.labelSmall,
            color = VigiaColors.ElectricBlue,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}

/** Insignia cian de Wi-Fi RTT: con este AP, "Buscar señal" mide la distancia (1–2 m) en vez de estimarla. */
@Composable
private fun RttBadge() {
    Surface(
        shape = RoundedCornerShape(50),
        color = VigiaColors.RadarCyan.copy(alpha = 0.14f),
        border = BorderStroke(1.dp, VigiaColors.RadarCyan.copy(alpha = 0.55f)),
    ) {
        Text(
            stringResource(R.string.wifi_rtt_badge),
            style = MaterialTheme.typography.labelSmall,
            color = VigiaColors.RadarCyan,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}

/** Insignia ámbar de WPS: tocarla abre el detalle con la explicación (ver [WpsDetailDialog]). */
@Composable
private fun WpsBadge() {
    var showDetail by remember { mutableStateOf(false) }
    Surface(
        shape = RoundedCornerShape(50),
        color = VigiaColors.Amber.copy(alpha = 0.14f),
        border = BorderStroke(1.dp, VigiaColors.Amber.copy(alpha = 0.55f)),
        // Insignia chica (ver AGENTS.md § accesibilidad): la acción de
        // tocarla, no solo el texto visible "WPS". Sin
        // minimumInteractiveComponentSize() a propósito: agrandar el área
        // táctil de un chip suelto en esta fila (junto a otros del mismo
        // tamaño) correría el resto de insignias y cambiaría la captura con
        // letra normal (ver el encargo: "con letra normal, el diseño no debe
        // cambiar").
        modifier = Modifier.clickable(onClickLabel = stringResource(R.string.wifi_wps_badge_action)) { showDetail = true },
    ) {
        Text(
            stringResource(R.string.wifi_wps_badge),
            style = MaterialTheme.typography.labelSmall,
            color = VigiaColors.Amber,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
    if (showDetail) {
        WpsDetailDialog(onDismiss = { showDetail = false })
    }
}

/** Qué es WPS y por qué conviene apagarlo, en el mismo estilo de diálogo que el resto de la app. */
@Composable
private fun WpsDetailDialog(onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.wifi_wps_detail_title)) },
        text = { Text(stringResource(R.string.wifi_wps_detail_message)) },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.wifi_wps_detail_dismiss)) }
        },
        containerColor = VigiaColors.SurfaceElevated,
        titleContentColor = VigiaColors.TextPrimary,
        textContentColor = VigiaColors.TextSecondary,
    )
}

/** Color de la insignia de seguridad: abierta en rojo, WEP/WPA en ámbar, WPA2/WPA3 en verde. */
private fun WifiSecurity.badgeColor(): Color = when (this) {
    WifiSecurity.OPEN -> VigiaColors.Risk
    WifiSecurity.WEP, WifiSecurity.WPA -> VigiaColors.Amber
    WifiSecurity.WPA2, WifiSecurity.WPA3 -> VigiaColors.Ok
    WifiSecurity.UNKNOWN -> VigiaColors.TextTertiary
}

@Composable
private fun SecurityBadge(security: WifiSecurity) {
    val color = security.badgeColor()
    Surface(
        shape = RoundedCornerShape(50),
        color = color.copy(alpha = 0.14f),
        border = BorderStroke(1.dp, color.copy(alpha = 0.55f)),
    ) {
        Text(
            security.label,
            style = MaterialTheme.typography.labelSmall,
            color = color,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}

/** Dominio de canales mostrado en el analizador, por banda. */
private fun domainFor(band: WifiBand): IntRange = when (band) {
    WifiBand.GHZ_2_4 -> 1..14
    WifiBand.GHZ_5 -> 32..169
    else -> 1..14
}

private fun ticksFor(band: WifiBand): List<Int> = when (band) {
    WifiBand.GHZ_2_4 -> listOf(1, 6, 11, 14)
    WifiBand.GHZ_5 -> listOf(36, 52, 100, 132, 149, 165)
    else -> emptyList()
}

private val analyzerBands = listOf(WifiBand.GHZ_2_4, WifiBand.GHZ_5)

/**
 * Analizador de canales tipo Wi-Fi: una curva (campana) por red sobre el eje
 * de canales, 2.4 GHz y 5 GHz por separado, con la red conectada resaltada
 * en cian. El ancho de canal ensancha la campana; el RSSI define su altura.
 */
@Composable
private fun ChannelAnalyzer(networks: List<WifiNetworkInfo>) {
    val byBand = networks.groupBy { it.band }
    Column(verticalArrangement = Arrangement.spacedBy(18.dp)) {
        analyzerBands.forEach { band ->
            val inBand = byBand[band].orEmpty()
            if (inBand.isNotEmpty()) {
                Column {
                    Text(band.label, style = MaterialTheme.typography.labelLarge, color = VigiaColors.TextSecondary)
                    Spacer(Modifier.height(4.dp))
                    BandCurveChart(band = band, networks = inBand)
                }
            }
        }
    }
}

@Composable
private fun BandCurveChart(band: WifiBand, networks: List<WifiNetworkInfo>) {
    val domain = domainFor(band)
    val ticks = ticksFor(band)
    val textMeasurer = rememberTextMeasurer()
    val tickStyle = remember { TextStyle(fontFamily = com.baxner.vigia.ui.theme.JetBrainsMono, fontSize = 9.sp) }

    // Descripción resumida para TalkBack (ver AGENTS.md § accesibilidad): no
    // lee la curva punto por punto, solo cuántas redes hay en esta banda y,
    // si una está conectada, en qué canal.
    val connected = networks.firstOrNull { it.isConnected }
    val chartDescription = if (connected != null) {
        stringResource(R.string.wifi_channel_band_summary_connected, band.label, networks.size, connected.channel)
    } else {
        stringResource(R.string.wifi_channel_band_summary, band.label, networks.size)
    }

    Canvas(modifier = Modifier.fillMaxWidth().height(96.dp).semantics { contentDescription = chartDescription }) {
        val w = size.width
        val baselineY = size.height - 16.dp.toPx()
        val usableH = baselineY - 8.dp.toPx()
        val domainSpan = (domain.last - domain.first).toFloat().coerceAtLeast(1f)
        fun xFor(channel: Float) = ((channel - domain.first) / domainSpan) * w

        drawLine(VigiaColors.Outline, Offset(0f, baselineY), Offset(w, baselineY), strokeWidth = 1.dp.toPx())

        networks.forEach { net ->
            val color = if (net.isConnected) VigiaColors.RadarCyan else VigiaColors.ElectricBlue
            val amplitude = (((net.rssiDbm + 90).coerceIn(0, 65) / 65f) * 0.8f + 0.2f)
            val widthMhz = net.channelWidthLabel.substringBefore(' ').toIntOrNull() ?: 20
            val sigma = ((widthMhz / 20f).coerceAtLeast(0.7f)) * 1.15f

            val path = Path()
            val steps = 70
            for (i in 0..steps) {
                val x = domain.first + domainSpan * (i.toFloat() / steps)
                val t = (x - net.channel) / sigma
                val y = baselineY - usableH * amplitude * exp(-(t * t) / 2f)
                val px = xFor(x)
                if (i == 0) path.moveTo(px, y) else path.lineTo(px, y)
            }
            path.lineTo(xFor(domain.last.toFloat()), baselineY)
            path.lineTo(xFor(domain.first.toFloat()), baselineY)
            path.close()

            drawPath(path, color = color.copy(alpha = if (net.isConnected) 0.22f else 0.10f))
            drawPath(
                path,
                color = color.copy(alpha = if (net.isConnected) 0.95f else 0.6f),
                style = Stroke(width = (if (net.isConnected) 2f else 1.3f).dp.toPx()),
            )
        }

        ticks.filter { it in domain }.forEach { channel ->
            val label = textMeasurer.measure(channel.toString(), style = tickStyle)
            drawText(
                textLayoutResult = label,
                color = VigiaColors.TextTertiary,
                topLeft = Offset(xFor(channel.toFloat()) - label.size.width / 2f, baselineY + 3.dp.toPx()),
            )
        }
    }
}
