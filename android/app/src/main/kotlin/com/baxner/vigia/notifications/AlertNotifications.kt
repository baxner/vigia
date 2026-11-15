package com.baxner.vigia.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.baxner.vigia.MainActivity
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.AlertType
import com.baxner.vigia.ui.permissions.hasNotificationPermission

/**
 * Los canales de Android del contrato: los 3 de v1.1 (ver
 * docs/agente-api.md § "Alertas v1.1" → "Notificaciones en la app") más
 * "Seguridad" de v1.2 (ver § "Seguridad" → "Alertas nuevas": alta, para
 * `ROUTER_CHANGED`/`ARP_CONFLICT`/`PORT_MAPPING_ADDED`/`RISKY_SERVICE`).
 */
object AlertChannels {
    const val NEW_DEVICES = "alerts_new_devices"
    const val IMPORTANT_DEVICES = "alerts_important_devices"
    const val AGENT_STATUS = "alerts_agent_status"
    const val SECURITY = "alerts_security"

    /** "Internet" (v1.4, ver docs/agente-api.md § "Internet" → "Alertas"): `INTERNET_DOWN`/`INTERNET_RESTORED`. */
    const val INTERNET = "alerts_internet"

    /**
     * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): la notificación
     * fija y silenciosa de [com.baxner.vigia.data.agent.LiveWatchService],
     * de baja importancia a propósito — es solo un indicador, no una alerta.
     */
    const val LIVE_WATCH = "live_watch"
}

/**
 * Extra del [Intent] que abre [MainActivity] al tocar la notificación (ver
 * [notifyAlert] y `MainActivity.kt`): el id del equipo en el historial del
 * agente, o [NotificationTargetAlerts] para abrir la lista de alertas.
 */
const val NotificationExtraDeviceId = "com.baxner.vigia.extra.DEVICE_ID"

/** Valor de [NotificationExtraDeviceId] de las alertas sin equipo (`AGENT_*`): abre la pantalla Alertas. */
const val NotificationTargetAlerts = 0L

/**
 * El canal (ver [AlertChannels]) para el [AlertType] de una alerta — por
 * TIPO, como pide el contrato, no por [AlertDto.severity] (esa decide el
 * icono/color en la lista, ver `ui/alerts/AlertsScreen.kt`). Un tipo que
 * esta versión no reconoce (`null`, ver [AlertType.fromRaw]) cae en el
 * canal de menor prioridad: se avisa igual, pero sin molestar de más por
 * algo que la app no sabe clasificar todavía.
 */
fun AlertType?.channelId(): String = when (this) {
    AlertType.NEW_DEVICE -> AlertChannels.NEW_DEVICES
    AlertType.IMPORTANT_OFFLINE, AlertType.IMPORTANT_ONLINE -> AlertChannels.IMPORTANT_DEVICES
    AlertType.AGENT_BLIND, AlertType.AGENT_RECOVERED, null -> AlertChannels.AGENT_STATUS
    AlertType.ROUTER_CHANGED, AlertType.ARP_CONFLICT,
    AlertType.PORT_MAPPING_ADDED, AlertType.RISKY_SERVICE,
    -> AlertChannels.SECURITY
    AlertType.INTERNET_DOWN, AlertType.INTERNET_RESTORED -> AlertChannels.INTERNET
}

private fun AlertType?.notificationPriority(): Int = when (this) {
    AlertType.NEW_DEVICE -> NotificationCompat.PRIORITY_HIGH
    AlertType.IMPORTANT_OFFLINE, AlertType.IMPORTANT_ONLINE -> NotificationCompat.PRIORITY_DEFAULT
    AlertType.AGENT_BLIND, AlertType.AGENT_RECOVERED, null -> NotificationCompat.PRIORITY_LOW
    AlertType.ROUTER_CHANGED, AlertType.ARP_CONFLICT,
    AlertType.PORT_MAPPING_ADDED, AlertType.RISKY_SERVICE,
    AlertType.INTERNET_DOWN,
    -> NotificationCompat.PRIORITY_HIGH
    // INTERNET_RESTORED (ver el contrato: severidad "low") comparte el
    // canal "Internet" con INTERNET_DOWN (misma importancia de canal, que
    // el usuario controla desde el sistema) pero con menos prioridad acá:
    // avisa, sin sonar como la caída misma.
    AlertType.INTERNET_RESTORED -> NotificationCompat.PRIORITY_LOW
}

