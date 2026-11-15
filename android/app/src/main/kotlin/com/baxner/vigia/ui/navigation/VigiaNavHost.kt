package com.baxner.vigia.ui.navigation

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bluetooth
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Devices
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AgentPanelState
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.notifications.NotificationTargetAlerts
import com.baxner.vigia.ui.agent.AgentPairingScreen
import com.baxner.vigia.ui.agent.AgentSettingsScreen
import com.baxner.vigia.ui.agent.WebhooksScreen
import com.baxner.vigia.ui.alerts.AlertsScreen
import com.baxner.vigia.ui.bluetooth.BluetoothScreen
import com.baxner.vigia.ui.devices.DeviceDetailScreen
import com.baxner.vigia.ui.devices.DeviceEditScreen
import com.baxner.vigia.ui.devices.DevicesScreen
import com.baxner.vigia.ui.internet.InternetScreen
import com.baxner.vigia.ui.panel.PanelScreen
import com.baxner.vigia.ui.permissions.rememberBluetoothScanPermissionState
import com.baxner.vigia.ui.permissions.rememberPermissionsState
import com.baxner.vigia.ui.radar.HuntScreen
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.security.SecurityScreen
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.wifi.WifiScreen
import com.baxner.vigia.ui.wifi.WifiViewModel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull

private sealed class TopLevelDestination(val route: String, val labelRes: Int, val icon: ImageVector) {
    data object Panel : TopLevelDestination("panel", R.string.nav_panel, Icons.Default.Dashboard)
    data object Devices : TopLevelDestination("devices", R.string.nav_devices, Icons.Default.Devices)
    data object Wifi : TopLevelDestination("wifi", R.string.nav_wifi, Icons.Default.Wifi)
    data object Bluetooth : TopLevelDestination("bluetooth", R.string.nav_bluetooth, Icons.Default.Bluetooth)
}

private val topLevelDestinations =
    listOf(TopLevelDestination.Panel, TopLevelDestination.Devices, TopLevelDestination.Wifi, TopLevelDestination.Bluetooth)

/** "Buscar señal" (fase 6, ver docs/plan.md): ruta propia, fuera de las pestañas. */
private const val HUNT_ROUTE = "hunt"

// Por id estable (sobrevive a cambios de IP), no por IP: ver KnownDevice.id.
private const val DEVICE_DETAIL_ROUTE = "devices/{deviceId}"
private fun deviceDetailRoute(id: Long) = "devices/$id"

private const val DEVICE_EDIT_ROUTE = "devices/{deviceId}/edit"
private fun deviceEditRoute(id: Long) = "devices/$id/edit"

private const val AGENT_PAIRING_ROUTE = "agent/pairing"
private const val AGENT_SETTINGS_ROUTE = "agent/settings"

/** "Webhooks" dentro de Ajustes del agente (fase 8, ver docs/plan.md). */
private const val AGENT_WEBHOOKS_ROUTE = "agent/webhooks"

private const val ALERTS_ROUTE = "alerts"

private const val SECURITY_ROUTE = "security"

/** Pantalla Internet (fase 7, ver `docs/plan.md` § "Fase 7"). */
private const val INTERNET_ROUTE = "internet"

/** Cuánto esperar a "Red monitoreada" antes de abrir el equipo de una notificación (ver VigiaApp). */
private const val AgentModeWaitMs = 8_000L

/**
 * Raíz de la app: permisos, los dos ViewModel compartidos (escaneo de red y
 * Wi-Fi) y la navegación de 3 pestañas + el detalle de dispositivo, que vive
 * en el mismo NavHost pero sin barra inferior (es una pantalla de "entrar").
 *
 * [demoMode] es exclusivo de builds debug (ver `com.baxner.vigia.demo.DemoMode`):
 * con él, los ViewModel cargan datos de muestra en vez de pedir permisos y
 * escanear la red real.
 *
 * [pendingDeviceId] es el id de equipo que trae el `Intent` de una
 * notificación de alerta tocada (ver `notifications/AlertNotifications.kt` y
 * `MainActivity.onNewIntent`): no nulo → navega al detalle de ese equipo y
 * se limpia de inmediato (para que tocar otra vez la misma notificación, o
 * el mismo id, vuelva a disparar la navegación). Null constante (el default)
 * en las capturas/tests: ahí nunca hay una notificación de por medio.
 */
