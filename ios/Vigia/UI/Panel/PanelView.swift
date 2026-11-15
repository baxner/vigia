import SwiftUI

public struct PanelView: View {
    @EnvironmentObject private var store: AppStore
    @Binding public var selectedTab: VigiaTab
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    @State private var showPairing = false

    public init(selectedTab: Binding<VigiaTab>) {
        self._selectedTab = selectedTab
    }

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    if let error = store.bannerError {
                        bannerErrorCard(error)
                    }

                    networkHeaderCard

                    RadarHeroView(
                        devices: store.devices.filter(\.online),
                        scanning: store.isScanning,
                        reduceMotion: reduceMotion
                    ) {
                        selectedTab = .devices
                    }

                    scanActionBar

                    metricsRow

                    surveillanceCard

                    attentionCard

                    if !store.devices.isEmpty {
                        byTypeCard
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
            }
            .background(VigiaColor.background.ignoresSafeArea())
            .navigationTitle("Vigía")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    if store.isLinked {
                        NavigationLink {
                            AgentSettingsView()
                        } label: {
                            Image(systemName: "server.rack")
                                .foregroundStyle(store.agentOnline ? VigiaColor.ok : VigiaColor.amber)
                                .vigiaMinTouchTarget()
                        }
                        .accessibilityLabel(Text(VigiaL10n.tr("Ajustes del agente 24/7")))
                        .accessibilityValue(Text(store.agentOnline ? VigiaL10n.tr("En línea") : VigiaL10n.tr("Sin respuesta")))
                    } else {
                        Button {
                            showPairing = true
                        } label: {
                            Image(systemName: "link.badge.plus")
                                .foregroundStyle(VigiaColor.radarCyan)
                                .vigiaMinTouchTarget()
                        }
                        .accessibilityLabel(Text(VigiaL10n.tr("Vincular Agente 24/7")))
                    }
                }

                ToolbarItem(placement: .topBarTrailing) {
                    NavigationLink {
                        AlertsView()
                    } label: {
                        ZStack(alignment: .topTrailing) {
                            Image(systemName: "bell")
                                .foregroundStyle(VigiaColor.textPrimary)
                            if store.unreadAlertCount > 0 {
                                Text("\(min(99, store.unreadAlertCount))")
                                    .font(.caption2.weight(.bold))
                                    .foregroundStyle(VigiaColor.background)
                                    .padding(.horizontal, 4)
                                    .background(Capsule().fill(VigiaColor.risk))
                                    .offset(x: 6, y: -6)
                            }
                        }
                        .vigiaMinTouchTarget()
                    }
                    .accessibilityLabel(Text(VigiaL10n.tr("Alertas")))
                    .accessibilityValue(Text(store.unreadAlertCount > 0
                                             ? VigiaL10n.fmt("%ld sin leer", store.unreadAlertCount)
                                             : VigiaL10n.tr("Sin alertas nuevas")))
                }
            }
            .sheet(isPresented: $showPairing) {
                AgentPairingView()
            }
            .refreshable {
                if store.isLinked {
                    await store.refreshAgentWorkspace()
                } else {
                    await store.refreshWifi()
                }
            }
        }
    }

    private func bannerErrorCard(_ message: String) -> some View {
        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.risk, padding: 12) {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(VigiaColor.risk)
                Text(message)
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textPrimary)
                Spacer()
                Button {
                    store.clearBannerError()
                } label: {
                    Image(systemName: "xmark")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(VigiaColor.textSecondary)
                        .vigiaMinTouchTarget()
                }
                .accessibilityLabel(Text(VigiaL10n.tr("Cerrar aviso")))
            }
        }
    }

    private var networkHeaderCard: some View {
        VigiaGlassCard(padding: 14) {
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    HStack(spacing: 6) {
                        Text(store.wifiSnapshot.isAvailable ? store.wifiSnapshot.ssid : "Red local")
                            .font(.headline)
                            .foregroundStyle(VigiaColor.textPrimary)
                        if store.networkSnapshot.isVpnActive {
                            VigiaStatusChip("VPN", color: VigiaColor.electricBlue)
                        }
                    }
                    Text(store.activeSubnetLabel)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(VigiaColor.textSecondary)
                }
                Spacer()
                if store.isLinked {
                    VigiaStatusChip(
                        store.agentOnline ? "24/7 activo" : "Agente sin enlace",
                        color: store.agentOnline ? VigiaColor.ok : VigiaColor.amber,
                        icon: store.agentOnline ? "checkmark.shield" : "exclamationmark.circle"
                    )
                } else {
                    VigiaStatusChip(
                        VigiaEngineBridge.shared.isCoreEngineLinked ? "Motor Go" : "Modo iPhone",
                        color: VigiaColor.radarCyan
                    )
                }
            }
        }
    }

    private var scanActionBar: some View {
        HStack(spacing: 12) {
            Button {
                Task { await store.triggerScan() }
            } label: {
                HStack(spacing: 8) {
                    if store.isScanning {
                        ProgressView()
                            .tint(VigiaColor.background)
                    } else {
                        Image(systemName: "dot.radiowaves.left.and.right")
                    }
                    Text(scanButtonTitle)
                        .font(.subheadline.weight(.semibold))
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 12)
                .background(
                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                        .fill(VigiaColor.radarCyan)
                )
                .foregroundStyle(VigiaColor.background)
            }
            .disabled(store.isScanning && store.isLinked)

            if store.isScanning && !store.isLinked {
                Button {
                    store.cancelScan()
                } label: {
                    Text("Detener")
                        .font(.subheadline.weight(.semibold))
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .fill(VigiaColor.surfaceElevated)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                                        .strokeBorder(VigiaColor.outline, lineWidth: 1)
                                )
                        )
                        .foregroundStyle(VigiaColor.risk)
                }
            }
        }
    }

    private var scanButtonTitle: String {
        guard store.isScanning else {
            return store.isLinked ? VigiaL10n.tr("Escanear ahora (Agente)") : VigiaL10n.tr("Escanear red local")
        }
        if store.isLinked {
            return VigiaL10n.tr("Escaneando en el agente…")
        }
        if store.scanTotal > 0 {
            return VigiaL10n.fmt("Escaneando %@ %ld/%ld…", store.scanStage, store.scanDone, store.scanTotal)
        }
        return VigiaL10n.tr("Escaneando red…")
    }

    @ViewBuilder
    private var metricsRow: some View {
        let layout = dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: 10))
            : AnyLayout(HStackLayout(spacing: 10))
        layout {
            Button {
                selectedTab = .devices
            } label: {
                VigiaGlassCard(padding: 12) {
                    VStack(alignment: .leading, spacing: 6) {
                        Label("Equipos", systemImage: "desktopcomputer")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Text("\(store.onlineDeviceCount)")
                            .font(.system(.title2, design: .monospaced).weight(.bold))
                            .foregroundStyle(VigiaColor.radarCyan)
                        Text(VigiaL10n.fmt("%ld conocidos", store.devices.count))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                }
            }
            .buttonStyle(.plain)

            NavigationLink {
                SecurityView()
            } label: {
                let score = store.securityReport?.score ?? 100
                let scoreColor: Color = score >= 85 ? VigiaColor.ok : (score >= 60 ? VigiaColor.amber : VigiaColor.risk)
                let scoreStatus: String = score >= 85
                    ? VigiaL10n.tr("Protegida")
                    : (score >= 60 ? VigiaL10n.tr("Revisar") : VigiaL10n.tr("Riesgo"))
                VigiaGlassCard(padding: 12) {
                    VStack(alignment: .leading, spacing: 6) {
                        Label("Seguridad", systemImage: "shield.lefthalf.filled")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Text("\(score)")
                            .font(.system(.title2, design: .monospaced).weight(.bold))
                            .foregroundStyle(scoreColor)
                        Text(VigiaL10n.fmt("de 100 · %@", scoreStatus))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                    }
                }
            }
            .buttonStyle(.plain)

            NavigationLink {
                InternetView()
            } label: {
                let internet = store.internetStatus
                let stateColor: Color = {
                    switch internet?.state {
                    case "up": return VigiaColor.ok
                    case "degraded": return VigiaColor.amber
                    case "down": return VigiaColor.risk
                    default: return VigiaColor.textSecondary
                    }
                }()
                let latencyText = internet?.now.internetMs.map { "\(Int($0.rounded())) ms" }
                    ?? (store.networkSnapshot.isWifi ? VigiaL10n.tr("Local") : "—")
                VigiaGlassCard(padding: 12) {
                    VStack(alignment: .leading, spacing: 6) {
                        Label("Internet", systemImage: "globe")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Text(latencyText)
                            .font(.system(.title3, design: .monospaced).weight(.bold))
                            .foregroundStyle(stateColor)
                            .lineLimit(1)
                            .minimumScaleFactor(0.75)
                        Text(internetStateLabel(internet?.state))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                }
            }
            .buttonStyle(.plain)
        }
    }

    private var surveillanceCard: some View {
        VigiaGlassCard(highlighted: store.isLinked, glowColor: VigiaColor.electricBlue) {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Label("Vigilancia 24/7", systemImage: "antenna.radiowaves.left.and.right")
                        .font(.headline)
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    if let info = store.agentInfo {
                        Text(info.name)
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(VigiaColor.radarCyan)
                    }
                }

                if store.isLinked {
                    if let warning = store.agentInfo?.warning, !warning.isEmpty {
                        Text(warning)
                            .font(.caption)
                            .foregroundStyle(VigiaColor.amber)
                    } else if let err = store.agentReachError, !store.agentOnline {
                        Text(err)
                            .font(.caption)
                            .foregroundStyle(VigiaColor.amber)
                    } else if let lastScan = store.agentInfo?.lastScanAt {
                        Text(VigiaL10n.fmt("Último barrido del agente: %@", VigiaDate.relativeDescription(from: lastScan)))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    } else {
                        Text("Agente vinculado. Recibiendo eventos y alertas en tiempo real.")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }

                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 112), spacing: 8)], alignment: .leading, spacing: 8) {
                        NavigationLink {
                            AlertsView()
                        } label: {
                            VigiaStatusChip(
                                VigiaL10n.fmt("Alertas (%ld)", store.unreadAlertCount),
                                color: store.unreadAlertCount > 0 ? VigiaColor.risk : VigiaColor.radarCyan,
                                icon: "bell"
                            )
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                        }
                        NavigationLink {
                            WebhooksView()
                        } label: {
                            VigiaStatusChip(VigiaL10n.tr("Push iPhone"), color: VigiaColor.electricBlue, icon: "paperplane")
                                .frame(minHeight: 44)
                                .contentShape(Rectangle())
                        }
                        NavigationLink {
                            AgentSettingsView()
                        } label: {
                            VigiaStatusChip(VigiaL10n.tr("Ajustes"), color: VigiaColor.textSecondary, icon: "gearshape")
                                .frame(minHeight: 44)
                                .contentShape(Rectangle())
                        }
                    }
                } else {
                    Text("Empareja tu servidor Vigía en la red local para tener historial de equipos, detector de caídas de internet y notificaciones instantáneas.")
                        .font(.subheadline)
                        .foregroundStyle(VigiaColor.textSecondary)
                    Button {
                        showPairing = true
                    } label: {
                        Label("Vincular Agente 24/7", systemImage: "qrcode")
                            .font(.subheadline.weight(.semibold))
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(VigiaColor.electricBlue)
                }
            }
        }
    }

    private var attentionCard: some View {
        let riskyDevices = store.devices.filter { !VigiaEngineBridge.shared.riskyPorts(among: $0.last.openPorts).isEmpty }
        let unknownDevices = store.devices.filter { $0.effectiveType == .unknown && $0.online }
        return NavigationLink {
            SecurityView()
        } label: {
            VigiaGlassCard {
                HStack(spacing: 12) {
                    Image(systemName: riskyDevices.isEmpty ? "checkmark.seal.fill" : "exclamationmark.shield.fill")
                        .font(.title2)
                        .foregroundStyle(riskyDevices.isEmpty ? VigiaColor.ok : VigiaColor.risk)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(riskyDevices.isEmpty
                             ? VigiaL10n.tr("Sin puertos de riesgo inmediatos")
                             : VigiaL10n.fmt("%ld equipo(s) requieren atención", riskyDevices.count))
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(VigiaColor.textPrimary)
                        Text(unknownDevices.isEmpty
                             ? VigiaL10n.tr("Toca para revisar el informe completo de seguridad.")
                             : VigiaL10n.fmt("Además hay %ld equipo(s) sin identificar.", unknownDevices.count))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    Spacer()
                    Image(systemName: "chevron.right")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(VigiaColor.textTertiary)
                }
            }
        }
        .buttonStyle(.plain)
    }

    private var byTypeCard: some View {
        let counts = Dictionary(grouping: store.devices.filter(\.online)) { $0.effectiveType }
            .map { (type: $0.key, count: $0.value.count) }
            .sorted { $0.count > $1.count }

        return VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text("Equipos activos por tipo")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 110), spacing: 8)], spacing: 8) {
                    ForEach(counts, id: \.type) { item in
                        HStack(spacing: 6) {
                            Image(systemName: item.type.sfSymbol)
                                .foregroundStyle(VigiaColor.radarCyan)
                            Text(item.type.labelEs)
                                .font(.caption)
                                .foregroundStyle(VigiaColor.textSecondary)
                                .lineLimit(1)
                            Spacer()
                            Text("\(item.count)")
                                .font(.system(.caption, design: .monospaced).weight(.bold))
                                .foregroundStyle(VigiaColor.textPrimary)
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 8)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                    }
                }
            }
        }
    }

    private func internetStateLabel(_ state: String?) -> String {
        switch state {
        case "up": return VigiaL10n.tr("En línea")
        case "degraded": return VigiaL10n.tr("Inestable")
        case "down": return VigiaL10n.tr("Sin salida")
        default: return store.isLinked ? VigiaL10n.tr("Sincronizando") : VigiaL10n.tr("Modo local")
        }
    }
}

