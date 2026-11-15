import SwiftUI
import UIKit

// MARK: - Paleta "Radar Nocturno"

public enum VigiaColor {
    public static let background = Color(red: 0x07 / 255, green: 0x0B / 255, blue: 0x12 / 255)
    public static let surface = Color(red: 0x0D / 255, green: 0x14 / 255, blue: 0x20 / 255)
    public static let surfaceElevated = Color(red: 0x12 / 255, green: 0x1B / 255, blue: 0x2B / 255)
    public static let outline = Color(red: 0x1E / 255, green: 0x2A / 255, blue: 0x3D / 255)

    public static let radarCyan = Color(red: 0x34 / 255, green: 0xE3 / 255, blue: 0xC2 / 255)
    public static let electricBlue = Color(red: 0x4C / 255, green: 0x8D / 255, blue: 0xFF / 255)
    public static let amber = Color(red: 0xFF / 255, green: 0xB5 / 255, blue: 0x47 / 255)
    public static let risk = Color(red: 0xFF / 255, green: 0x5D / 255, blue: 0x6C / 255)
    public static let ok = Color(red: 0x3D / 255, green: 0xDC / 255, blue: 0x84 / 255)

    public static let textPrimary = Color(red: 0xE6 / 255, green: 0xEE / 255, blue: 0xF8 / 255)
    public static let textSecondary = Color(red: 0x98 / 255, green: 0xAA / 255, blue: 0xC2 / 255)
    public static let textTertiary = Color(red: 0x7A / 255, green: 0x8C / 255, blue: 0xA6 / 255)
}

// MARK: - Superficies y componentes reutilizables

public struct VigiaGlassCard<Content: View>: View {
    private let highlighted: Bool
    private let glowColor: Color
    private let padding: CGFloat
    private let content: Content

    public init(
        highlighted: Bool = false,
        glowColor: Color = VigiaColor.radarCyan,
        padding: CGFloat = 16,
        @ViewBuilder content: () -> Content
    ) {
        self.highlighted = highlighted
        self.glowColor = glowColor
        self.padding = padding
        self.content = content()
    }

    public var body: some View {
        content
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: 20, style: .continuous)
                    .fill(highlighted ? VigiaColor.surfaceElevated : VigiaColor.surface)
                    .overlay(
                        RoundedRectangle(cornerRadius: 20, style: .continuous)
                            .strokeBorder(
                                highlighted ? glowColor.opacity(0.35) : VigiaColor.outline,
                                lineWidth: 1
                            )
                    )
                    .shadow(
                        color: highlighted ? glowColor.opacity(0.14) : .clear,
                        radius: 16,
                        x: 0,
                        y: 4
                    )
            )
    }
}

public struct VigiaStatusChip: View {
    public let title: String
    public let color: Color
    public var icon: String?

    public init(_ title: String, color: Color, icon: String? = nil) {
        self.title = title
        self.color = color
        self.icon = icon
    }

    public var body: some View {
        HStack(spacing: 5) {
            if let icon {
                Image(systemName: icon)
                    .font(.caption2.weight(.semibold))
            }
            Text(title)
                .font(.caption.weight(.semibold))
                .lineLimit(1)
        }
        .foregroundStyle(color)
        .padding(.horizontal, 10)
        .padding(.vertical, 4)
        .background(
            Capsule()
                .fill(color.opacity(0.14))
                .overlay(Capsule().strokeBorder(color.opacity(0.32), lineWidth: 1))
        )
    }
}

public struct VigiaConfidenceRing: View {
    public let value: Int
    public var size: CGFloat = 46
    public var color: Color = VigiaColor.radarCyan

    public var body: some View {
        let clamped = max(0, min(100, value))
        ZStack {
            Circle()
                .stroke(VigiaColor.outline, lineWidth: 4)
            Circle()
                .trim(from: 0, to: CGFloat(clamped) / 100.0)
                .stroke(color, style: StrokeStyle(lineWidth: 4, lineCap: .round))
                .rotationEffect(.degrees(-90))
            Text("\(clamped)")
                .font(.system(.caption, design: .monospaced).weight(.bold))
                .foregroundStyle(VigiaColor.textPrimary)
                .minimumScaleFactor(0.6)
        }
        .frame(width: size, height: size)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Puntuación"))
        .accessibilityValue(Text(VigiaL10n.fmt("%ld de 100", clamped)))
    }
}

public struct VigiaSignalBars: View {
    public let rssi: Int
    public var activeColor: Color = VigiaColor.radarCyan

    private var activeBars: Int {
        switch rssi {
        case (-55)...: return 4
        case -68 ..< -55: return 3
        case -82 ..< -68: return 2
        case -95 ..< -82: return 1
        default: return 0
        }
    }

    public var body: some View {
        HStack(alignment: .bottom, spacing: 2.5) {
            ForEach(0..<4, id: \.self) { idx in
                RoundedRectangle(cornerRadius: 1.5)
                    .fill(idx < activeBars ? activeColor : VigiaColor.outline)
                    .frame(width: 4, height: CGFloat(6 + idx * 4))
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(VigiaL10n.fmt("Señal %ld dBm (%ld de 4 barras)", rssi, activeBars)))
    }
}

public struct VigiaEmptyState: View {
    public let symbol: String
    public let title: String
    public let message: String
    public var actionTitle: String?
    public var action: (() -> Void)?

