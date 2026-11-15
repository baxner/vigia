import SwiftUI

public struct SecurityView: View {
    @EnvironmentObject private var store: AppStore
    @State private var expandedIds: Set<String> = []
    @State private var isProbingRouter = false

    public init() {}

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                scoreCard

                if let router = store.securityReport?.router ?? store.localRouterInfo {
                    routerCard(router)
                } else {
                    probeRouterPromptCard
                }

                if let outer = store.securityReport?.outerRing, !outer.devices.isEmpty {
                    outerRingCard(outer)
                }

                findingsSection
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle("Seguridad")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    Task { await store.refreshSecurity() }
                } label: {
                    if store.isLoadingSecurity {
                        ProgressView()
                            .tint(VigiaColor.radarCyan)
                            .vigiaMinTouchTarget()
                    } else {
                        Image(systemName: "arrow.clockwise")
                            .vigiaMinTouchTarget()
                    }
                }
                .accessibilityLabel(Text(VigiaL10n.tr("Actualizar seguridad")))
            }
        }
        .refreshable {
            await store.refreshSecurity()
        }
    }

    private var scoreCard: some View {
        let score = store.securityReport?.score ?? 100
        let activeCount = store.securityReport?.findings.filter { !$0.acknowledged && $0.points > 0 }.count ?? 0
        let color: Color = score >= 85 ? VigiaColor.ok : (score >= 60 ? VigiaColor.amber : VigiaColor.risk)

        return VigiaGlassCard(highlighted: true, glowColor: color) {
            HStack(spacing: 16) {
                VigiaConfidenceRing(value: score, size: 74, color: color)
                VStack(alignment: .leading, spacing: 4) {
                    Text(score >= 85
                         ? VigiaL10n.tr("Red bien protegida")
                         : (score >= 60 ? VigiaL10n.tr("Hay puntos por revisar") : VigiaL10n.tr("Riesgos importantes detectados")))
                        .font(.headline)
                        .foregroundStyle(VigiaColor.textPrimary)
                    Text(activeCount == 0
                         ? VigiaL10n.tr("Sin hallazgos activos que resten puntos.")
                         : VigiaL10n.fmt("%ld hallazgo(s) activos descontando puntos.", activeCount))
                        .font(.subheadline)
                        .foregroundStyle(VigiaColor.textSecondary)
                    if let generated = store.securityReport?.generatedAt {
                        Text(VigiaL10n.fmt("Actualizado %@", VigiaDate.relativeDescription(from: generated)))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                }
            }
        }
    }

    private var probeRouterPromptCard: some View {
        VigiaGlassCard {
            HStack(spacing: 12) {
                Image(systemName: "wifi.router")
                    .font(.title2)
                    .foregroundStyle(VigiaColor.electricBlue)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Puertos y UPnP del router")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Text("Comprueba si el router permite apertura automática de puertos desde tu red.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                }
                Spacer()
                Button {
                    isProbingRouter = true
                    Task {
                        await store.probeLocalRouterFromHere()
                        isProbingRouter = false
                    }
                } label: {
                    if isProbingRouter {
                        ProgressView().controlSize(.small)
                    } else {
                        Text("Probar router")
                            .font(.caption.weight(.semibold))
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(VigiaColor.electricBlue)
                .frame(minHeight: 44)
            }
        }
    }

    private func routerCard(_ router: RouterInfo) -> some View {
        let reach = RouterReach.fromRaw(router.reach)
        return VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Label("Router y redirecciones", systemImage: "wifi.router.fill")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    Button {
                        isProbingRouter = true
                        Task {
                            await store.probeLocalRouterFromHere()
                            isProbingRouter = false
                        }
                    } label: {
                        if isProbingRouter {
                            ProgressView().controlSize(.small)
                                .frame(minHeight: 44)
                        } else {
                            Text("Probar desde aquí")
                                .font(.caption.weight(.semibold))
                                .frame(minHeight: 44)
                                .contentShape(Rectangle())
                        }
                    }
                    .foregroundStyle(VigiaColor.radarCyan)
                }

                HStack(spacing: 8) {
                    VigiaStatusChip(router.upnp ? VigiaL10n.tr("UPnP activo") : VigiaL10n.tr("UPnP apagado"), color: router.upnp ? VigiaColor.amber : VigiaColor.ok)
                    VigiaStatusChip(router.natpmp ? VigiaL10n.tr("NAT-PMP activo") : VigiaL10n.tr("NAT-PMP apagado"), color: router.natpmp ? VigiaColor.amber : VigiaColor.ok)
                    if reach != .unknown {
                        VigiaStatusChip(
                            reach == .internet ? VigiaL10n.tr("IP pública") : VigiaL10n.tr("Doble NAT"),
                            color: reach == .internet ? VigiaColor.electricBlue : VigiaColor.textSecondary
                        )
                    }
                }

                if !router.ip.isEmpty {
                    HStack {
                        Text("Gateway")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Spacer()
                        Text(router.ip)
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(VigiaColor.textPrimary)
                    }
                }
                if !router.externalIp.isEmpty {
                    HStack {
                        Text("IP exterior reportada")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Spacer()
                        Text(router.externalIp)
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(VigiaColor.textPrimary)
                    }
                }

                if router.portMappings.isEmpty {
                    Text("No hay puertos UPnP/NAT-PMP abiertos hacia fuera.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                } else {
                    Divider().background(VigiaColor.outline)
                    Text(VigiaL10n.fmt("Puertos mapeados (%ld)", router.portMappings.count))
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(VigiaColor.textSecondary)
                    ForEach(router.portMappings) { m in
                        HStack {
                            Text(":\(m.externalPort)/\(m.transportProtocol)")
                                .font(.system(.caption, design: .monospaced).weight(.bold))
                                .foregroundStyle(VigiaColor.amber)
                            Image(systemName: "arrow.right")
                                .font(.caption2)
                                .foregroundStyle(VigiaColor.textTertiary)
                            Text("\(m.internalIp):\(m.internalPort)")
                                .font(.system(.caption, design: .monospaced))
                                .foregroundStyle(VigiaColor.textPrimary)
                            Spacer()
                            Text(m.description.isEmpty ? m.source.uppercased() : m.description)
                                .font(.caption2)
                                .foregroundStyle(VigiaColor.textSecondary)
                                .lineLimit(1)
                        }
                        .padding(8)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                    }
                }
            }
        }
    }

    private func outerRingCard(_ outer: OuterRing) -> some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Label("Anillo exterior (delante del router)", systemImage: "network")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    Text("\(outer.devices.count) eq.")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(VigiaColor.electricBlue)
                }
                Text("Subred \(outer.subnet) · Gateway \(outer.gateway)")
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(VigiaColor.textSecondary)
                ForEach(outer.devices, id: \.device.ip) { item in
                    HStack {
                        Text(item.name.isEmpty ? item.device.displayName : item.name)
                            .font(.caption.weight(.medium))
                            .foregroundStyle(VigiaColor.textPrimary)
                        Spacer()
                        Text(item.device.ip)
                            .font(.system(.caption2, design: .monospaced))
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private var findingsSection: some View {
        let all = store.securityReport?.findings ?? []
        let active = all.filter { !$0.acknowledged }
        let acknowledged = all.filter { $0.acknowledged }

        if all.isEmpty {
            VigiaEmptyState(
                symbol: "shield.checkmark",
                title: "Sin hallazgos pendientes",
                message: "Cuando escanees tu red o el agente genere un informe verás aquí recomendaciones claras y accionables."
            )
        } else {
            if !active.isEmpty {
                VStack(alignment: .leading, spacing: 10) {
                    Text(VigiaL10n.fmt("Hallazgos activos (%ld)", active.count))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textSecondary)
                        .padding(.horizontal, 4)
                    ForEach(active) { finding in
                        findingRow(finding)
                    }
                }
            }

            if !acknowledged.isEmpty {
                VStack(alignment: .leading, spacing: 10) {
                    Text(VigiaL10n.fmt("Reconocidos por ti (%ld)", acknowledged.count))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textTertiary)
                        .padding(.horizontal, 4)
                        .padding(.top, 6)
                    ForEach(acknowledged) { finding in
                        findingRow(finding)
                    }
                }
            }
        }
    }

    private func findingRow(_ finding: SecurityFinding) -> some View {
        let expanded = expandedIds.contains(finding.id)
        let color = severityColor(finding.severityLevel, acknowledged: finding.acknowledged)

        return VigiaGlassCard(highlighted: !finding.acknowledged && finding.severityLevel == .high, glowColor: color) {
            VStack(alignment: .leading, spacing: 8) {
                Button {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        if expanded {
                            expandedIds.remove(finding.id)
                        } else {
                            expandedIds.insert(finding.id)
                        }
                    }
                } label: {
                    HStack(alignment: .top, spacing: 10) {
                        VigiaStatusChip(
                            finding.acknowledged ? VigiaL10n.tr("Reconocido") : finding.severityLevel.label,
                            color: color
                        )
                        VStack(alignment: .leading, spacing: 3) {
                            Text(finding.title)
                                .font(.subheadline.weight(.semibold))
                                .foregroundStyle(finding.acknowledged ? VigiaColor.textSecondary : VigiaColor.textPrimary)
                                .multilineTextAlignment(.leading)
                            if !expanded && !finding.detail.isEmpty {
                                Text(finding.detail)
                                    .font(.caption)
                                    .foregroundStyle(VigiaColor.textSecondary)
                                    .lineLimit(2)
                                    .multilineTextAlignment(.leading)
                            }
                        }
                        Spacer()
                        Image(systemName: expanded ? "chevron.up" : "chevron.down")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityHint(Text(expanded ? VigiaL10n.tr("Toca para contraer detalle") : VigiaL10n.tr("Toca para ver detalle y solución")))

                if expanded {
                    if !finding.detail.isEmpty {
                        Text(finding.detail)
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    if !finding.action.isEmpty {
                        HStack(alignment: .top, spacing: 6) {
                            Image(systemName: "lightbulb.fill")
                                .font(.caption)
                                .foregroundStyle(VigiaColor.radarCyan)
                            Text(finding.action)
                                .font(.caption)
                                .foregroundStyle(VigiaColor.textPrimary)
                        }
                        .padding(10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                    }
                    if finding.points > 0 || finding.acknowledged {
                        HStack {
                            if finding.points > 0 && !finding.acknowledged {
                                Text("-\(finding.points) pts")
                                    .font(.system(.caption2, design: .monospaced).weight(.semibold))
                                    .foregroundStyle(color)
                            }
                            Spacer()
                            Button {
                                Task { await store.toggleAcknowledgeFinding(finding) }
                            } label: {
                                Label(
                                    finding.acknowledged ? VigiaL10n.tr("Deshacer reconocimiento") : VigiaL10n.tr("Ya lo sé, es a propósito"),
                                    systemImage: finding.acknowledged ? "arrow.uturn.backward" : "checkmark.circle"
                                )
                                .font(.caption.weight(.semibold))
                            }
                            .buttonStyle(.bordered)
                            .tint(finding.acknowledged ? VigiaColor.textSecondary : VigiaColor.radarCyan)
                            .frame(minHeight: 44)
                        }
                    }
                }
            }
        }
    }

    private func severityColor(_ level: FindingSeverity, acknowledged: Bool) -> Color {
        if acknowledged { return VigiaColor.textTertiary }
        switch level {
        case .high: return VigiaColor.risk
        case .medium: return VigiaColor.amber
        case .low: return VigiaColor.electricBlue
        case .info: return VigiaColor.ok
        }
    }
}
