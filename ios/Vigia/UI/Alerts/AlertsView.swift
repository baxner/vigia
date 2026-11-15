import SwiftUI

public struct AlertsView: View {
    @EnvironmentObject private var store: AppStore
    @State private var unreadOnly = false

    public init() {}

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                if !store.isLinked {
                    VigiaEmptyState(
                        symbol: "bell.badge",
                        title: "Alertas 24/7 no vinculadas",
                        message: "Vincula tu Agente Vigía para recibir avisos cuando llegue un equipo desconocido, caiga internet o cambie el router."
                    )
                } else {
                    filterBar
                    alertsContent
                }
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle("Alertas")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if store.isLinked {
                ToolbarItemGroup(placement: .topBarTrailing) {
                    if store.unreadAlertCount > 0 {
                        Button("Leer todas") {
                            Task { await store.markAllAlertsRead() }
                        }
                        .font(.caption.weight(.semibold))
                        .vigiaMinTouchTarget()
                    }
                    NavigationLink {
                        WebhooksView()
                    } label: {
                        Image(systemName: "paperplane")
                            .vigiaMinTouchTarget()
                    }
                    .accessibilityLabel(VigiaL10n.tr("Push webhooks"))
                    NavigationLink {
                        AgentSettingsView()
                    } label: {
                        Image(systemName: "gearshape")
                            .vigiaMinTouchTarget()
                    }
                    .accessibilityLabel(VigiaL10n.tr("Agent settings"))
                }
            }
        }
        .refreshable {
            await store.refreshAlerts()
        }
        .task {
            if store.isLinked {
                await store.refreshAlerts()
            }
        }
    }

    private var filterBar: some View {
        HStack {
            Picker("Filtro", selection: $unreadOnly) {
                Text(VigiaL10n.fmt("All (%ld)", store.alerts.count)).tag(false)
                Text(VigiaL10n.fmt("Unread (%ld)", store.unreadAlertCount)).tag(true)
            }
            .pickerStyle(.segmented)
        }
    }

    @ViewBuilder
    private var alertsContent: some View {
        let items = unreadOnly ? store.alerts.filter { !$0.read } : store.alerts
        if items.isEmpty {
            VigiaEmptyState(
                symbol: "checkmark.circle",
                title: unreadOnly ? "Sin alertas pendientes" : "Sin alertas registradas",
                message: unreadOnly ? "Ya revisaste todas las notificaciones del agente." : "Todo tranquilo en tu red."
            )
        } else {
            LazyVStack(spacing: 10) {
                ForEach(items) { alert in
                    let matchedDevice = alert.deviceId.flatMap { did in
                        store.devices.first(where: { $0.id == did })
                    }
                    if let matchedDevice {
                        NavigationLink {
                            DeviceDetailView(initialDevice: matchedDevice)
                        } label: {
                            alertCard(alert, hasTarget: true)
                        }
                        .buttonStyle(.plain)
                    } else {
                        alertCard(alert, hasTarget: false)
                    }
                }
            }
        }
    }

    private func alertCard(_ alert: AlertDto, hasTarget: Bool) -> some View {
        let sevColor: Color = {
            switch alert.severityLevel {
            case .critical: return VigiaColor.risk
            case .warning: return VigiaColor.amber
            case .info: return VigiaColor.radarCyan
            }
        }()
        return VigiaGlassCard(highlighted: !alert.read, glowColor: sevColor, padding: 14) {
            VStack(alignment: .leading, spacing: 6) {
                HStack {
                    VigiaStatusChip(alertTypeTitle(alert.type), color: sevColor)
                    Spacer()
                    if !alert.read {
                        VigiaStatusChip(VigiaL10n.tr("New"), color: VigiaColor.risk, icon: "circle.fill")
                    }
                    Text(VigiaDate.relativeDescription(from: alert.at))
                        .font(.caption2)
                        .foregroundStyle(VigiaColor.textTertiary)
                }

                Text(alert.title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                if !alert.body.isEmpty {
                    Text(alert.body)
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                }

                if hasTarget {
                    HStack {
                        Spacer()
                        Label("Ver equipo", systemImage: "chevron.right")
                            .font(.caption2.weight(.semibold))
                            .foregroundStyle(VigiaColor.electricBlue)
                    }
                }
            }
        }
    }

    private func alertTypeTitle(_ raw: String) -> String {
        AlertType(rawValue: raw)?.label ?? raw
    }
}
