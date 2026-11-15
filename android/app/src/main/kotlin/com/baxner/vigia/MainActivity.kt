package com.baxner.vigia

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import com.baxner.vigia.data.agent.AgentModule
import com.baxner.vigia.demo.DemoMode
import com.baxner.vigia.notifications.NotificationExtraDeviceId
import com.baxner.vigia.ui.navigation.VigiaApp
import com.baxner.vigia.ui.theme.VigiaTheme

/**
 * Punto de entrada único de Vigía (single-activity, Navigation Compose).
 *
 * Solo en builds debug: `adb shell am start -n com.baxner.vigia/.MainActivity
 * --ez demo true` abre la app en modo demo (ver [DemoMode]), con datos de
 * muestra en vez de escanear la red real. En release ese extra no hace nada.
 */
class MainActivity : ComponentActivity() {
    // Fuera de setContent (ver onNewIntent): Compose no vuelve a llamar
    // setContent cuando llega un Intent nuevo con la Activity ya en memoria,
    // así que el id pendiente vive en este MutableState compartido en vez de
    // leerse de `intent` directo en cada composición.
    private val pendingDeviceId = mutableStateOf<Long?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // La app es siempre oscura: barras del sistema con iconos claros aunque
        // el teléfono esté en modo claro (si no, la hora sale oscura sobre oscuro).
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
        )
        val demoMode = DemoMode.isRequestedBy(intent)
        pendingDeviceId.value = intent.alertDeviceIdOrNull()
        setContent {
            VigiaTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    VigiaApp(demoMode = demoMode, pendingDeviceId = pendingDeviceId)
                }
            }
        }
    }

    // La vista remota por Tailscale solo con la app a la vista (ver
    // AgentModeController.onAppForegroundChanged).
    override fun onStart() {
        super.onStart()
        AgentModule.controller(this).onAppForegroundChanged(true)
    }

    override fun onStop() {
        super.onStop()
        AgentModule.controller(this).onAppForegroundChanged(false)
    }

    /** Tocar la notificación de una alerta con la app ya abierta (ver `notifications/AlertNotifications.kt`): no hay `onCreate` nuevo, solo este callback. */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intent.alertDeviceIdOrNull()?.let { pendingDeviceId.value = it }
    }
}

/** El id de equipo de una notificación de alerta, [com.baxner.vigia.notifications.NotificationTargetAlerts] para la lista, o null si no se abrió desde una. */
private fun Intent.alertDeviceIdOrNull(): Long? =
    getLongExtra(NotificationExtraDeviceId, -1L).takeIf { it >= 0 }
