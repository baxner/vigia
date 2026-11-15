package com.baxner.vigia.ui.bluetooth

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.network.rssiToBars
import com.baxner.vigia.data.radar.BleDevice
import com.baxner.vigia.data.radar.BleScanner
import com.baxner.vigia.data.radar.DistanceModel
import com.baxner.vigia.data.radar.MacKinship
import com.baxner.vigia.data.radar.RadarModule
import com.baxner.vigia.data.radar.SignalSourceState
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.SignalBars
import com.baxner.vigia.ui.components.StatusChip
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.devices.displayName
import com.baxner.vigia.ui.permissions.rememberBluetoothScanPermissionState
import com.baxner.vigia.ui.radar.formatDistanceRangeLabel
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.relativeTimeFrom
import kotlinx.coroutines.awaitCancellation
import java.time.Instant

/**
 * Envoltura con estado de la pestaña Bluetooth (fase 6, ver docs/plan.md):
 * crea [BluetoothViewModel] con el [BleScanner] real de [RadarModule]
 * (sustituible en pruebas/capturas vía el parámetro [bleScanner]) y lo
 * gobierna con [repeatOnLifecycle] — escanea solo mientras la pestaña está
 * visible. El parentesco de MAC ("Parece ser tu...") usa los equipos
 * conocidos de la misma [scanViewModel] que comparten Panel/Dispositivos.
 */
@Composable
fun BluetoothScreen(
    scanViewModel: ScanViewModel,
    hasLocationPermission: Boolean,
    onRequestLocationPermission: () -> Unit,
    onHunt: (SignalTarget.Ble) -> Unit,
    modifier: Modifier = Modifier,
    bleScanner: BleScanner? = null,
) {
    val context = LocalContext.current
    val effectiveBleScanner = bleScanner ?: remember { RadarModule.bleScanner(context) }
    val bluetoothViewModel = remember(effectiveBleScanner) { BluetoothViewModel(effectiveBleScanner) }
    DisposableEffect(bluetoothViewModel) {
        onDispose { bluetoothViewModel.onCleared() }
    }
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(bluetoothViewModel, lifecycleOwner) {
        lifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
            bluetoothViewModel.start()
            try {
                awaitCancellation()
            } finally {
                bluetoothViewModel.stop()
            }
        }
    }

    val uiState by bluetoothViewModel.uiState.collectAsStateWithLifecycle()
    val knownDevices by scanViewModel.knownDevices.collectAsStateWithLifecycle()
    val macToName = remember(knownDevices) {
        knownDevices.mapNotNull { kd -> kd.mac?.takeIf { it.isNotBlank() }?.let { it to kd.displayName() } }
    }

    val (bluetoothScanGranted, requestBluetoothScanPermission) = rememberBluetoothScanPermissionState()
    val hasPermission = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) bluetoothScanGranted else hasLocationPermission
    val onRequestPermission = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) requestBluetoothScanPermission else onRequestLocationPermission

    BluetoothScreenContent(
        uiState = uiState,
        hasPermission = hasPermission,
        onRequestPermission = onRequestPermission,
        knownDevices = macToName,
        onToggleOnlyNamed = bluetoothViewModel::setOnlyNamed,
        onHunt = onHunt,
        modifier = modifier,
    )
}