    public init(
        symbol: String,
        title: String,
        message: String,
        actionTitle: String? = nil,
        action: (() -> Void)? = nil
    ) {
        self.symbol = symbol
        self.title = title
        self.message = message
        self.actionTitle = actionTitle
        self.action = action
    }

    public var body: some View {
        VigiaGlassCard {
            VStack(spacing: 12) {
                Image(systemName: symbol)
                    .font(.system(size: 36, weight: .medium))
                    .foregroundStyle(VigiaColor.radarCyan)
                Text(title)
                    .font(.headline)
                    .foregroundStyle(VigiaColor.textPrimary)
                    .multilineTextAlignment(.center)
                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(VigiaColor.textSecondary)
                    .multilineTextAlignment(.center)
                if let actionTitle, let action {
                    Button(actionTitle, action: action)
                        .buttonStyle(.borderedProminent)
                        .tint(VigiaColor.radarCyan)
                        .foregroundStyle(VigiaColor.background)
                        .padding(.top, 4)
                }
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 8)
        }
    }
}

/// Evita que la pantalla se bloquee durante el radar de búsqueda BLE ("Vuelta guiada").
public struct KeepScreenOnModifier: ViewModifier {
    public func body(content: Content) -> some View {
        content
            .onAppear { UIApplication.shared.isIdleTimerDisabled = true }
            .onDisappear { UIApplication.shared.isIdleTimerDisabled = false }
    }
}

public extension View {
    func keepScreenOnWhileVisible() -> some View {
        modifier(KeepScreenOnModifier())
    }

    /// Asegura área táctil mínima de 44x44 pt según Apple Human Interface Guidelines.
    func vigiaMinTouchTarget() -> some View {
        frame(minWidth: 44, minHeight: 44)
            .contentShape(Rectangle())
    }
}

// MARK: - Helpers de dominio compartidos en UI

public extension KnownDevice {
    var effectiveType: DeviceType {
        if let rawCustom = custom.typeId, !rawCustom.trimmingCharacters(in: .whitespaces).isEmpty {
            return DeviceType.fromRaw(rawCustom)
        }
        return DeviceType.fromRaw(last.identity.type)
    }

    var statusColor: Color {
        if !online {
            return VigiaColor.textTertiary
        }
        let risky = VigiaEngineBridge.shared.riskyPorts(among: last.openPorts)
        if !risky.isEmpty {
            return VigiaColor.risk
        }
        if effectiveType == .unknown {
            return VigiaColor.amber
        }
        return VigiaColor.radarCyan
    }
}

public extension Device {
    var effectiveType: DeviceType {
        DeviceType.fromRaw(identity.type)
    }

    var statusColor: Color {
        let risky = VigiaEngineBridge.shared.riskyPorts(among: openPorts)
        if !risky.isEmpty {
            return VigiaColor.risk
        }
        if effectiveType == .unknown {
            return VigiaColor.amber
        }
        return VigiaColor.radarCyan
    }
}

public enum VigiaRadarLayout {
    public static func angles(for devices: [KnownDevice]) -> [Int64: Double] {
        var result: [Int64: Double] = [:]
        let grouped = Dictionary(grouping: devices) { $0.effectiveType.radarRing }
        for (ring, items) in grouped {
            let sorted = items.sorted { ipSortKey($0.lastIp) < ipSortKey($1.lastIp) }
            let step = 360.0 / Double(max(1, sorted.count))
            let offset = (Double(ring) * 137.5).truncatingRemainder(dividingBy: 360.0)
            for (index, item) in sorted.enumerated() {
                result[item.id] = (offset + Double(index) * step).truncatingRemainder(dividingBy: 360.0)
            }
        }
        return result
    }

    public static func ipSortKey(_ ip: String) -> UInt64 {
        ip.split(separator: ".").reduce(0) { acc, part in
            acc * 256 + (UInt64(part) ?? 0)
        }
    }

    public static func sourceLabel(_ source: String) -> String {
        switch source {
        case "ping": return VigiaL10n.tr("TCP")
        case "icmp": return VigiaL10n.tr("ping")
        case "arp": return VigiaL10n.tr("tabla ARP")
        case "ndp": return VigiaL10n.tr("vecinos IPv6")
        case "mdns": return VigiaL10n.tr("Bonjour")
        case "ssdp": return VigiaL10n.tr("UPnP")
        case "netbios": return VigiaL10n.tr("NetBIOS")
        case "tuya": return VigiaL10n.tr("Tuya")
        case "api": return VigiaL10n.tr("API del equipo")
        case "gateway": return VigiaL10n.tr("Gateway")
        case "tcp": return VigiaL10n.tr("sondeo TCP")
        case "self": return VigiaL10n.tr("este iPhone")
        default: return source
        }
    }

    public static func sourceLabelEs(_ source: String) -> String {
        sourceLabel(source)
    }
}
