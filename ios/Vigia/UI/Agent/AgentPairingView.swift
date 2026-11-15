import SwiftUI
import UIKit

public struct AgentPairingView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.dismiss) private var dismiss

    @StateObject private var bonjour = AgentBonjourBrowser()

    @State private var host = ""
    @State private var portText = "8787"
    @State private var pairingCode = ""
    @State private var clientName = UIDevice.current.name
    @State private var probedAgent: AgentInfoProbe?
    @State private var expectedCertPrefix = ""
    @State private var isProbing = false
    @State private var isPairing = false
    @State private var errorMessage: String?

    public init() {}

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 14) {
                    discoveredAgentsCard
                    manualAddressCard
                    if let probed = probedAgent {
                        tlsVerificationAndCodeCard(probed)
                    }
                    if let errorMessage {
                        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.risk, padding: 12) {
                            Text(errorMessage)
                                .font(.caption)
                                .foregroundStyle(VigiaColor.risk)
                        }
                    }
                }
                .padding(16)
            }
            .background(VigiaColor.background.ignoresSafeArea())
            .navigationTitle("Vincular Agente 24/7")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cerrar") { dismiss() }
                }
            }
            .onAppear {
                bonjour.start()
            }
            .onDisappear {
                bonjour.stop()
            }
        }
        .preferredColorScheme(.dark)
    }

    private var discoveredAgentsCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Label("Agentes en tu red (Bonjour)", systemImage: "bonjour")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    if bonjour.isSearching {
                        ProgressView().controlSize(.small)
                    } else {
                        Button("Reintentar") { bonjour.start() }
                            .font(.caption.weight(.semibold))
                            .vigiaMinTouchTarget()
                    }
                }

                if bonjour.discovered.isEmpty {
                    Text("Buscando servicios _vigia._tcp en tu LAN… Si estás fuera de casa o usas Tailscale, escribe la IP abajo.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                } else {
                    ForEach(bonjour.discovered) { svc in
                        Button {
                            host = svc.host
                            portText = "\(svc.port)"
                            expectedCertPrefix = svc.certPrefix
                            probeHost()
                        } label: {
                            HStack {
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(svc.name)
                                        .font(.subheadline.weight(.semibold))
                                        .foregroundStyle(VigiaColor.textPrimary)
                                    Text("\(svc.host):\(svc.port)")
                                        .font(.system(.caption, design: .monospaced))
                                        .foregroundStyle(VigiaColor.textSecondary)
                                    if !svc.certPrefix.isEmpty {
                                        Text("TLS SHA-256: \(svc.certPrefix)…")
                                            .font(.system(.caption2, design: .monospaced))
                                            .foregroundStyle(VigiaColor.radarCyan)
                                    }
                                }
                                Spacer()
                                Image(systemName: "arrow.right.circle.fill")
                                    .foregroundStyle(VigiaColor.radarCyan)
                            }
                            .padding(10)
                            .background(RoundedRectangle(cornerRadius: 12).fill(VigiaColor.surfaceElevated))
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    private var manualAddressCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text("Conexión manual (LAN o Tailscale)")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                HStack(spacing: 10) {
                    TextField("IP o host (ej. 192.168.1.50)", text: $host)
                        .keyboardType(.asciiCapable)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))

                    TextField("Puerto", text: $portText)
                        .keyboardType(.numberPad)
                        .frame(minWidth: 78, maxWidth: 110)
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                        .accessibilityLabel(VigiaL10n.tr("Port"))
                }
                .font(.system(.subheadline, design: .monospaced))

                Button {
                    expectedCertPrefix = ""
                    probeHost()
                } label: {
                    HStack {
                        if isProbing { ProgressView().controlSize(.small) }
                        Text("Comprobar agente y certificado TLS")
                            .font(.subheadline.weight(.semibold))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                }
                .buttonStyle(.borderedProminent)
                .tint(VigiaColor.electricBlue)
                .disabled(host.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isProbing)
            }
        }
    }

    private func tlsVerificationAndCodeCard(_ probe: AgentInfoProbe) -> some View {
        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.radarCyan) {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Image(systemName: "lock.shield.fill")
                        .foregroundStyle(VigiaColor.radarCyan)
                    Text("Huella TLS verificada (TOFU)")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                }

                VStack(alignment: .leading, spacing: 4) {
                    Text(VigiaL10n.fmt("Agent: %@ (v%@)", probe.info.name, probe.info.version))
                        .font(.caption.weight(.medium))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Text("SHA-256:")
                        .font(.caption2)
                        .foregroundStyle(VigiaColor.textSecondary)
                    Text(formattedFingerprint(probe.handshakeCertSha256))
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(VigiaColor.radarCyan)
                        .textSelection(.enabled)
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))

                Text("Ejecuta `vigia-agent pair` en tu servidor e introduce el código de 6 dígitos:")
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textSecondary)

                TextField("Código de 6 dígitos", text: $pairingCode)
                    .keyboardType(.numberPad)
                    .font(.system(.title3, design: .monospaced).weight(.bold))
                    .multilineTextAlignment(.center)
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))

                TextField("Nombre de este iPhone", text: $clientName)
                    .font(.subheadline)
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))

                Button {
                    submitPair(probe: probe)
                } label: {
                    HStack {
                        if isPairing { ProgressView().controlSize(.small) }
                        Text("Confirmar huella y vincular")
                            .font(.subheadline.weight(.semibold))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                }
                .buttonStyle(.borderedProminent)
                .tint(VigiaColor.radarCyan)
                .foregroundStyle(VigiaColor.background)
                .disabled(pairingCode.trimmingCharacters(in: .whitespacesAndNewlines).count < 6 || isPairing)
            }
        }
    }

    private func probeHost() {
        let cleanHost = host.trimmingCharacters(in: .whitespacesAndNewlines)
        let rawPort = portText.trimmingCharacters(in: .whitespacesAndNewlines)
        let port = rawPort.isEmpty ? 8787 : (Int(rawPort) ?? -1)
        guard !cleanHost.isEmpty else { return }
        guard (1...65535).contains(port) else {
            errorMessage = VigiaL10n.tr("Enter a valid port between 1 and 65535.")
            return
        }

        errorMessage = nil
        isProbing = true
        probedAgent = nil

        Task {
            do {
                let probe = try await AgentApiClient.probeInfo(host: cleanHost, port: port)
                if !expectedCertPrefix.isEmpty,
                   !probe.handshakeCertSha256.isEmpty,
                   !probe.handshakeCertSha256.lowercased().hasPrefix(expectedCertPrefix.lowercased()) {
                    throw AgentApiError.certPinMismatch(expected: expectedCertPrefix, actual: probe.handshakeCertSha256)
                }
                probedAgent = probe
            } catch {
                errorMessage = error.localizedDescription
            }
            isProbing = false
        }
    }

    private func submitPair(probe: AgentInfoProbe) {
        let code = pairingCode.trimmingCharacters(in: .whitespacesAndNewlines)
        let client = clientName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "iPhone" : clientName
        errorMessage = nil
        isPairing = true

        Task {
            do {
                let link = try await AgentApiClient.pair(
                    host: probe.host,
                    port: probe.port,
                    code: code,
                    clientName: client,
                    expectedSha256: probe.handshakeCertSha256
                )
                await store.linkAgent(link)
                isPairing = false
                dismiss()
            } catch {
                errorMessage = error.localizedDescription
                isPairing = false
            }
        }
    }

    private func formattedFingerprint(_ hex: String) -> String {
        let clean = hex.uppercased()
        var groups: [String] = []
        var idx = clean.startIndex
        while idx < clean.endIndex {
            let end = clean.index(idx, offsetBy: 4, limitedBy: clean.endIndex) ?? clean.endIndex
            groups.append(String(clean[idx..<end]))
            idx = end
        }
        return groups.joined(separator: " ")
    }
}
