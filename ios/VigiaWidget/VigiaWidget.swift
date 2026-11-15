import SwiftUI
import WidgetKit

// MARK: - Paleta local del Widget ("Radar Nocturno")

private enum WidgetTheme {
    static let background = Color(red: 0x07 / 255, green: 0x0B / 255, blue: 0x12 / 255)
    static let surface = Color(red: 0x0D / 255, green: 0x14 / 255, blue: 0x20 / 255)
    static let outline = Color(red: 0x1E / 255, green: 0x2A / 255, blue: 0x3D / 255)
    static let radarCyan = Color(red: 0x34 / 255, green: 0xE3 / 255, blue: 0xC2 / 255)
    static let electricBlue = Color(red: 0x4C / 255, green: 0x8D / 255, blue: 0xFF / 255)
    static let amber = Color(red: 0xFF / 255, green: 0xB5 / 255, blue: 0x47 / 255)
    static let risk = Color(red: 0xFF / 255, green: 0x5D / 255, blue: 0x6C / 255)
    static let ok = Color(red: 0x3D / 255, green: 0xDC / 255, blue: 0x84 / 255)
    static let textPrimary = Color(red: 0xE6 / 255, green: 0xEE / 255, blue: 0xF8 / 255)
    static let textSecondary = Color(red: 0x98 / 255, green: 0xAA / 255, blue: 0xC2 / 255)
}

public struct VigiaWidgetEntry: TimelineEntry {
    public let date: Date
    public let summary: WidgetSummaryDto

    public init(date: Date, summary: WidgetSummaryDto) {
        self.date = date
        self.summary = summary
    }
}

public struct VigiaWidgetProvider: TimelineProvider {
    public init() {}

    public func placeholder(in context: Context) -> VigiaWidgetEntry {
        VigiaWidgetEntry(
            date: Date(),
            summary: WidgetSummaryDto(
                linked: true,
                onlineDevices: 12,
                unreadAlerts: 0,
                internetState: "up",
                internetLatencyMs: 14,
                updatedAtEpochMs: Int64(Date().timeIntervalSince1970 * 1000)
            )
        )
    }

    public func getSnapshot(in context: Context, completion: @escaping (VigiaWidgetEntry) -> Void) {
        let summary = WidgetSummaryDto.load()
        completion(VigiaWidgetEntry(date: Date(), summary: summary))
    }

    public func getTimeline(in context: Context, completion: @escaping (Timeline<VigiaWidgetEntry>) -> Void) {
        let now = Date()
        let summary = WidgetSummaryDto.load()
        let entry = VigiaWidgetEntry(date: now, summary: summary)
        let nextUpdate = Calendar.current.date(byAdding: .minute, value: 15, to: now) ?? now.addingTimeInterval(900)
        completion(Timeline(entries: [entry], policy: .after(nextUpdate)))
    }
}

public struct VigiaWidgetEntryView: View {
    @Environment(\.widgetFamily) private var family
    public let entry: VigiaWidgetEntry

    public init(entry: VigiaWidgetEntry) {
        self.entry = entry
    }

    public var body: some View {
        switch family {
        case .systemSmall:
            smallView
        case .systemMedium:
            mediumView
        case .accessoryRectangular:
            accessoryRectangularView
        default:
            smallView
        }
    }

    private var smallView: some View {
        let s = entry.summary
        let netInfo = internetPresentation(s)

        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Image(systemName: "dot.radiowaves.left.and.right")
                    .foregroundStyle(WidgetTheme.radarCyan)
                Text("Vigía")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(WidgetTheme.textPrimary)
                Spacer()
                HStack(spacing: 4) {
                    Circle()
                        .fill(netInfo.color)
                        .frame(width: 7, height: 7)
                    Text(netInfo.shortValue)
                        .font(.system(.caption2, design: .monospaced).weight(.semibold))
                        .foregroundStyle(netInfo.color)
                }
            }

            Spacer(minLength: 0)

            Text("\(s.onlineDevices)")
                .font(.system(.title, design: .monospaced).weight(.bold))
                .foregroundStyle(WidgetTheme.radarCyan)
            Text(VigiaL10n.tr("online devices"))
                .font(.caption2)
                .foregroundStyle(WidgetTheme.textSecondary)

