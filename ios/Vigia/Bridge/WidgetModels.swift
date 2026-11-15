import Foundation

/// Constantes compartidas entre la app principal (`Vigia`) y la extensión de widget (`VigiaWidget`).
public enum AppGroupConstants {
    public static let appGroupID = "group.com.baxner.vigia"
    public static let widgetSummaryKey = "vigia.widget_summary.v1"
    public static let refreshTaskID = "com.baxner.vigia.alerts.refresh"
    public static let widgetKind = "VigiaWidget"

    public static var sharedDefaults: UserDefaults {
        UserDefaults(suiteName: appGroupID) ?? .standard
    }
}

/// Resumen liviano persistido en el App Group (`group.com.baxner.vigia`) para que
/// `VigiaWidget` muestre el estado actual sin abrir red ni leer Keychain desde la extensión.
public struct WidgetSummaryDto: Codable, Equatable, Sendable {
    public var linked: Bool
    public var onlineDevices: Int
    public var unreadAlerts: Int
    public var internetState: String
    public var internetLatencyMs: Int?
    public var updatedAtEpochMs: Int64?

    public init(
        linked: Bool = false,
        onlineDevices: Int = 0,
        unreadAlerts: Int = 0,
        internetState: String = "unknown",
        internetLatencyMs: Int? = nil,
        updatedAtEpochMs: Int64? = nil
    ) {
        self.linked = linked
        self.onlineDevices = onlineDevices
        self.unreadAlerts = unreadAlerts
        self.internetState = internetState
        self.internetLatencyMs = internetLatencyMs
        self.updatedAtEpochMs = updatedAtEpochMs
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        linked = try c.decodeIfPresent(Bool.self, forKey: .linked) ?? false
        onlineDevices = try c.decodeIfPresent(Int.self, forKey: .onlineDevices) ?? 0
        unreadAlerts = try c.decodeIfPresent(Int.self, forKey: .unreadAlerts) ?? 0
        internetState = try c.decodeIfPresent(String.self, forKey: .internetState) ?? "unknown"
        internetLatencyMs = try c.decodeIfPresent(Int.self, forKey: .internetLatencyMs)
        updatedAtEpochMs = try c.decodeIfPresent(Int64.self, forKey: .updatedAtEpochMs)
    }

    public static func load(from defaults: UserDefaults = AppGroupConstants.sharedDefaults) -> WidgetSummaryDto {
        guard let data = defaults.data(forKey: AppGroupConstants.widgetSummaryKey),
              let decoded = try? JSONDecoder().decode(WidgetSummaryDto.self, from: data) else {
            return WidgetSummaryDto()
        }
        return decoded
    }

    public func save(to defaults: UserDefaults = AppGroupConstants.sharedDefaults) {
        if let data = try? JSONEncoder().encode(self) {
            defaults.set(data, forKey: AppGroupConstants.widgetSummaryKey)
        }
    }
}

/// Utilidades compartidas para timestamps RFC 3339 / RFC 3339 Nano del motor Go y texto relativo en español.
public enum VigiaDate {
    private static let withFractionalLock = NSLock()

    public static func parse(_ raw: String?) -> Date? {
        guard let raw = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return nil
        }
        withFractionalLock.lock()
        defer { withFractionalLock.unlock() }
        if let d = fractionalFormatter.date(from: raw) {
            return d
        }
        return plainFormatter.date(from: raw)
    }

    public static func format(_ date: Date) -> String {
        withFractionalLock.lock()
        defer { withFractionalLock.unlock() }
        return fractionalFormatter.string(from: date)
    }

    public static func nowString() -> String {
        format(Date())
    }

    public static func relativeDescription(from raw: String?, now: Date = Date()) -> String {
        guard let date = parse(raw) else { return "—" }
        return relativeDescription(from: date, now: now)
    }

    public static func relativeDescription(from date: Date, now: Date = Date()) -> String {
        let delta = Int(now.timeIntervalSince(date))
        if delta < 45 { return VigiaL10n.tr("hace un momento") }
        if delta < 3600 {
            let mins = max(1, delta / 60)
            return VigiaL10n.fmt("hace %ld min", mins)
        }
        if delta < 86_400 {
            let hours = delta / 3600
            return hours == 1 ? VigiaL10n.tr("hace 1 h") : VigiaL10n.fmt("hace %ld h", hours)
        }
        let days = delta / 86_400
        if days < 7 {
            return days == 1 ? VigiaL10n.tr("hace 1 día") : VigiaL10n.fmt("hace %ld días", days)
        }
        let fmt = DateFormatter()
        fmt.locale = Locale.current
        fmt.dateFormat = "d MMM HH:mm"
        return fmt.string(from: date)
    }

    public static func formatDuration(_ seconds: Double) -> String {
        let total = max(0, Int(seconds.rounded()))
        if total < 60 { return VigiaL10n.fmt("%ld s", total) }
        let mins = total / 60
        let secs = total % 60
        if mins < 60 {
            return secs == 0
                ? VigiaL10n.fmt("%ld min", mins)
                : VigiaL10n.fmt("%ld min %ld s", mins, secs)
        }
        let hours = mins / 60
        let remMins = mins % 60
        return remMins == 0
            ? VigiaL10n.fmt("%ld h", hours)
            : VigiaL10n.fmt("%ld h %ld min", hours, remMins)
    }

    private static let fractionalFormatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let plainFormatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()
}

/// Decodificador/codificador JSON tolerante al contrato Go (claves omitidas y timestamps RFC3339Nano).
public enum VigiaJSON {
    public static func makeDecoder() -> JSONDecoder {
        JSONDecoder()
    }

    public static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.withoutEscapingSlashes]
        return encoder
    }
}
