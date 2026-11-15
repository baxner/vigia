package com.baxner.vigia.data.repository

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import androidx.core.app.NotificationCompat
import androidx.work.CoroutineWorker
import androidx.work.Data
import androidx.work.ForegroundInfo
import androidx.work.OneTimeWorkRequest
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.OutOfQuotaPolicy
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import com.baxner.vigia.R
import com.baxner.vigia.data.history.NetworkKey
import java.util.concurrent.TimeUnit

/**
 * Escaneo completo de puertos de un equipo (~1,150 puertos, 20-40 s) como
 * trabajo urgente de WorkManager, para que termine aunque se salga del
 * detalle o de la app: en segundo plano Android congela el proceso en
 * segundos, y un escaneo congelado a medias da puertos "cerrados" falsos.
 * El resultado queda en la base de WorkManager (ver
 * [PortScanWork.KEEP_RESULTS_DAYS]) y el detalle lo muestra al volver.
 */
class PortScanWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        val ip = inputData.getString(PortScanWork.KEY_IP) ?: return Result.failure()
        // Si Android mató el proceso a medias, WorkManager lo reintenta más
        // tarde, quizá ya en otra red: mejor avisar que escanear a ciegas.
        if (runAttemptCount > 0) {
            return Result.failure(PortScanWork.failureData(ip, applicationContext.getString(R.string.port_scan_error_interrupted)))
        }
        val spec = inputData.getString(PortScanWork.KEY_SPEC).orEmpty()
        return ScanRepository.scanPortsOnce(ip, spec).fold(
            onSuccess = { ports -> Result.success(PortScanWork.successData(ip, ports, System.currentTimeMillis())) },
            onFailure = { error ->
                Result.failure(
                    PortScanWork.failureData(ip, error.message ?: applicationContext.getString(R.string.error_unknown)),
                )
            },
        )
    }

    // Solo en Android < 12: ahí el trabajo urgente corre como servicio en
    // primer plano y Android exige su aviso. En 12+ no se llama.
    override suspend fun getForegroundInfo(): ForegroundInfo {
        val manager = applicationContext.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, applicationContext.getString(R.string.port_scan_channel_name), NotificationManager.IMPORTANCE_LOW),
        )
        val ip = inputData.getString(PortScanWork.KEY_IP).orEmpty()
        val notification = NotificationCompat.Builder(applicationContext, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle(applicationContext.getString(R.string.port_scan_notification_title))
            .setContentText(ip)
            .setOngoing(true)
            .build()
        return ForegroundInfo(NOTIFICATION_ID, notification)
    }

    private companion object {
        const val CHANNEL_ID = "port_scans"
        const val NOTIFICATION_ID = 7341
    }
}

/** Cómo se pide y se guarda un [PortScanWorker]: nombres, claves de datos y la petición. */
object PortScanWork {
    const val KEY_IP = "ip"
    const val KEY_SPEC = "spec"
    const val KEY_PORTS = "ports"
    const val KEY_FINISHED_AT = "finishedAt"
    const val KEY_ERROR = "error"

    /** Lo que se guarda el último resultado de cada equipo (WorkManager borra después). */
    const val KEEP_RESULTS_DAYS = 7L

    /**
     * Un escaneo por equipo. El id solo es único dentro de su historial (el
     * local o el del agente, con claves de red de forma distinta), por eso
     * va junto con la clave de red.
     */
    fun uniqueName(network: NetworkKey, deviceId: Long): String = "ports|${network.value}|$deviceId"

    fun request(ip: String, spec: String): OneTimeWorkRequest =
        OneTimeWorkRequestBuilder<PortScanWorker>()
            .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            .setInputData(workDataOf(KEY_IP to ip, KEY_SPEC to spec))
            .keepResultsForAtLeast(KEEP_RESULTS_DAYS, TimeUnit.DAYS)
            .build()

    // Un IntArray de hasta ~1,150 puertos ocupa ~4.6 KB: cabe en el límite
    // de 10 KB de Data aunque el equipo tuviera todo abierto.
    fun successData(ip: String, ports: List<Int>, finishedAtMillis: Long): Data =
        workDataOf(KEY_IP to ip, KEY_PORTS to ports.toIntArray(), KEY_FINISHED_AT to finishedAtMillis)

    fun failureData(ip: String, message: String): Data = workDataOf(KEY_IP to ip, KEY_ERROR to message)
}
