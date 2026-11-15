import SwiftUI

public struct InternetView: View {
    @EnvironmentObject private var store: AppStore

    public init() {}

    public var body: some View {
        ScrollView {
            VStack(spacing: 14) {
                if !store.isLinked {
                    VigiaEmptyState(
                        symbol: "globe.badge.chevron.backward",
                        title: "Monitor de internet 24/7",
                        message: "Vincula tu Agente Vigía para medir continuamente tu router, módem/ONT, DNS, caídas reales con culpable y pruebas automáticas de velocidad."
                    )
                } else if let status = store.internetStatus {
                    statusHeaderCard(status)
                    if status.ipv6 == "broken" {
                        ipv6BrokenCard
                    }
                    nowLatenciesGrid(status.now)
                    latencyChartCard
                    speedTestCard(status)
                    outagesCard(status.outages)
                } else {
                    VigiaEmptyState(
                        symbol: "hourglass",
                        title: "Esperando primeras mediciones",
                        message: "El agente está recabando muestras de latencia y disponibilidad."
                    )
                }
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle("Internet")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if store.isLinked {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await store.refreshInternet() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .vigiaMinTouchTarget()
                    }
                    .accessibilityLabel(Text(VigiaL10n.tr("Actualizar internet")))
                }
            }
        }
        .refreshable {
            await store.refreshInternet()
        }
        .task {
            if store.isLinked {
                await store.refreshInternet()
            }
        }
    }

    private func statusHeaderCard(_ status: InternetStatusDto) -> some View {
        let info = statusVisuals(status.state)
        return VigiaGlassCard(highlighted: true, glowColor: info.color) {
            HStack(spacing: 12) {
                Image(systemName: info.symbol)
                    .font(.title.weight(.semibold))
                    .foregroundStyle(info.color)
                VStack(alignment: .leading, spacing: 3) {
                    Text(info.title)
                        .font(.headline)
                        .foregroundStyle(VigiaColor.textPrimary)
                    if let since = status.since {
                        Text(VigiaL10n.fmt("En este estado desde %@", VigiaDate.relativeDescription(from: since)))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                }
                Spacer()
                VigiaStatusChip(
                    VigiaL10n.fmt("%.0f%% pérdida", status.now.lossPct),
                    color: status.now.lossPct > 2 ? VigiaColor.risk : VigiaColor.ok
                )
            }
        }
    }

    private var ipv6BrokenCard: some View {
        VigiaGlassCard(highlighted: true, glowColor: VigiaColor.amber, padding: 12) {
            HStack(spacing: 10) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(VigiaColor.amber)
                Text("IPv6 parece bloqueado o roto: tus equipos podrían tardar en abrir algunas webs antes de pasar a IPv4.")
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textPrimary)
            }
        }
    }

    private func nowLatenciesGrid(_ now: InternetNowDto) -> some View {
        LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
            latencyTile(title: "Router local", ms: now.routerMs, color: VigiaColor.radarCyan)
            latencyTile(title: "Módem / ONT", ms: now.modemMs, color: VigiaColor.electricBlue)
            latencyTile(title: "Internet", ms: now.internetMs, color: VigiaColor.ok)
            latencyTile(title: "Resolución DNS", ms: now.dnsMs, color: VigiaColor.amber)
        }
    }

    private func latencyTile(title: String, ms: Double?, color: Color) -> some View {
        VigiaGlassCard(padding: 12) {
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textSecondary)
                Text(ms.map { String(format: "%.1f ms", $0) } ?? "—")
                    .font(.system(.title3, design: .monospaced).weight(.bold))
                    .foregroundStyle(ms == nil ? VigiaColor.textTertiary : color)
            }
        }
    }

    private var latencyChartCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Text("Historial de latencia")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    Picker("Rango", selection: Binding(
                        get: { store.internetRange },
                        set: { next in Task { await store.changeInternetRange(next) } }
                    )) {
                        ForEach(InternetSeriesRange.allCases) { range in
                            Text(range.label).tag(range)
                        }
                    }
                    .pickerStyle(.segmented)
                    .fixedSize()
                }

                InternetSeriesCanvas(points: store.internetSeries)
                    .frame(height: 150)

                HStack(spacing: 14) {
                    legendDot("Internet", color: VigiaColor.ok)
                    legendDot("Router", color: VigiaColor.radarCyan)
                    legendDot("DNS", color: VigiaColor.amber)
                    legendDot("Pérdida", color: VigiaColor.risk)
                }
                .font(.caption2)
                .foregroundStyle(VigiaColor.textSecondary)
            }
        }
    }

    private func speedTestCard(_ status: InternetStatusDto) -> some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Label("Velocidad de línea", systemImage: "speedometer")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                    Spacer()
                    Button {
                        Task { await store.triggerSpeedTest() }
                    } label: {
                        if store.isRunningSpeedTest {
                            HStack(spacing: 6) {
                                ProgressView().controlSize(.small)
                                Text("Probando…")
                            }
                        } else {
                            Text("Probar ahora")
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(VigiaColor.radarCyan)
                    .foregroundStyle(VigiaColor.background)
                    .disabled(store.isRunningSpeedTest)
                }

                if let latest = status.speedTests.first {
                    if let err = latest.error, !err.isEmpty {
                        Text("Última prueba falló: \(err)")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.risk)
                    } else {
                        HStack(spacing: 12) {
                            speedMetric("Bajada", value: latest.downMbps.map { String(format: "%.1f Mbps", $0) } ?? "—", color: VigiaColor.radarCyan)
                            speedMetric("Subida", value: latest.upMbps.map { String(format: "%.1f Mbps", $0) } ?? "—", color: VigiaColor.electricBlue)
                            speedMetric("Ping carga", value: latest.loadedLatencyMs.map { String(format: "%.0f ms", $0) } ?? "—", color: VigiaColor.amber)
                        }
                        Text(VigiaL10n.fmt("Medido %@", VigiaDate.relativeDescription(from: latest.at)))
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                } else {
                    Text("Aún no se ha ejecutado ninguna prueba de velocidad.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                }

                Divider().background(VigiaColor.outline)

                Stepper(value: Binding(
                    get: { store.internetSettings.speedTestsPerDay },
                    set: { next in Task { await store.updateSpeedTestsPerDay(next) } }
                ), in: 0...6) {
                    Text(VigiaL10n.fmt("Pruebas automáticas al día: %ld", store.internetSettings.speedTestsPerDay))
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)
                }
            }
        }
    }

    private func outagesCard(_ outages: [OutageDto]) -> some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Text(VigiaL10n.fmt("Caídas registradas (%ld)", outages.count))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                if outages.isEmpty {
                    Text("Sin interrupciones registradas en el período reciente.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.ok)
                } else {
                    ForEach(outages) { outage in
                        HStack(alignment: .top) {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(outage.blameLabelEs)
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(outage.blame == "router" ? VigiaColor.amber : VigiaColor.risk)
                                Text("Inicio: \(VigiaDate.relativeDescription(from: outage.start))")
                                    .font(.caption2)
                                    .foregroundStyle(VigiaColor.textSecondary)
                            }
                            Spacer()
                            Text(outage.end == nil ? "En curso" : VigiaDate.formatDuration(outage.durationSec))
                                .font(.system(.caption, design: .monospaced).weight(.semibold))
                                .foregroundStyle(outage.end == nil ? VigiaColor.risk : VigiaColor.textPrimary)
                        }
                        .padding(10)
                        .background(RoundedRectangle(cornerRadius: 10).fill(VigiaColor.surfaceElevated))
                    }
                }
            }
        }
    }

    private func speedMetric(_ title: String, value: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
                .font(.caption2)
                .foregroundStyle(VigiaColor.textSecondary)
            Text(value)
                .font(.system(.subheadline, design: .monospaced).weight(.bold))
                .foregroundStyle(color)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func legendDot(_ title: String, color: Color) -> some View {
        HStack(spacing: 4) {
            Circle().fill(color).frame(width: 7, height: 7)
            Text(title)
        }
    }

    private func statusVisuals(_ state: String) -> (title: String, symbol: String, color: Color) {
        switch state {
        case "up": return (VigiaL10n.tr("Internet operando normal"), "checkmark.circle.fill", VigiaColor.ok)
        case "degraded": return (VigiaL10n.tr("Conexión inestable o con pérdidas"), "exclamationmark.triangle.fill", VigiaColor.amber)
        case "down": return (VigiaL10n.tr("Sin conexión a internet"), "wifi.slash", VigiaColor.risk)
        default: return (VigiaL10n.tr("Estado desconocido"), "questionmark.circle", VigiaColor.textSecondary)
        }
    }
}