/** Versión sin estado (capturas, tests): ver [BluetoothScreen] para la envoltura real. */
@Composable
fun BluetoothScreenContent(
    uiState: BluetoothUiState,
    hasPermission: Boolean,
    onRequestPermission: () -> Unit,
    knownDevices: List<Pair<String, String>>,
    onToggleOnlyNamed: (Boolean) -> Unit,
    onHunt: (SignalTarget.Ble) -> Unit,
    modifier: Modifier = Modifier,
    now: Instant = Instant.now(),
) {
    val visible = remember(uiState.devices, uiState.onlyNamed) { sortedBleDevices(uiState.devices, uiState.onlyNamed) }

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") {
            Text(stringResource(R.string.bluetooth_title), style = MaterialTheme.typography.titleLarge, color = VigiaColors.TextPrimary)
        }

        if (!hasPermission) {
            item(key = "permission") {
                SectionCard(title = stringResource(R.string.permissions_title)) {
                    Text(stringResource(R.string.bluetooth_permission_explanation), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                    Button(
                        onClick = onRequestPermission,
                        colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
                    ) { Text(stringResource(R.string.permissions_grant)) }
                }
            }
        } else if (uiState.state == SignalSourceState.RADIO_OFF) {
            item(key = "radio_off") {
                EmptyState(
                    title = stringResource(R.string.bluetooth_radio_off_title),
                    subtitle = stringResource(R.string.bluetooth_radio_off_subtitle),
                )
            }
        }

        if (hasPermission) {
            item(key = "filter") {
                Row(
                    modifier = Modifier.fillMaxWidth().toggleable(value = uiState.onlyNamed, onValueChange = onToggleOnlyNamed, role = Role.Switch),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(stringResource(R.string.bluetooth_only_named), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
                    Switch(checked = uiState.onlyNamed, onCheckedChange = null)
                }
            }

            if (visible.isEmpty() && uiState.state == SignalSourceState.OK) {
                item(key = "empty") {
                    EmptyState(title = stringResource(R.string.bluetooth_empty), modifier = Modifier.fillMaxWidth().padding(top = 32.dp))
                }
            } else {
                items(visible, key = { it.address }) { device ->
                    val siblingName = if (!device.randomAddress) MacKinship.siblingName(device.address, knownDevices) else null
                    BleDeviceRow(device = device, siblingName = siblingName, now = now, onHunt = onHunt)
                }
            }
        }
    }
}

/** Orden por señal (más fuerte primero) y, si [onlyNamed], solo los que anunciaron un nombre. */
internal fun sortedBleDevices(devices: List<BleDevice>, onlyNamed: Boolean): List<BleDevice> =
    devices.filter { !onlyNamed || !it.name.isNullOrBlank() }.sortedByDescending { it.rssi }

/** El [SignalTarget.Ble] de este equipo: la etiqueta prioriza el parentesco ("Tira Govee") sobre el nombre anunciado. */
internal fun bleSignalTargetFor(device: BleDevice, siblingName: String?): SignalTarget.Ble {
    val label = siblingName ?: device.name?.takeIf { it.isNotBlank() } ?: device.brand?.takeIf { it.isNotBlank() } ?: device.address
    return SignalTarget.Ble(address = device.address, name = device.name, brand = device.brand, txPowerDbm = device.txPowerDbm, label = label)
}

@Composable
private fun BleDeviceRow(device: BleDevice, siblingName: String?, now: Instant, onHunt: (SignalTarget.Ble) -> Unit) {
    val target = remember(device, siblingName) { bleSignalTargetFor(device, siblingName) }
    val distance = remember(device) { DistanceModel.estimate(device.rssi.toDouble(), DistanceModel.referenceAt1m(target)) }
    val relative = remember(device.lastSeenMillis, now) { relativeTimeFrom(Instant.ofEpochMilli(device.lastSeenMillis), now) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(VigiaColors.Surface, RoundedCornerShape(16.dp))
            .border(1.dp, VigiaColors.Outline, RoundedCornerShape(16.dp))
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SignalBars(level = rssiToBars(device.rssi))
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                device.name?.takeIf { it.isNotBlank() } ?: stringResource(R.string.bluetooth_no_name),
                style = MaterialTheme.typography.titleSmall,
                color = VigiaColors.TextPrimary,
            )
            if (device.randomAddress) {
                StatusChip(text = stringResource(R.string.bluetooth_random_address), dotColor = VigiaColors.Amber)
            }
            device.brand?.takeIf { it.isNotBlank() }?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            }
            siblingName?.let {
                Text(stringResource(R.string.bluetooth_sibling_match, it), style = MaterialTheme.typography.bodySmall, color = VigiaColors.RadarCyan)
            }
            Text(formatDistanceRangeLabel(distance), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            Text(stringResource(R.string.devices_seen_relative, agoLabel(relative)), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
        }
        Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("${device.rssi} dBm", style = VigiaMono.label, color = VigiaColors.TextSecondary)
            OutlinedButton(onClick = { onHunt(target) }) {
                Text(stringResource(R.string.radar_search_button))
            }
        }
    }
}
