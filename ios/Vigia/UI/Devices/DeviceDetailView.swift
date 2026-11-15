import SwiftUI

public struct DeviceDetailView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.dismiss) private var dismiss

    public let initialDevice: KnownDevice

    @State private var events: [DeviceEvent] = []
    @State private var isScanningPorts = false
    @State private var showEditSheet = false
    @State private var confirmForget = false

    public init(initialDevice: KnownDevice) {
        self.initialDevice = initialDevice
    }

    private var device: KnownDevice {
        store.devices.first(where: { $0.id == initialDevice.id }) ?? initialDevice
    }

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                headerCard
                identityCard
                portsAndServicesCard
                findingsCard
                if !events.isEmpty {
                    eventsCard
                }
                if store.isLinked {
                    actionsCard
                }
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle(device.displayName)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if store.isLinked {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Editar") {
                        showEditSheet = true
                    }
                }
            }
        }
        .sheet(isPresented: $showEditSheet) {
            DeviceEditView(device: device)
        }
        .confirmationDialog(
            "¿Olvidar este equipo?",
            isPresented: $confirmForget,
            titleVisibility: .visible
        ) {
            Button("Olvidar del historial", role: .destructive) {
                Task {
                    if await store.forgetDevice(id: device.id) {
                        dismiss()
                    }
                }
            }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("Se borrará del inventario y de su historial de eventos en el agente. Si vuelve a aparecer en la red se registrará como nuevo.")
        }
        .task {
            if store.isLinked {
                events = await store.fetchDeviceEvents(deviceId: device.id)
            }
        }
    }

    private var headerCard: some View {
        let color = device.statusColor
        return VigiaGlassCard(highlighted: true, glowColor: color) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 12) {
                    ZStack {
                        Circle()
                            .fill(color.opacity(0.18))
                            .frame(width: 52, height: 52)
                        Image(systemName: device.effectiveType.sfSymbol)
                            .font(.title2.weight(.semibold))
                            .foregroundStyle(color)
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(spacing: 6) {
                            Text(device.displayName)
                                .font(.title3.weight(.bold))
                                .foregroundStyle(VigiaColor.textPrimary)
                            if device.custom.important {
                                Image(systemName: "star.fill")
                                    .foregroundStyle(VigiaColor.amber)
                            }
                        }
                        Text(device.effectiveType.labelEs)
                            .font(.subheadline)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    Spacer()
                    VigiaStatusChip(
                        device.online ? VigiaL10n.tr("En línea") : VigiaL10n.tr("Desconectado"),
                        color: device.online ? VigiaColor.ok : VigiaColor.textTertiary
                    )
                }

                Divider().background(VigiaColor.outline)

                infoRow("IP local", value: device.lastIp, mono: true)
                if let mac = device.mac ?? device.last.mac, !mac.isEmpty {
                    infoRow("MAC", value: mac.uppercased(), mono: true)
                }
                if let vendor = device.last.vendor, !vendor.isEmpty {
                    infoRow("Fabricante OUI", value: vendor)
                }
                if let host = device.last.hostname, !host.isEmpty {
                    infoRow("Hostname", value: host, mono: true)
                }
                if !device.last.ipv6.isEmpty {
                    infoRow("IPv6", value: device.last.ipv6.joined(separator: ", "), mono: true)
                }
                if let loc = device.custom.location, !loc.isEmpty {
                    infoRow("Ubicación", value: loc)
                }
                if let notes = device.custom.notes, !notes.isEmpty {
                    infoRow("Notas", value: notes)
                }
                infoRow("Última vez visto", value: VigiaDate.relativeDescription(from: device.lastSeen))
            }
        }
    }

    private var identityCard: some View {
        let id = device.last.identity
        return VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 12) {
                    VigiaConfidenceRing(value: id.confidence, color: VigiaColor.electricBlue)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Identificación del motor")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(VigiaColor.textPrimary)
                        let parts = [id.brand, id.model, id.os].filter { !$0.isEmpty }
                        Text(parts.isEmpty ? VigiaL10n.tr("Confianza del perfilado automático") : parts.joined(separator: " · "))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                }
                if !id.reason.isEmpty {
                    Text(VigiaL10n.fmt("Pista principal: %@", id.reason))
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                }
                if !device.last.sources.isEmpty {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 6) {
                            ForEach(device.last.sources, id: \.self) { src in
                                VigiaStatusChip(VigiaRadarLayout.sourceLabelEs(src), color: VigiaColor.electricBlue)
                            }
                        }
                    }
                }
            }
        }
    }

    private var portsAndServicesCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Text("Puertos y servicios")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    Button {
                        isScanningPorts = true
                        Task {
                            _ = await store.scanPortsOnDevice(device, spec: "all")
                            isScanningPorts = false
                        }
                    } label: {
                        if isScanningPorts {
                            ProgressView().controlSize(.small)
                        } else {
                            Label("Sondear catálogo", systemImage: "magnifyingglass")
                                .font(.caption.weight(.semibold))
                        }
                    }
                    .buttonStyle(.bordered)
                    .tint(VigiaColor.radarCyan)
                    .frame(minHeight: 44)
                }

                if device.last.openPorts.isEmpty && device.last.services.isEmpty {
                    Text("No se detectaron puertos TCP ni servicios anunciados abiertos.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                } else {
                    ForEach(device.last.openPorts, id: \.self) { port in
                        let info = VigiaEngineBridge.shared.portInfo(port)
                        let riskColor: Color = {
                            switch info?.risk {
                            case "high": return VigiaColor.risk
                            case "medium": return VigiaColor.amber
                            case "low": return VigiaColor.electricBlue
                            default: return VigiaColor.ok
                            }
                        }()
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(":\(port)")
                                    .font(.system(.subheadline, design: .monospaced).weight(.bold))
                                    .foregroundStyle(riskColor)
                                Text(info?.name ?? "Puerto TCP")
                                    .font(.subheadline.weight(.medium))
                                    .foregroundStyle(VigiaColor.textPrimary)
                                Spacer()
                                if let risk = info?.risk, risk != "none" {
                                    VigiaStatusChip(risk.uppercased(), color: riskColor)
                                }
                            }
                            if let why = info?.why, !why.isEmpty {
                                Text(why)
                                    .font(.caption)
                                    .foregroundStyle(VigiaColor.textSecondary)
                            }
                            if let action = info?.action, !action.isEmpty {
                                Text(action)
                                    .font(.caption)
                                    .foregroundStyle(VigiaColor.radarCyan)
                            }
                        }
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 12).fill(VigiaColor.surfaceElevated))
                    }

                    ForEach(device.last.services, id: \.self) { svc in
                        HStack {
                            Text(svc.proto.uppercased())
                                .font(.system(.caption2, design: .monospaced).weight(.bold))
                                .foregroundStyle(VigiaColor.electricBlue)
                            Text(svc.name)
                                .font(.caption.weight(.medium))
                                .foregroundStyle(VigiaColor.textPrimary)
                            Spacer()
                            if let extra = svc.info, !extra.isEmpty {
                                Text(extra)
                                    .font(.caption2)
                                    .foregroundStyle(VigiaColor.textTertiary)
                                    .lineLimit(1)
                            }
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder
    private var findingsCard: some View {
        let matched = (store.securityReport?.findings ?? []).filter {
            $0.ip == device.lastIp || $0.deviceId == device.id
        }
        if !matched.isEmpty {
            VigiaGlassCard(highlighted: true, glowColor: VigiaColor.amber) {
                VStack(alignment: .leading, spacing: 10) {
                    Text("Hallazgos en este equipo")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    ForEach(matched) { finding in
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                VigiaStatusChip(
                                    finding.severityLevel.labelEs,
                                    color: finding.severityLevel == .high ? VigiaColor.risk : VigiaColor.amber
                                )
                                Text(finding.title)
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(VigiaColor.textPrimary)
                            }
                            Text(finding.action)
                                .font(.caption)
                                .foregroundStyle(VigiaColor.textSecondary)
                            Button(finding.acknowledged ? VigiaL10n.tr("Deshacer reconocimiento") : VigiaL10n.tr("Ya lo sé, es a propósito")) {
                                Task { await store.toggleAcknowledgeFinding(finding) }
                            }
                            .font(.caption.weight(.semibold))
                            .buttonStyle(.bordered)
                            .tint(finding.acknowledged ? VigiaColor.textSecondary : VigiaColor.radarCyan)
                            .frame(minHeight: 44)
                        }
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 12).fill(VigiaColor.surfaceElevated))
                    }
                }
            }
        }
    }

    private var eventsCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text("Historial reciente")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)
                ForEach(events.prefix(25)) { event in
                    HStack(alignment: .top, spacing: 10) {
                        Circle()
                            .fill(event.type == "WENT_OFFLINE" ? VigiaColor.amber : VigiaColor.radarCyan)
                            .frame(width: 8, height: 8)
                            .padding(.top, 5)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(event.labelEs)
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(VigiaColor.textPrimary)
                            if let detail = event.detail, !detail.isEmpty {
                                Text(detail)
                                    .font(.system(.caption2, design: .monospaced))
                                    .foregroundStyle(VigiaColor.textSecondary)
                            }
                        }
                        Spacer()
                        Text(VigiaDate.relativeDescription(from: event.at))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                }
            }
        }
    }

    private var actionsCard: some View {
        Button(role: .destructive) {
            confirmForget = true
        } label: {
            Label("Olvidar equipo del inventario", systemImage: "trash")
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

    private func infoRow(_ label: String, value: String, mono: Bool = false) -> some View {
        HStack(alignment: .top) {
            Text(label)
                .font(.caption)
                .foregroundStyle(VigiaColor.textSecondary)
            Spacer()
            Text(value)
                .font(mono ? .system(.caption, design: .monospaced) : .caption)
                .foregroundStyle(VigiaColor.textPrimary)
                .multilineTextAlignment(.trailing)
                .textSelection(.enabled)
        }
    }
}
