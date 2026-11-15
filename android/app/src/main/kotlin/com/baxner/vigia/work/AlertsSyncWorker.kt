package com.baxner.vigia.work

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.baxner.vigia.data.agent.AlertsModule
import java.util.concurrent.TimeUnit

/**
 * Pide `/alerts?after=<última vista>` cada ~15 min con la app cerrada (ver
 * el contrato § "Alertas v1.1" → "Notificaciones en la app", y el encargo,
 * punto 4): sin servicio en primer plano, solo esta tarea periódica. Todo
 * lo demás (qué direcciones probar, qué notificar, sin duplicados) vive en
 * [com.baxner.vigia.data.agent.AlertsRepository]; esta clase solo lo dispara.
 *
 * Siempre devuelve [Result.success]: es un sondeo best-effort. Con
 * [Result.retry] WorkManager reintentaría con su propio backoff, compitiendo
 * con el período fijo de 15 min — mejor un solo reloj: si ninguna dirección
 * contestó, el próximo período ya lo vuelve a intentar solo.
 */
class AlertsSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        // Mismo camino (caché, cursor y candado) que la app abierta: si el
        // stream en vivo y este sondeo traen la misma alerta, se avisa una vez.
        AlertsModule.repository(applicationContext).refreshFromNetwork()
        return Result.success()
    }

    companion object {
        private const val UniqueWorkName = "alerts-sync"

        /**
         * Programa (o deja como está, ver [ExistingPeriodicWorkPolicy.KEEP])
         * el sondeo cada 15 min con red conectada (mínimo que admite
         * WorkManager, igual al del contrato). Llamarlo al vincular
         * ([com.baxner.vigia.data.agent.AgentModeController.pair]) y, por si
         * acaso, cuando ya hay un vínculo activo (ver `AlertsRepository.init`):
         * es idempotente, no duplica la tarea.
         */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<AlertsSyncWorker>(15, TimeUnit.MINUTES)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .build()
            WorkManager.getInstance(context.applicationContext)
                .enqueueUniquePeriodicWork(UniqueWorkName, ExistingPeriodicWorkPolicy.KEEP, request)
        }

        /** Al desvincular ([AgentModeController.forget]): no seguir sondeando un agente que esta app ya no conoce. */
        fun cancel(context: Context) {
            WorkManager.getInstance(context.applicationContext).cancelUniqueWork(UniqueWorkName)
        }
    }
}
