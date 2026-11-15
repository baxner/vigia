package com.baxner.vigia.data.agent

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1", y el límite
 * documentado en [LiveWatchService]): tras reiniciar el teléfono, si el
 * interruptor seguía prendido y hay un agente vinculado, vuelve a arrancar
 * [LiveWatchService] — que reevalúa la red él solo y se detiene de nuevo si
 * todavía no hay Wi-Fi o no es la de casa.
 *
 * `BOOT_COMPLETED` es una de las excepciones documentadas a la restricción
 * de Android 12+ de arrancar un servicio en primer plano desde segundo
 * plano (un receptor de manifiesto registrado para ese broadcast puede
 * hacerlo); por eso esta clase existe aparte de [LiveWatchService] en vez de
 * intentarlo desde algún otro lado.
 */
class BootStartReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
        val appContext = context.applicationContext
        val pendingResult = goAsync()
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val store: AlertsStore = SharedPrefsAlertsStore(appContext)
                if (!store.liveWatchEnabled()) return@launch
                if (AgentModule.controller(appContext).linkedHostAndPort() == null) return@launch
                LiveWatchService.start(appContext)
            } finally {
                pendingResult.finish()
            }
        }
    }
}