private struct InternetSeriesCanvas: View {
    let points: [InternetSeriesPointDto]

    var body: some View {
        Canvas { context, size in
            guard points.count >= 2 else { return }
            let padTop: CGFloat = 10
            let padBottom: CGFloat = 18
            let usableH = max(20, size.height - padTop - padBottom)
            let w = size.width

            let allMs = points.flatMap { [$0.internetMs, $0.routerMs, $0.dnsMs].compactMap { $0 } }
            let maxMs = max(50.0, (allMs.max() ?? 100.0) * 1.15)

            for frac in [0.0, 0.5, 1.0] {
                let y = padTop + usableH * CGFloat(1.0 - frac)
                var line = Path()
                line.move(to: CGPoint(x: 0, y: y))
                line.addLine(to: CGPoint(x: w, y: y))
                context.stroke(line, with: .color(VigiaColor.outline.opacity(0.6)), lineWidth: 1)
            }

            func drawSeries(_ extractor: (InternetSeriesPointDto) -> Double?, color: Color) {
                var path = Path()
                var started = false
                for (idx, pt) in points.enumerated() {
                    guard let val = extractor(pt) else {
                        started = false
                        continue
                    }
                    let x = w * CGFloat(idx) / CGFloat(max(1, points.count - 1))
                    let y = padTop + usableH * CGFloat(1.0 - min(1.0, max(0.0, val / maxMs)))
                    if !started {
                        path.move(to: CGPoint(x: x, y: y))
                        started = true
                    } else {
                        path.addLine(to: CGPoint(x: x, y: y))
                    }
                }
                context.stroke(path, with: .color(color), style: StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
            }

            drawSeries(\.routerMs, color: VigiaColor.radarCyan)
            drawSeries(\.dnsMs, color: VigiaColor.amber)
            drawSeries(\.internetMs, color: VigiaColor.ok)

            // Barras de pérdida de paquetes al pie
            for (idx, pt) in points.enumerated() {
                let loss = pt.lossPct ?? 0
                guard loss > 0 else { continue }
                let x = w * CGFloat(idx) / CGFloat(max(1, points.count - 1))
                let barH = max(3, CGFloat(min(100.0, loss) / 100.0) * usableH * 0.4)
                let rect = CGRect(x: x - 1.5, y: padTop + usableH - barH, width: 3, height: barH)
                context.fill(Path(rect), with: .color(VigiaColor.risk.opacity(0.75)))
            }
        }
        .overlay(alignment: .center) {
            if points.count < 2 {
                Text("Sin suficientes puntos para graficar todavía")
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textTertiary)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(points.count < 2
                                 ? VigiaL10n.tr("Gráfica de latencia sin suficientes muestras")
                                 : VigiaL10n.fmt("Gráfica de latencia con %ld muestras", points.count)))
    }
}
