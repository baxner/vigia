package com.baxner.vigia.ui.permissions

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.core.content.ContextCompat

/**
 * El permiso de notificaciones (ver el encargo, punto 2): no existe antes de
 * Android 13, así que ahí siempre "concedido". Separado de [PermissionsState]
 * (ubicación/Wi-Fi, pedidos todos juntos al abrir la app) porque este se pide
 * en un momento distinto: al vincular el agente o al activar alertas, nunca
 * al primer abrir (ver `ui/alerts/AlertsScreen.kt`).
 */
fun hasNotificationPermission(context: Context): Boolean =
    Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
        ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

/**
 * Da una función para pedir el permiso de notificaciones (no-op en Android
 * <13, donde ni existe). [onResult] se llama con el resultado del diálogo del
 * sistema (concedido o no).
 */
@Composable
fun rememberNotificationPermissionRequester(onResult: (Boolean) -> Unit = {}): () -> Unit {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return {}
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> onResult(granted) }
    return { launcher.launch(Manifest.permission.POST_NOTIFICATIONS) }
}
