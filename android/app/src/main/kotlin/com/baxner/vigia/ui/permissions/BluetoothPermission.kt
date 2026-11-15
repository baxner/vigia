package com.baxner.vigia.ui.permissions

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LifecycleResumeEffect

/**
 * Permiso para escanear Bluetooth LE (fase 6, pestaña Bluetooth, ver
 * docs/plan.md): BLUETOOTH_SCAN en Android 12+ (31+); en 11 y anteriores no
 * existe ese permiso y lo que de verdad exige el escaneo BLE es la ubicación,
 * que [PermissionsState] ya pide al abrir la app (ver AndroidManifest.xml).
 * Separado de [PermissionsState] porque BLUETOOTH_SCAN se pide en contexto,
 * al abrir la pestaña Bluetooth, no junto a los demás permisos al abrir la app.
 */
fun hasBluetoothScanPermission(context: Context): Boolean =
    Build.VERSION.SDK_INT < Build.VERSION_CODES.S ||
        ContextCompat.checkSelfPermission(context, Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED

/**
 * Estado de BLUETOOTH_SCAN (solo relevante en Android 12+; en versiones
 * viejas [primero][Pair.first] siempre sale `true` porque ahí lo que manda es
 * la ubicación, ver [hasBluetoothScanPermission]) y una función para pedirlo.
 * Igual que [rememberPermissionsState]: se refresca solo al volver de Ajustes.
 */
@Composable
fun rememberBluetoothScanPermissionState(): Pair<Boolean, () -> Unit> {
    val context = LocalContext.current
    var granted by remember { mutableStateOf(hasBluetoothScanPermission(context)) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { result ->
        granted = result
    }
    LifecycleResumeEffect(Unit) {
        granted = hasBluetoothScanPermission(context)
        onPauseOrDispose { }
    }
    val request: () -> Unit = {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) launcher.launch(Manifest.permission.BLUETOOTH_SCAN)
    }
    return granted to request
}