// MARK: - Canvas animado del Radar Hero

public struct RadarHeroView: View {
    public let devices: [KnownDevice]
    public let scanning: Bool
    public let reduceMotion: Bool
    public let onTap: () -> Void

    private let ringFractions: [Double] = [0.0, 0.40, 0.67, 0.94]

    public var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 30.0, paused: !scanning || reduceMotion)) { timeline in
            let sweepDegrees: Double = {
                guard scanning && !reduceMotion else { return 40 }
                let period = 2.6
                let t = timeline.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: period)
                return (t / period) * 360.0
            }()

            let angles = VigiaRadarLayout.angles(for: devices)

            Canvas { context, size in
                let maxRadius = min(size.width, size.height) / 2.0 * 0.88
                let center = CGPoint(x: size.width / 2.0, y: size.height / 2.0)
                let restAlpha = scanning ? 1.0 : 0.6

                let bgGradient = Gradient(colors: [
                    VigiaColor.surfaceElevated.opacity(0.75),
                    .clear
                ])
                context.fill(
                    Path(ellipseIn: CGRect(
                        x: center.x - maxRadius * 1.15,
                        y: center.y - maxRadius * 1.15,
                        width: maxRadius * 2.3,
                        height: maxRadius * 2.3
                    )),
                    with: .radialGradient(bgGradient, center: center, startRadius: 0, endRadius: maxRadius * 1.15)
                )

                for frac in ringFractions.dropFirst() {
                    let r = maxRadius * frac
                    let rect = CGRect(x: center.x - r, y: center.y - r, width: r * 2, height: r * 2)
                    context.stroke(
                        Path(ellipseIn: rect),
                        with: .color(VigiaColor.radarCyan.opacity(0.16 * restAlpha)),
                        lineWidth: 1
                    )
                }

                var cross = Path()
                cross.move(to: CGPoint(x: center.x - maxRadius, y: center.y))
                cross.addLine(to: CGPoint(x: center.x + maxRadius, y: center.y))
                cross.move(to: CGPoint(x: center.x, y: center.y - maxRadius))
                cross.addLine(to: CGPoint(x: center.x, y: center.y + maxRadius))
                context.stroke(cross, with: .color(VigiaColor.radarCyan.opacity(0.11 * restAlpha)), lineWidth: 1)

                if scanning {
                    let startRad = Angle.degrees(sweepDegrees - 65).radians
                    let endRad = Angle.degrees(sweepDegrees).radians
                    var wedge = Path()
                    wedge.move(to: center)
                    wedge.addArc(
                        center: center,
                        radius: maxRadius,
                        startAngle: .radians(startRad),
                        endAngle: .radians(endRad),
                        clockwise: false
                    )
                    wedge.closeSubpath()
                    context.fill(
                        wedge,
                        with: .conicGradient(
                            Gradient(stops: [
                                .init(color: .clear, location: 0.0),
                                .init(color: VigiaColor.radarCyan.opacity(0.08), location: 0.65),
                                .init(color: VigiaColor.radarCyan.opacity(0.38), location: 1.0)
                            ]),
                            center: center,
                            angle: .degrees(sweepDegrees - 65)
                        )
                    )
                }

                for device in devices {
                    let ring = device.effectiveType.radarRing
                    let frac = ring < ringFractions.count ? ringFractions[ring] : 0.94
                    let angleDeg = angles[device.id] ?? 0
                    let pos: CGPoint
                    if frac == 0 {
                        pos = center
                    } else {
                        let rad = angleDeg * Double.pi / 180.0
                        pos = CGPoint(
                            x: center.x + maxRadius * frac * cos(rad),
                            y: center.y + maxRadius * frac * sin(rad)
                        )
                    }
                    let color = device.statusColor
                    let radius: CGFloat = ring == 0 ? 7.0 : 5.0
                    let haloRect = CGRect(
                        x: pos.x - radius * 3,
                        y: pos.y - radius * 3,
                        width: radius * 6,
                        height: radius * 6
                    )
                    context.fill(
                        Path(ellipseIn: haloRect),
                        with: .radialGradient(
                            Gradient(colors: [color.opacity(0.35), .clear]),
                            center: pos,
                            startRadius: 0,
                            endRadius: radius * 3
                        )
                    )
                    let dotRect = CGRect(x: pos.x - radius, y: pos.y - radius, width: radius * 2, height: radius * 2)
                    context.fill(Path(ellipseIn: dotRect), with: .color(color))
                    context.stroke(Path(ellipseIn: dotRect), with: .color(VigiaColor.background), lineWidth: 1.5)
                }
            }
            .frame(height: 230)
            .frame(maxWidth: .infinity)
            .background(
                RoundedRectangle(cornerRadius: 20, style: .continuous)
                    .fill(VigiaColor.surface)
                    .overlay(
                        RoundedRectangle(cornerRadius: 20, style: .continuous)
                            .strokeBorder(VigiaColor.outline, lineWidth: 1)
                    )
            )
            .contentShape(Rectangle())
            .onTapGesture(perform: onTap)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text(scanning
                                     ? VigiaL10n.tr("Escaneando la red")
                                     : VigiaL10n.fmt("Radar de red con %ld equipos activos", devices.count)))
            .accessibilityHint(Text("Toca para ver la lista completa de equipos"))
            .accessibilityAddTraits(.isButton)
        }
    }
}
