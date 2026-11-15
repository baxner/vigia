import SwiftUI

public struct AgentSettingsView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.dismiss) private var dismiss

    @State private var confirmUnlink = false
    @State private var syncStatusMessage: String?
    @State private var isSyncingBg = false

    public init() {}

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                if let link = store.agentLink {
                    connectionCard(link)
                    backgroundAndWebhooksCard
                    alertTypesCard
                    unlinkButton
                } else {
                    VigiaEmptyState(
                        symbol: "server.rack",
                        title: "Sin agente vinculado",
                        message: "Empareja un servidor Vigía desde el Panel para configurar alertas, notificaciones y webhooks."
                    )
                }
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle("Agente 24/7")
        .navigationBarTitleDisplayMode(.inline)
        .confirmationDialog(
            "¿Desvincular este iPhone del agente?",
            isPresented: $confirmUnlink,
            titleVisibility: .visible
        ) {
            Button("Desvincular", role: .destructive) {
                store.unlinkAgent()
                dismiss()
            }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("Se borrará el token y el pin TLS del Keychain de este iPhone.")
        }
    }

    private func connectionCard(_ link: AgentLink) -> some View {
        VigiaGlassCard(highlighted: store.agentOnline, glowColor: store.agentOnline ? VigiaColor.ok : VigiaColor.amber) {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Text(store.agentInfo?.name ?? link.agentName)
                        .font(.headline)
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    VigiaStatusChip(
                        store.agentOnline ? "En línea" : "Sin respuesta",
                        color: store.agentOnline ? VigiaColor.ok : VigiaColor.amber
                    )
                }
                row("Host principal", value: "\(link.host):\(link.port)", mono: true)
                if let version = store.agentInfo?.version, !version.isEmpty {
                    row("Versión del agente", value: version, mono: true)
                }
                if !link.normalizedCertPin.isEmpty {
                    row("Pin TLS SHA-256", value: "\(link.normalizedCertPin.prefix(16))…", mono: true)
                }
                let saved = AgentPreferencesStore.savedAddresses
                if !saved.isEmpty {
                    row("Direcciones LAN/Tailscale", value: saved.joined(separator: ", "), mono: true)
                }
            }
        }
    }

    private var backgroundAndWebhooksCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text("Notificaciones en iPhone")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                NavigationLink {
                    WebhooksView()
                } label: {
                    HStack {
                        Label("Webhooks y push instantáneo (ntfy / Telegram)", systemImage: "paperplane.fill")
                            .font(.subheadline.weight(.medium))
                            .foregroundStyle(VigiaColor.radarCyan)
                        Spacer()
                        Text("\(store.webhooks.count)")
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(VigiaColor.textSecondary)
                        Image(systemName: "chevron.right")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: 12).fill(VigiaColor.surfaceElevated))
                }
                .buttonStyle(.plain)

                Button {
                    isSyncingBg = true
                    Task {
                        let ok = await BackgroundAlertManager.shared.syncFromAgentNow()
                        syncStatusMessage = ok
                            ? VigiaL10n.tr("Widget and alerts synced successfully.")
                            : VigiaL10n.tr("Could not reach the agent to sync.")
                        isSyncingBg = false
                    }
                } label: {
                    HStack {
                        if isSyncingBg { ProgressView().controlSize(.small) }
                        Text("Probar actualización de fondo y Widget")
                            .font(.caption.weight(.semibold))
                    }
                }
                .buttonStyle(.bordered)
                .tint(VigiaColor.electricBlue)
                .vigiaMinTouchTarget()

                if let syncStatusMessage {
                    Text(syncStatusMessage)
                        .font(.caption2)
                        .foregroundStyle(VigiaColor.textSecondary)
                }
            }
        }
    }

    private var alertTypesCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text("Tipos de alerta activos en el agente")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                ForEach(AlertType.configurableCases) { alertType in
                    Toggle(
                        alertType.label,
                        isOn: Binding(
                            get: { store.alertSettings.isEnabled(alertType) },
                            set: { enabled in Task { await store.updateAlertSetting(alertType, enabled: enabled) } }
                        )
                    )
                    .font(.caption)
                    .tint(VigiaColor.radarCyan)
                }
            }
        }
    }

    private var unlinkButton: some View {
        Button(role: .destructive) {
            confirmUnlink = true
        } label: {
            Label("Desvincular este iPhone", systemImage: "link.badge.minus")
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity)
                .padding(.vertical, 12)
                .background(
                    RoundedRectangle(cornerRadius: 14)
                        .fill(VigiaColor.risk.opacity(0.14))
                        .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(VigiaColor.risk.opacity(0.35), lineWidth: 1))
                )
                .foregroundStyle(VigiaColor.risk)
        }
    }

    private func row(_ title: String, value: String, mono: Bool = false) -> some View {
        HStack(alignment: .top) {
            Text(title)
                .font(.caption)
                .foregroundStyle(VigiaColor.textSecondary)
            Spacer()
            Text(value)
                .font(mono ? .system(.caption, design: .monospaced) : .caption)
                .foregroundStyle(VigiaColor.textPrimary)
                .multilineTextAlignment(.trailing)
        }
    }
}