            HStack(spacing: 4) {
                if s.unreadAlerts > 0 {
                    Image(systemName: "bell.badge.fill")
                        .font(.caption2)
                        .foregroundStyle(WidgetTheme.risk)
                    Text(VigiaL10n.fmt("%ld alert(s)", s.unreadAlerts))
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(WidgetTheme.risk)
                } else {
                    Text(netInfo.label)
                        .font(.caption2.weight(.medium))
                        .foregroundStyle(netInfo.color)
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .containerBackground(for: .widget) {
            WidgetTheme.background
        }
    }

    private var mediumView: some View {
        let s = entry.summary
        let netInfo = internetPresentation(s)

        return VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label(VigiaL10n.tr("Vigía · Home network"), systemImage: "dot.radiowaves.left.and.right")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(WidgetTheme.textPrimary)
                Spacer()
                Text(s.linked ? VigiaL10n.tr("24/7 active") : VigiaL10n.tr("iPhone mode"))
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(s.linked ? WidgetTheme.ok : WidgetTheme.radarCyan)
            }

            HStack(spacing: 10) {
                metricBox(
                    title: VigiaL10n.tr("Devices"),
                    value: "\(s.onlineDevices)",
                    subtitle: VigiaL10n.tr("online"),
                    color: WidgetTheme.radarCyan
                )
                metricBox(
                    title: VigiaL10n.tr("Alerts"),
                    value: "\(s.unreadAlerts)",
                    subtitle: s.unreadAlerts == 0 ? VigiaL10n.tr("all good") : VigiaL10n.tr("unread"),
                    color: s.unreadAlerts > 0 ? WidgetTheme.risk : WidgetTheme.ok
                )
                metricBox(
                    title: VigiaL10n.tr("Internet"),
                    value: s.internetLatencyMs.map { "\($0) ms" } ?? netInfo.shortValue,
                    subtitle: netInfo.label,
                    color: netInfo.color
                )
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .containerBackground(for: .widget) {
            WidgetTheme.background
        }
    }

    private var accessoryRectangularView: some View {
        let s = entry.summary
        let netInfo = internetPresentation(s)

        return VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: "dot.radiowaves.left.and.right")
                Text("Vigía")
                    .font(.headline)
                Spacer()
                Text(netInfo.shortValue)
                    .font(.system(.caption, design: .monospaced))
            }
            Text(VigiaL10n.fmt("%ld devices · %ld alerts", s.onlineDevices, s.unreadAlerts))
                .font(.caption)
            if let lat = s.internetLatencyMs {
                Text(VigiaL10n.fmt("Internet: %@ (%ld ms)", netInfo.label, lat))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                Text(VigiaL10n.fmt("Status: %@", netInfo.label))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .containerBackground(for: .widget) {
            Color.clear
        }
    }

    private func metricBox(title: String, value: String, subtitle: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title)
                .font(.caption2)
                .foregroundStyle(WidgetTheme.textSecondary)
            Text(value)
                .font(.system(.headline, design: .monospaced).weight(.bold))
                .foregroundStyle(color)
                .lineLimit(1)
            Text(subtitle)
                .font(.caption2)
                .foregroundStyle(WidgetTheme.textSecondary)
                .lineLimit(1)
        }
        .padding(8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(WidgetTheme.surface)
                .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(WidgetTheme.outline, lineWidth: 1))
        )
    }

    private func internetPresentation(_ s: WidgetSummaryDto) -> (label: String, shortValue: String, color: Color) {
        switch s.internetState {
        case "up": return (VigiaL10n.tr("Online"), VigiaL10n.tr("OK"), WidgetTheme.ok)
        case "degraded": return (VigiaL10n.tr("Unstable"), VigiaL10n.tr("SLOW"), WidgetTheme.amber)
        case "down": return (VigiaL10n.tr("No internet"), VigiaL10n.tr("DOWN"), WidgetTheme.risk)
        default: return (s.linked ? VigiaL10n.tr("Syncing") : VigiaL10n.tr("Local"), "—", WidgetTheme.electricBlue)
        }
    }
}

@main
public struct VigiaWidgetBundle: WidgetBundle {
    public init() {}

    public var body: some Widget {
        VigiaStatusWidget()
    }
}

public struct VigiaStatusWidget: Widget {
    public init() {}

    public var body: some WidgetConfiguration {
        StaticConfiguration(kind: AppGroupConstants.widgetKind, provider: VigiaWidgetProvider()) { entry in
            VigiaWidgetEntryView(entry: entry)
        }
        .configurationDisplayName(LocalizedStringKey("widget.display_name"))
        .description(LocalizedStringKey("widget.description"))
        .supportedFamilies([.systemSmall, .systemMedium, .accessoryRectangular])
    }
}
