import SwiftUI
import UIKit

public struct WebhooksView: View {
    @EnvironmentObject private var store: AppStore

    @State private var presetMode: PresetMode = .ntfy
    @State private var ntfyTopic: String = PushWebhookPreset.generateNtfyTopic()
    @State private var ntfyServer: String = "https://ntfy.sh"
    @State private var telegramToken: String = ""
    @State private var telegramChatId: String = ""
    @State private var haBaseUrl: String = ""
    @State private var haWebhookId: String = ""
    @State private var customUrl: String = ""
    @State private var customSecret: String = ""
    @State private var onlyUrgentTypes = true

    @State private var isAdding = false
    @State private var testingWebhookId: Int64?
    @State private var lastTestBanner: String?

    private enum PresetMode: String, CaseIterable, Identifiable {
        case ntfy
        case telegram
        case homeAssistant
        case custom

        var id: String { rawValue }

        var title: String {
            switch self {
            case .ntfy: return "ntfy.sh"
            case .telegram: return "Telegram"
            case .homeAssistant: return "Home Assistant"
            case .custom: return VigiaL10n.tr("Custom")
            }
        }
    }

    public init() {}

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                if !store.isLinked {
                    VigiaEmptyState(
                        symbol: "paperplane",
                        title: "Sin agente 24/7 vinculado",
                        message: "Empareja tu servidor Vigía primero para registrar webhooks push directos desde tu red."
                    )
                } else {
                    quickPushAssistantCard
                    if let lastTestBanner {
                        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.radarCyan, padding: 12) {
                            Text(lastTestBanner)
                                .font(.caption)
                                .foregroundStyle(VigiaColor.textPrimary)
                        }
                    }
                    registeredWebhooksCard
                }
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle("Webhooks Push")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if store.isLinked {
                await store.refreshWebhooks()
            }
        }
    }

    private var quickPushAssistantCard: some View {
        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.electricBlue) {
            VStack(alignment: .leading, spacing: 12) {
                Label("Asistente rápido de notificaciones iPhone", systemImage: "iphone.badge.play")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                Text("Como iOS puede espaciar las tareas de fondo cuando la app lleva horas cerrada, puedes registrar en 1 toque un webhook en el Agente para recibir push al instante.")
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textSecondary)

                Picker("Plantilla", selection: $presetMode) {
                    ForEach(PresetMode.allCases) { mode in
                        Text(mode.title).tag(mode)
                    }
                }
                .pickerStyle(.segmented)

                switch presetMode {
                case .ntfy:
                    ntfyForm
                case .telegram:
                    telegramForm
                case .homeAssistant:
                    homeAssistantForm
                case .custom:
                    customForm
                }

                Toggle("Solo alertas importantes/urgentes", isOn: $onlyUrgentTypes)
                    .font(.caption)
                    .tint(VigiaColor.radarCyan)

                if let urlValidationError {
                    Text(urlValidationError)
                        .font(.caption2)
                        .foregroundStyle(VigiaColor.risk)
                }

                Button {
                    submitWebhook()
                } label: {
                    HStack {
                        if isAdding { ProgressView().controlSize(.small) }
                        Text("Registrar webhook en el Agente")
                            .font(.subheadline.weight(.semibold))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                }
                .buttonStyle(.borderedProminent)
                .tint(VigiaColor.radarCyan)
                .foregroundStyle(VigiaColor.background)
                .disabled(resolvedTargetUrl.isEmpty || urlValidationError != nil || isAdding)
            }
        }
    }

    private var ntfyForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Canal privado para la app gratuita ntfy en iOS:")
                .font(.caption)
                .foregroundStyle(VigiaColor.textSecondary)
            HStack {
                TextField("Tema ntfy", text: $ntfyTopic)
                    .font(.system(.caption, design: .monospaced))
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .padding(9)
                    .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                Button {
                    UIPasteboard.general.string = ntfyTopic
                    lastTestBanner = VigiaL10n.fmt("Topic copied to clipboard: %@", ntfyTopic)
                } label: {
                    Image(systemName: "doc.on.doc")
                }
                .buttonStyle(.bordered)
                .vigiaMinTouchTarget()
                .accessibilityLabel(VigiaL10n.tr("Copy ntfy topic"))
                Button {
                    ntfyTopic = PushWebhookPreset.generateNtfyTopic()
                } label: {
                    Image(systemName: "dice")
                }
                .buttonStyle(.bordered)
                .vigiaMinTouchTarget()
                .accessibilityLabel(VigiaL10n.tr("Generate random topic"))
            }
            Text("URL: \(PushWebhookPreset.ntfyWebhookUrl(topic: ntfyTopic, serverBaseUrl: ntfyServer))")
                .font(.system(.caption2, design: .monospaced))
                .foregroundStyle(VigiaColor.radarCyan)
                .textSelection(.enabled)
        }
    }

    private var telegramForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            TextField("Token del Bot (ej. 123456:ABC-DEF)", text: $telegramToken)
                .font(.system(.caption, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
            TextField("Chat ID (ej. 123456789)", text: $telegramChatId)
                .font(.system(.caption, design: .monospaced))
                .keyboardType(.numbersAndPunctuation)
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
        }
    }

    private var homeAssistantForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            TextField("URL de Home Assistant (ej. http://192.168.1.20:8123)", text: $haBaseUrl)
                .font(.system(.caption, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
            TextField("ID del Webhook en HA", text: $haWebhookId)
                .font(.system(.caption, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
        }
    }

    private var customForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            TextField("URL destino (https://…)", text: $customUrl)
                .font(.system(.caption, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
            TextField("Secreto HMAC opcional (X-Vigia-Signature)", text: $customSecret)
                .font(.system(.caption, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .padding(9)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
        }
    }

    private var registeredWebhooksCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text(VigiaL10n.fmt("Active webhooks (%ld)", store.webhooks.count))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                if store.webhooks.isEmpty {
                    Text("No hay webhooks registrados todavía.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                } else {
                    ForEach(store.webhooks) { hook in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(hook.url)
                                .font(.system(.caption, design: .monospaced).weight(.semibold))
                                .foregroundStyle(VigiaColor.textPrimary)
                                .lineLimit(2)

                            HStack {
                                if let status = hook.lastStatus, status > 0 {
                                    VigiaStatusChip(
                                        "HTTP \(status)",
                                        color: (200...299).contains(status) ? VigiaColor.ok : VigiaColor.risk
                                    )
                                }
                                Text(hook.types.isEmpty ? VigiaL10n.tr("All types") : VigiaL10n.fmt("%ld types", hook.types.count))
                                    .font(.caption2)
                                    .foregroundStyle(VigiaColor.textSecondary)
                                Spacer()
                                Button {
                                    testingWebhookId = hook.id
                                    Task {
                                        if let res = await store.testWebhook(id: hook.id) {
                                            lastTestBanner = res.ok
                                                ? VigiaL10n.fmt("Test sent successfully (HTTP %ld).", res.status ?? 200)
                                                : VigiaL10n.fmt("Webhook test failed: %@", res.error ?? VigiaL10n.tr("error"))
                                        }
                                        testingWebhookId = nil
                                    }
                                } label: {
                                    if testingWebhookId == hook.id {
                                        ProgressView().controlSize(.small)
                                    } else {
                                        Text("Probar")
                                            .font(.caption.weight(.semibold))
                                    }
                                }
                                .buttonStyle(.bordered)
                                .tint(VigiaColor.radarCyan)
                                .vigiaMinTouchTarget()

                                Button(role: .destructive) {
                                    Task { await store.deleteWebhook(id: hook.id) }
                                } label: {
                                    Image(systemName: "trash")
                                        .font(.caption)
                                }
                                .buttonStyle(.bordered)
                                .vigiaMinTouchTarget()
                                .accessibilityLabel(VigiaL10n.tr("Delete webhook"))
                            }

                            if let err = hook.lastError, !err.isEmpty {
                                Text(err)
                                    .font(.caption2)
                                    .foregroundStyle(VigiaColor.risk)
                            }
                        }
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 12).fill(VigiaColor.surfaceElevated))
                    }
                }
            }
        }
    }

    private var resolvedTargetUrl: String {
        switch presetMode {
        case .ntfy:
            let t = ntfyTopic.trimmingCharacters(in: .whitespacesAndNewlines)
            return t.isEmpty ? "" : PushWebhookPreset.ntfyWebhookUrl(topic: t, serverBaseUrl: ntfyServer)
        case .telegram:
            let tok = telegramToken.trimmingCharacters(in: .whitespacesAndNewlines)
            let chat = telegramChatId.trimmingCharacters(in: .whitespacesAndNewlines)
            return (tok.isEmpty || chat.isEmpty) ? "" : PushWebhookPreset.telegramWebhookUrl(botToken: tok, chatId: chat)
        case .homeAssistant:
            let base = haBaseUrl.trimmingCharacters(in: .whitespacesAndNewlines)
            let id = haWebhookId.trimmingCharacters(in: .whitespacesAndNewlines)
            return (base.isEmpty || id.isEmpty) ? "" : PushWebhookPreset.homeAssistantWebhookUrl(baseUrl: base, webhookId: id)
        case .custom:
            return customUrl.trimmingCharacters(in: .whitespacesAndNewlines)
        }
    }

    private var urlValidationError: String? {
        let candidate = resolvedTargetUrl
        guard !candidate.isEmpty else { return nil }
        return WebhookUrlValidator.validate(candidate).errorMessage
    }

    private func submitWebhook() {
        let url = resolvedTargetUrl
        guard !url.isEmpty, urlValidationError == nil else { return }
        let types = onlyUrgentTypes ? PushWebhookPreset.defaultUrgentAlertTypes : []
        let secret = presetMode == .custom && !customSecret.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? customSecret.trimmingCharacters(in: .whitespacesAndNewlines)
            : nil

        isAdding = true
        Task {
            let ok = await store.addWebhook(url: url, types: types, secret: secret)
            if ok {
                lastTestBanner = VigiaL10n.tr("Webhook registered successfully on Agent.")
            }
            isAdding = false
        }
    }
}