/**
 * Crea los 3 canales si no existen (idempotente: `createNotificationChannel`
 * no duplica nada si ya existe uno con ese id — solo Android <26 no tiene
 * canales, pero minSdk 26 ya los exige siempre). Llamarlo temprano (ver
 * `AlertsRepository.init`) para que existan antes de la primera notificación.
 */
fun ensureAlertChannels(context: Context) {
    val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.NEW_DEVICES, context.getString(R.string.alert_channel_new_devices), NotificationManager.IMPORTANCE_HIGH).apply {
            description = context.getString(R.string.alert_channel_new_devices_desc)
        },
    )
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.IMPORTANT_DEVICES, context.getString(R.string.alert_channel_important_devices), NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = context.getString(R.string.alert_channel_important_devices_desc)
        },
    )
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.AGENT_STATUS, context.getString(R.string.alert_channel_agent_status), NotificationManager.IMPORTANCE_LOW).apply {
            description = context.getString(R.string.alert_channel_agent_status_desc)
        },
    )
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.SECURITY, context.getString(R.string.alert_channel_security), NotificationManager.IMPORTANCE_HIGH).apply {
            description = context.getString(R.string.alert_channel_security_desc)
        },
    )
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.INTERNET, context.getString(R.string.alert_channel_internet), NotificationManager.IMPORTANCE_HIGH).apply {
            description = context.getString(R.string.alert_channel_internet_desc)
        },
    )
    ensureLiveWatchChannel(context)
}

/**
 * El canal de [AlertChannels.LIVE_WATCH] (ver "Avisos al instante",
 * `docs/plan.md` § "Fase 5.1"): aparte de [ensureAlertChannels] porque
 * [com.baxner.vigia.data.agent.LiveWatchService] lo necesita sin tener que
 * crear los otros 4 primero (es el único canal que usa, y arranca antes de
 * que [com.baxner.vigia.data.agent.AlertsModule] se instancie en algunos
 * caminos — p. ej. tras `BOOT_COMPLETED`).
 */
fun ensureLiveWatchChannel(context: Context) {
    val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
    manager.createNotificationChannel(
        NotificationChannel(AlertChannels.LIVE_WATCH, context.getString(R.string.alert_channel_live_watch), NotificationManager.IMPORTANCE_LOW).apply {
            description = context.getString(R.string.alert_channel_live_watch_desc)
            setShowBadge(false)
        },
    )
}

/**
 * Notificación de una [AlertDto] (ver el contrato y el encargo, puntos 2 y
 * 3). Tocarla abre [MainActivity] en el detalle del equipo (deep link a
 * `devices/{id}`, ver `ui/navigation/VigiaNavHost.kt`) o la lista de
 * alertas si [AlertDto.deviceId] es null (las de tipo `AGENT_*`).
 *
 * No hace nada sin el permiso de notificaciones (Android 13+, ver
 * [hasNotificationPermission]): quien llama no necesita chequearlo antes —
 * la alerta de todas formas ya quedó en el caché/lista de la pantalla.
 */
fun notifyAlert(context: Context, alert: AlertDto) {
    if (!hasNotificationPermission(context)) return
    val type = AlertType.fromRaw(alert.type)
    val intent = Intent(context, MainActivity::class.java).apply {
        flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        putExtra(NotificationExtraDeviceId, alert.deviceId ?: NotificationTargetAlerts)
    }
    // alert.id (Long) -> Int: el agente lo da como autoincremental; haría
    // falta que una sola instalación acumulara más de dos mil millones de
    // alertas para que esto colisione.
    val requestCode = alert.id.toInt()
    val pendingIntent = PendingIntent.getActivity(
        context,
        requestCode,
        intent,
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )
    val notification = NotificationCompat.Builder(context, type.channelId())
        .setSmallIcon(R.drawable.ic_notification)
        .setContentTitle(alert.title)
        .setContentText(alert.body)
        .setStyle(NotificationCompat.BigTextStyle().bigText(alert.body))
        .setAutoCancel(true)
        // Si la misma alerta se vuelve a publicar (mismo id), que no suene otra vez.
        .setOnlyAlertOnce(true)
        .setContentIntent(pendingIntent)
        .setPriority(type.notificationPriority())
        .build()
    try {
        NotificationManagerCompat.from(context).notify(requestCode, notification)
    } catch (_: SecurityException) {
        // El permiso pudo haberse revocado entre hasNotificationPermission() y notify().
    }
}
