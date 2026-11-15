import CoreLocation
import SwiftUI

public struct HuntView: View {
    @ObservedObject private var bleEngine: RadarBleEngine
    public let peripheralId: UUID
    public let fallbackTitle: String

    public init(bleEngine: RadarBleEngine, peripheralId: UUID, fallbackTitle: String) {
        self.bleEngine = bleEngine
        self.peripheralId = peripheralId
        self.fallbackTitle = fallbackTitle
    }

    public var body: some View {
        let reading = bleEngine.huntedReading
        let rssi = reading?.smoothedRssi ?? -95.0
        let fraction = DistanceModel.proximityFraction(rssi: rssi)
        let pct = Int((fraction * 100).rounded())
        let txPower = Double(reading?.txPower ?? -59)

        ScrollView {
            VStack(spacing: 14) {
                hotColdCard(reading: reading, rssi: rssi, pct: pct, txPower: txPower)
                geigerCard
                guidedTurnCard
            }
            .padding(16)
        }
        .background(VigiaColor.background.ignoresSafeArea())
        .navigationTitle(reading?.displayName ?? fallbackTitle)
        .navigationBarTitleDisplayMode(.inline)
        .keepScreenOnWhileVisible()
        .onAppear {
            bleEngine.startHunt(peripheralId: peripheralId)
        }
        .onDisappear {
            bleEngine.stopHunt()
        }
    }

    private func hotColdCard(reading: BlePeripheralReading?, rssi: Double, pct: Int, txPower: Double) -> some View {
        let color: Color = pct >= 70 ? VigiaColor.radarCyan : (pct >= 40 ? VigiaColor.electricBlue : VigiaColor.amber)
        let stale = reading.map { Date().timeIntervalSince($0.lastSeen) > 5.0 } ?? true

        return VigiaGlassCard(highlighted: !stale, glowColor: color) {
            VStack(spacing: 12) {
                HStack {
                    VStack(alignment: .leading, spacing: 3) {
                        Text(stale ? VigiaL10n.tr("Waiting for signal…") : DistanceModel.zoneLabel(rssi: rssi, txPowerAt1m: txPower))
                            .font(.headline)
                            .foregroundStyle(VigiaColor.textPrimary)
                        Text(reading.map { VigiaL10n.fmt("Estimated distance ~%.1f m", $0.estimatedMeters) } ?? VigiaL10n.tr("Move around the room"))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    Spacer()
                    trendBadge(bleEngine.huntedTrend)
                }

                ProgressView(value: Double(pct), total: 100)
                    .tint(color)
                    .accessibilityLabel(VigiaL10n.tr("Signal proximity"))
                    .accessibilityValue("\(pct)%")

                HStack {
                    Text("\(Int(rssi.rounded())) dBm")
                        .font(.system(.title2, design: .monospaced).weight(.bold))
                        .foregroundStyle(color)
                    Spacer()
                    Text(VigiaL10n.fmt("Signal %ld%%", pct))
                        .font(.system(.subheadline, design: .monospaced).weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                }

                if let lanName = reading?.matchedLanDeviceName, let lanIp = reading?.matchedLanDeviceIp {
                    HStack {
                        Image(systemName: "link")
                            .foregroundStyle(VigiaColor.electricBlue)
                            .accessibilityHidden(true)
                        Text(VigiaL10n.fmt("Possible LAN match: %@ (%@)", lanName, lanIp))
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                        Spacer()
                    }
                }
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var geigerCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Label("Detector Geiger", systemImage: "waveform.path.ecg")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)

                HStack(spacing: 12) {
                    Toggle(isOn: $bleEngine.geigerHapticEnabled) {
                        Label("Vibración", systemImage: "iphone.radiowaves.left.and.right")
                            .font(.caption.weight(.semibold))
                    }
                    .toggleStyle(.button)
                    .tint(VigiaColor.radarCyan)

                    Toggle(isOn: $bleEngine.geigerSoundEnabled) {
                        Label("Sonido", systemImage: "speaker.wave.2.fill")
                            .font(.caption.weight(.semibold))
                    }
                    .toggleStyle(.button)
                    .tint(VigiaColor.electricBlue)

                    Spacer()
                }

                Text("El pulso acelera conforme aumenta la intensidad de señal suavizada (Kalman).")
                    .font(.caption2)
                    .foregroundStyle(VigiaColor.textTertiary)
            }
        }
    }

    private var guidedTurnCard: some View {
        let est = bleEngine.bearingEstimate
        let heading = bleEngine.currentHeadingDeg

        let headingAvailable = CLLocationManager.headingAvailable()
        return VigiaGlassCard {
            VStack(spacing: 12) {
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Vuelta guiada (8 sectores)")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(VigiaColor.textPrimary)
                        Text(guidanceInstruction(est: est, heading: heading, headingAvailable: headingAvailable))
                            .font(.caption)
                            .foregroundStyle(est.bearingDegrees != nil ? VigiaColor.radarCyan : VigiaColor.textSecondary)
                    }
                    Spacer()
                    Button("Reiniciar") {
                        bleEngine.resetGuidedTurn()
                    }
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(VigiaColor.electricBlue)
                    .vigiaMinTouchTarget()
                    .accessibilityLabel(VigiaL10n.tr("Reset guided turn"))
                }

                GuidedCompassCanvas(
                    headingDeg: heading,
                    estimate: est
                )
                .frame(width: 230, height: 230)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(VigiaL10n.tr("Guided turn compass"))
                .accessibilityValue(guidanceInstruction(est: est, heading: heading, headingAvailable: headingAvailable))

                HStack {
                    Text(VigiaL10n.fmt("Compass: %ld°", Int(heading.rounded())))
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(VigiaColor.textSecondary)
                    Spacer()
                    Text(VigiaL10n.fmt("Sectors: %ld/8", est.coveredCount))
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(VigiaColor.radarCyan)
                    if est.bearingDegrees != nil {
                        Text(VigiaL10n.fmt("· Conf. %ld%%", Int((est.confidence * 100).rounded())))
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                }
            }
        }
    }

