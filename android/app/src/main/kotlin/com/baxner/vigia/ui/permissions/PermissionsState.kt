package com.baxner.vigia.ui.permissions

import android.Manifest
import android.annotation.SuppressLint
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
 * Permisos de runtime que necesita Vigía: ubicación (obligatoria para
 * SSID/BSSID real y escanear Wi-Fi) y, en Android 13+, "dispositivos
 * cercanos". La app debe andar sin ellos, solo que con menos información.
 */
class PermissionsState internal constructor(
    private val context: Context,
    private val relevantPermissions: List<String>,
    initialGranted: Map<String, Boolean>,
) {
    var granted: Map<String, Boolean> by mutableStateOf(initialGranted)
        internal set

    val hasLocation: Boolean
        get() = granted[Manifest.permission.ACCESS_FINE_LOCATION] == true

    // NEARBY_WIFI_DEVICES es API 33+, solo presente en relevantPermissions cuando SDK_INT >= 33.
    @get:SuppressLint("InlinedApi")
    val hasNearbyWifiDevices: Boolean
        get() = granted[Manifest.permission.NEARBY_WIFI_DEVICES] != false

    val allGranted: Boolean
        get() = relevantPermissions.all { granted[it] == true }

    internal fun refresh() {
        granted = relevantPermissions.associateWith {
            ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED
        }
    }
}

/** Qué pedirle al usuario según la versión de Android. */
@SuppressLint("InlinedApi")
private fun relevantPermissionsFor(sdkInt: Int): List<String> = buildList {
    add(Manifest.permission.ACCESS_FINE_LOCATION)
    // Android 12+ ignora una petición de FINE que no vaya junto con COARSE.
    add(Manifest.permission.ACCESS_COARSE_LOCATION)
    if (sdkInt >= Build.VERSION_CODES.TIRAMISU) {
        add(Manifest.permission.NEARBY_WIFI_DEVICES)
    }
}

/**
 * Da el estado de permisos y una función para pedirlos. No bloquea: quien la
 * use decide qué mostrar mientras no estén todos concedidos.
 */
@Composable
fun rememberPermissionsState(): Pair<PermissionsState, () -> Unit> {
    val context = LocalContext.current
    val permissions = remember { relevantPermissionsFor(Build.VERSION.SDK_INT) }
    val state = remember {
        PermissionsState(
            context = context,
            relevantPermissions = permissions,
            initialGranted = permissions.associateWith { perm ->
                ContextCompat.checkSelfPermission(context, perm) == PackageManager.PERMISSION_GRANTED
            },
        )
    }
    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { result ->
        state.granted = state.granted + result
    }
    // Al volver de Ajustes (o de cualquier otra app) el usuario pudo haber
    // cambiado los permisos a mano.
    LifecycleResumeEffect(state) {
        state.refresh()
        onPauseOrDispose { }
    }
    val request: () -> Unit = { launcher.launch(permissions.toTypedArray()) }
    return state to request
}