@Composable
fun VigiaApp(demoMode: Boolean = false, pendingDeviceId: MutableState<Long?>? = null) {
    val navController = rememberNavController()
    val scanViewModel: ScanViewModel = viewModel()
    val wifiViewModel: WifiViewModel = viewModel()
    val (permissionsState, requestPermissions) = rememberPermissionsState()
    val (_, requestBluetoothScanPermission) = rememberBluetoothScanPermissionState()

    // "Buscar señal" (fase 6): el objetivo viaja por fuera de la navegación
    // (no es un tipo serializable simple como los id de equipo) y se guarda con
    // Saver para sobrevivir recreaciones de Activity o muerte de proceso.
    var huntTarget by rememberSaveable(stateSaver = SignalTargetSaver) { mutableStateOf<SignalTarget?>(null) }
    val onOpenHunt: (SignalTarget) -> Unit = { target ->
        huntTarget = target
        navController.navigate(HUNT_ROUTE) { launchSingleTop = true }
    }

    LaunchedEffect(demoMode) {
        if (demoMode) {
            scanViewModel.enableDemoMode()
            wifiViewModel.enableDemoMode()
        } else {
            requestPermissions()
            // Como Fing: al abrir, la app ya está escaneando la red en la que estás.
            scanViewModel.startScanIfNeverScanned()
        }
    }

    LaunchedEffect(pendingDeviceId?.value) {
        val id = pendingDeviceId?.value ?: return@LaunchedEffect
        // El id es del historial del agente: solo vale en "Red monitoreada"
        // (en modo local el mismo número es otro equipo). Al abrir en frío ese
        // modo tarda un momento en activarse; fuera de casa no llega nunca, y
        // entonces se abre la lista de alertas.
        val monitored = id != NotificationTargetAlerts && withTimeoutOrNull(AgentModeWaitMs) {
            scanViewModel.agentPanelState.first {
                val linked = it as? AgentPanelState.Linked
                linked != null && (linked.monitoringCurrentNetwork || linked.viewingRemote)
            }
        } != null
        navController.navigate(if (monitored) deviceDetailRoute(id) else ALERTS_ROUTE) { launchSingleTop = true }
        pendingDeviceId.value = null
    }

    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = backStackEntry?.destination?.route
    val showBottomBar = topLevelDestinations.any { it.route == currentRoute }

    Scaffold(
        containerColor = VigiaColors.Background,
        bottomBar = {
            if (showBottomBar) {
                NavigationBar(containerColor = VigiaColors.Surface, contentColor = VigiaColors.TextSecondary) {
                    topLevelDestinations.forEach { destination ->
                        NavigationBarItem(
                            selected = currentRoute == destination.route,
                            onClick = {
                                if (currentRoute != destination.route) {
                                    navController.navigate(destination.route) {
                                        launchSingleTop = true
                                        restoreState = true
                                        popUpTo(TopLevelDestination.Panel.route) { saveState = true }
                                    }
                                }
                            },
                            icon = { Icon(destination.icon, contentDescription = null) },
                            label = { Text(stringResource(destination.labelRes)) },
                            colors = NavigationBarItemDefaults.colors(
                                selectedIconColor = VigiaColors.RadarCyan,
                                selectedTextColor = VigiaColors.RadarCyan,
                                indicatorColor = VigiaColors.RadarCyan.copy(alpha = 0.16f),
                                unselectedIconColor = VigiaColors.TextSecondary,
                                unselectedTextColor = VigiaColors.TextSecondary,
                            ),
                        )
                    }
                }
            }
        },
    ) { contentPadding ->
        NavHost(
            navController = navController,
            startDestination = TopLevelDestination.Panel.route,
            modifier = Modifier.padding(contentPadding),
        ) {
            composable(TopLevelDestination.Panel.route) {
                PanelScreen(
                    scanViewModel = scanViewModel,
                    permissionsState = permissionsState,
                    onRequestPermissions = requestPermissions,
                    onOpenDevices = {
                        navController.navigate(TopLevelDestination.Devices.route) { launchSingleTop = true }
                    },
                    demoMode = demoMode,
                    onOpenAgentPairing = { navController.navigate(AGENT_PAIRING_ROUTE) },
                    onOpenAgentSettings = { navController.navigate(AGENT_SETTINGS_ROUTE) },
                    onOpenAlerts = { navController.navigate(ALERTS_ROUTE) },
                    onOpenSecurity = { navController.navigate(SECURITY_ROUTE) },
                    onOpenInternet = { navController.navigate(INTERNET_ROUTE) },
                )
            }
            composable(ALERTS_ROUTE) {
                AlertsScreen(onBack = { navController.popBackStack() })
            }
            composable(SECURITY_ROUTE) {
                SecurityScreen(
                    scanViewModel = scanViewModel,
                    onBack = { navController.popBackStack() },
                    onOpenDevice = { id -> navController.navigate(deviceDetailRoute(id)) },
                )
            }
            composable(INTERNET_ROUTE) {
                InternetScreen(onBack = { navController.popBackStack() })
            }
            composable(AGENT_PAIRING_ROUTE) {
                AgentPairingScreen(
                    onDone = { navController.popBackStack() },
                    onCancel = { navController.popBackStack() },
                )
            }
            composable(AGENT_SETTINGS_ROUTE) {
                AgentSettingsScreen(
                    onDone = { navController.popBackStack() },
                    onOpenWebhooks = { navController.navigate(AGENT_WEBHOOKS_ROUTE) },
                )
            }
            composable(AGENT_WEBHOOKS_ROUTE) {
                WebhooksScreen(onBack = { navController.popBackStack() })
            }
            composable(TopLevelDestination.Devices.route) {
                DevicesScreen(
                    scanViewModel = scanViewModel,
                    onDeviceClick = { id -> navController.navigate(deviceDetailRoute(id)) },
                )
            }
            composable(
                route = DEVICE_DETAIL_ROUTE,
                arguments = listOf(navArgument("deviceId") { type = NavType.LongType }),
            ) { entry ->
                val id = entry.arguments?.getLong("deviceId") ?: -1L
                DeviceDetailScreen(
                    id = id,
                    scanViewModel = scanViewModel,
                    onBack = { navController.popBackStack() },
                    onEdit = { navController.navigate(deviceEditRoute(id)) },
                )
            }
            composable(
                route = DEVICE_EDIT_ROUTE,
                arguments = listOf(navArgument("deviceId") { type = NavType.LongType }),
            ) { entry ->
                val id = entry.arguments?.getLong("deviceId") ?: -1L
                DeviceEditScreen(
                    id = id,
                    scanViewModel = scanViewModel,
                    onDone = { navController.popBackStack() },
                )
            }
            composable(TopLevelDestination.Wifi.route) {
                WifiScreen(
                    wifiViewModel = wifiViewModel,
                    hasLocationPermission = permissionsState.hasLocation,
                    onRequestPermissions = requestPermissions,
                    demoMode = demoMode,
                    onHunt = onOpenHunt,
                )
            }
            composable(TopLevelDestination.Bluetooth.route) {
                BluetoothScreen(
                    scanViewModel = scanViewModel,
                    hasLocationPermission = permissionsState.hasLocation,
                    onRequestLocationPermission = requestPermissions,
                    onHunt = onOpenHunt,
                )
            }
            composable(HUNT_ROUTE) {
                val target = huntTarget
                if (target == null) {
                    // No debería pasar: solo se navega aquí con un objetivo ya puesto (ver onOpenHunt).
                    LaunchedEffect(Unit) { navController.popBackStack() }
                } else {
                    HuntScreen(
                        target = target,
                        onBack = { navController.popBackStack() },
                        onRequestLocationPermission = requestPermissions,
                        onRequestBluetoothPermission = requestBluetoothScanPermission,
                    )
                }
            }
        }
    }
}