    private func guidanceInstruction(est: BearingEstimate, heading: Double, headingAvailable: Bool = true) -> String {
        if !headingAvailable {
            return VigiaL10n.tr("Compass unavailable on this device; use the hot/cold meter above.")
        }
        guard let bearing = est.bearingDegrees else {
            return VigiaL10n.tr("Turn slowly 360° holding iPhone in front of you to sample all 8 sectors.")
        }
        let diff = CosineBearingEstimator.shortestAngleDiff(from: heading, to: bearing)
        if abs(diff) <= 18 {
            return VigiaL10n.fmt("Straight ahead (%ld°)", Int(bearing.rounded()))
        } else if diff > 0 {
            return VigiaL10n.fmt("Turn right ~%ld°", Int(abs(diff).rounded()))
        } else {
            return VigiaL10n.fmt("Turn left ~%ld°", Int(abs(diff).rounded()))
        }
    }

    private func trendBadge(_ trend: ProximityTrend) -> some View {
        let color: Color = {
            switch trend {
            case .approaching: return VigiaColor.ok
            case .receding: return VigiaColor.amber
            case .steady: return VigiaColor.textSecondary
            }
        }()
        let icon: String = {
            switch trend {
            case .approaching: return "arrow.up.right"
            case .receding: return "arrow.down.right"
            case .steady: return "equal"
            }
        }()
        return VigiaStatusChip(trend.label, color: color, icon: icon)
    }
}

private struct GuidedCompassCanvas: View {
    let headingDeg: Double
    let estimate: BearingEstimate

    var body: some View {
        Canvas { context, size in
            let center = CGPoint(x: size.width / 2, y: size.height / 2)
            let radius = min(size.width, size.height) / 2 - 12

            // Anillo exterior y los 8 sectores de 45 grados relativos al norte magnético.
            for sector in 0..<8 {
                let startWorld = Double(sector) * 45.0 - 22.5
                let endWorld = startWorld + 45.0
                let relStart = Angle.degrees(startWorld - headingDeg - 90)
                let relEnd = Angle.degrees(endWorld - headingDeg - 90)

                var arc = Path()
                arc.addArc(
                    center: center,
                    radius: radius,
                    startAngle: relStart,
                    endAngle: relEnd,
                    clockwise: false
                )

                let covered = sector < estimate.coveredSectors.count && estimate.coveredSectors[sector]
                let isTarget = estimate.targetSector == sector
                let color: Color = isTarget ? VigiaColor.radarCyan : (covered ? VigiaColor.electricBlue.opacity(0.85) : VigiaColor.outline)
                context.stroke(arc, with: .color(color), style: StrokeStyle(lineWidth: isTarget ? 10 : 6, lineCap: .butt))
            }

            // Círculo guía interno
            let innerR = radius * 0.68
            context.stroke(
                Path(ellipseIn: CGRect(x: center.x - innerR, y: center.y - innerR, width: innerR * 2, height: innerR * 2)),
                with: .color(VigiaColor.outline),
                lineWidth: 1
            )

            // Marca fija superior (hacia donde apunta el iPhone)
            var topTick = Path()
            topTick.move(to: CGPoint(x: center.x, y: center.y - radius - 8))
            topTick.addLine(to: CGPoint(x: center.x - 6, y: center.y - radius + 2))
            topTick.addLine(to: CGPoint(x: center.x + 6, y: center.y - radius + 2))
            topTick.closeSubpath()
            context.fill(topTick, with: .color(VigiaColor.textPrimary))

            // Flecha hacia el rumbo estimado por el ajuste de coseno
            if let bearing = estimate.bearingDegrees {
                let relRad = (bearing - headingDeg - 90.0) * Double.pi / 180.0
                let tip = CGPoint(
                    x: center.x + innerR * 0.92 * cos(relRad),
                    y: center.y + innerR * 0.92 * sin(relRad)
                )
                var line = Path()
                line.move(to: center)
                line.addLine(to: tip)
                context.stroke(line, with: .color(VigiaColor.radarCyan), style: StrokeStyle(lineWidth: 4, lineCap: .round))
                context.fill(
                    Path(ellipseIn: CGRect(x: tip.x - 6, y: tip.y - 6, width: 12, height: 12)),
                    with: .color(VigiaColor.radarCyan)
                )
            }

            // Centro
            context.fill(
                Path(ellipseIn: CGRect(x: center.x - 5, y: center.y - 5, width: 10, height: 10)),
                with: .color(VigiaColor.textPrimary)
            )
        }
    }
}
