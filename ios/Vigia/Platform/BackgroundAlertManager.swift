import Foundation
import BackgroundTasks
import UserNotifications
import WidgetKit

/// Plantillas de un toque para registrar webhooks push en el Agente Vigía y recibir
/// alertas instantáneas aunque iOS haya suspendido la app en segundo plano.
public enum PushWebhookPreset {
    public static func generateNtfyTopic() -> String {
        let suffix = UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(12).lowercased()
        return "vigia-alertas-\(suffix)"
    }

    public static func ntfyWebhookUrl(topic: String, serverBaseUrl: String = "https://ntfy.sh") -> String {
        let cleanBase = serverBaseUrl.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let cleanTopic = topic.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return "\(cleanBase)/\(cleanTopic)"
    }

    public static func telegramWebhookUrl(botToken: String, chatId: String) -> String {
        let token = botToken.trimmingCharacters(in: .whitespacesAndNewlines)
        let chat = chatId.trimmingCharacters(in: .whitespacesAndNewlines)
        return "https://api.telegram.org/bot\(token)/sendMessage?chat_id=\(chat)"
    }

    public static func homeAssistantWebhookUrl(baseUrl: String, webhookId: String) -> String {
        let cleanBase = baseUrl.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let cleanId = webhookId.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return "\(cleanBase)/api/webhook/\(cleanId)"
    }

    public static let defaultUrgentAlertTypes: [String] = [
        AlertType.newDevice.rawValue,
        AlertType.importantOffline.rawValue,
        AlertType.arpConflict.rawValue,
        AlertType.routerChanged.rawValue,
        AlertType.portMappingAdded.rawValue,
        AlertType.riskyService.rawValue,
        AlertType.internetDown.rawValue
    ]
}

/// Coordina `BGAppRefreshTask` (`com.baxner.vigia.alerts.refresh`), notificaciones locales,
/// actualización del WidgetKit en App Group y asistentes de webhooks push.
#if os(iOS)
public final class BackgroundAlertManager: @unchecked Sendable {
    public static let shared = BackgroundAlertManager()

    private let lock = NSLock()
    private var isRegistered = false

    private init() {}

    /// Registra la tarea de actualización en segundo plano al arrancar la app.
    public func registerBackgroundTasks() {
        lock.lock()
        let already = isRegistered
        isRegistered = true
        lock.unlock()
        guard !already else { return }

        BGTaskScheduler.shared.register(
            forTaskWithIdentifier: AppGroupConstants.refreshTaskID,
            using: nil
        ) { [weak self] task in
            guard let refreshTask = task as? BGAppRefreshTask else {
                task.setTaskCompleted(success: false)
                return
            }
            self?.handleAppRefresh(refreshTask)
        }
    }

    /// Programa la próxima ejecución periódica (~15 minutos según política de iOS).
    public func scheduleNextRefresh() {
        let request = BGAppRefreshTaskRequest(identifier: AppGroupConstants.refreshTaskID)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        do {
            try BGTaskScheduler.shared.submit(request)
        } catch {
            // En simulador o con actualización en segundo plano desactivada el scheduler puede rechazar la petición.
        }
    }

    @discardableResult
    public func requestNotificationAuthorization() async -> Bool {
        let center = UNUserNotificationCenter.current()
        do {
            return try await center.requestAuthorization(options: [.alert, .sound, .badge])
        } catch {
            return false
        }
    }

    public func notificationAuthorizationStatus() async -> UNAuthorizationStatus {
        await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    /// Actualiza el resumen compartido con `VigiaWidget` en `group.com.baxner.vigia`.
    public func updateWidgetSummary(
        linked: Bool,
        onlineDevices: Int,
        unreadAlerts: Int,
        internetState: String,
        internetLatencyMs: Int?
    ) {
        let summary = WidgetSummaryDto(
            linked: linked,
            onlineDevices: onlineDevices,
            unreadAlerts: unreadAlerts,
            internetState: internetState,
            internetLatencyMs: internetLatencyMs,
            updatedAtEpochMs: Int64(Date().timeIntervalSince1970 * 1000.0)
        )
        summary.save()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// Dispara una notificación local de iOS para una alerta nueva del agente.
    public func deliverLocalNotification(for alert: AlertDto) {
        let content = UNMutableNotificationContent()
        content.title = alert.title
        content.body = alert.body
        content.sound = alert.severityLevel == .critical ? .defaultCritical : .default
        content.userInfo = [
            "alertId": alert.id,
            "type": alert.type,
            "deviceId": alert.deviceId as Any
        ]

        let request = UNNotificationRequest(
            identifier: "vigia-alert-\(alert.id)",
            content: content,
            trigger: nil
        )
        UNUserNotificationCenter.current().add(request)
    }

    private func handleAppRefresh(_ task: BGAppRefreshTask) {
        scheduleNextRefresh()

        let syncWork = Task { [weak self] in
            guard let self else {
                task.setTaskCompleted(success: false)
                return
            }
            let ok = await self.syncFromAgentNow()
            task.setTaskCompleted(success: ok)
        }

        task.expirationHandler = {
            syncWork.cancel()
        }
    }

    /// Sincroniza alertas nuevas, conteo de equipos en línea y estado de internet desde el agente.
    @discardableResult
    public func syncFromAgentNow() async -> Bool {
        guard let link = KeychainLinkStore.shared.load() else {
            updateWidgetSummary(
                linked: false,
                onlineDevices: 0,
                unreadAlerts: 0,
                internetState: "unknown",
                internetLatencyMs: nil
            )
            return true
        }

        let client = await AgentApiClient.connectReachable(link: link)
        do {
            async let infoTask = client.info()
            async let alertsTask = client.alerts(after: 0, limit: 50)
            async let internetTask = client.internetStatus()
            async let networksTask = client.networks()

            let info = try await infoTask
            if !info.addresses.isEmpty {
                AgentPreferencesStore.savedAddresses = info.addresses
            }
            let alerts = try await alertsTask
            let internet = try? await internetTask
            let networks = (try? await networksTask) ?? []

            var onlineCount = 0
            if let preferredKey = AgentPreferencesStore.selectedNetworkKeyValue
                ?? networks.first(where: { $0.role == "inner" })?.key
                ?? networks.first?.key {
                if let devs = try? await client.devices(networkKey: preferredKey) {
                    onlineCount = devs.filter(\.online).count
                }
            }

            let unread = alerts.filter { !$0.read }
            let lastSeenId = AgentPreferencesStore.lastNotifiedAlertId
            let unseenNew = unread.filter { $0.id > lastSeenId }.sorted { $0.id < $1.id }

            for alert in unseenNew.prefix(5) {
                deliverLocalNotification(for: alert)
            }
            if let maxId = alerts.map(\.id).max(), maxId > lastSeenId {
                AgentPreferencesStore.lastNotifiedAlertId = maxId
            }

            let latencyMs = internet?.now.internetMs.map { Int($0.rounded()) }
            updateWidgetSummary(
                linked: true,
                onlineDevices: onlineCount,
                unreadAlerts: unread.count,
                internetState: internet?.state ?? "unknown",
                internetLatencyMs: latencyMs
            )
            return true
        } catch {
            return false
        }
    }
}
#endif