internal val SignalTargetSaver: Saver<SignalTarget?, Any> = Saver(
    save = { target ->
        when (target) {
            null -> ""
            is SignalTarget.WifiAp -> arrayListOf(
                "wifi",
                target.bssid,
                target.ssid,
                target.frequencyMhz,
                target.rttCapable,
                target.connected,
                target.label,
            )
            is SignalTarget.Ble -> arrayListOf(
                "ble",
                target.address,
                target.name,
                target.brand,
                target.txPowerDbm,
                target.label,
            )
        }
    },
    restore = { raw ->
        val list = raw as? List<*> ?: return@Saver null
        when (list.firstOrNull()) {
            "wifi" -> if (list.size >= 7) {
                SignalTarget.WifiAp(
                    bssid = list[1] as? String ?: return@Saver null,
                    ssid = list[2] as? String ?: return@Saver null,
                    frequencyMhz = list[3] as? Int ?: return@Saver null,
                    rttCapable = list[4] as? Boolean ?: return@Saver null,
                    connected = list[5] as? Boolean ?: return@Saver null,
                    label = list[6] as? String ?: return@Saver null,
                )
            } else {
                null
            }
            "ble" -> if (list.size >= 6) {
                SignalTarget.Ble(
                    address = list[1] as? String ?: return@Saver null,
                    name = list[2] as? String,
                    brand = list[3] as? String,
                    txPowerDbm = list[4] as? Int,
                    label = list[5] as? String ?: return@Saver null,
                )
            } else {
                null
            }
            else -> null
        }
    },
)
