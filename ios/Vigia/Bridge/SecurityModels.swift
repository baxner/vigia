import Foundation

/// Nivel de riesgo de un hallazgo de seguridad (`core.Severity`).
public enum FindingSeverity: String, Codable, Comparable, Sendable {
    case high
    case medium
    case low
    case info

    public static func fromRaw(_ raw: String) -> FindingSeverity {
        FindingSeverity(rawValue: raw.lowercased()) ?? .info
    }

    private var sortOrder: Int {
        switch self {
        case .high: return 0
        case .medium: return 1
        case .low: return 2
        case .info: return 3
        }
    }

    public static func < (lhs: FindingSeverity, rhs: FindingSeverity) -> Bool {
        lhs.sortOrder < rhs.sortOrder
    }

    public var label: String {
        switch self {
        case .high: return VigiaL10n.tr("Alto")
        case .medium: return VigiaL10n.tr("Medio")
        case .low: return VigiaL10n.tr("Bajo")
        case .info: return VigiaL10n.tr("Info")
        }
    }

    public var labelEs: String { label }
}

/// Alcance exterior observado del router (`core.RouterInfo.reach`).
public enum RouterReach: String, Sendable {
    case outerRing = "outer_ring"
    case internet = "internet"
    case unknown = "unknown"

    public static func fromRaw(_ raw: String) -> RouterReach {
        RouterReach(rawValue: raw.lowercased()) ?? .unknown
    }
}

/// Informe completo de seguridad calculado por `core.BuildSecurityReport`.
public struct SecurityReport: Codable, Equatable, Sendable {
    public var generatedAt: String
    public var score: Int
    public var findings: [SecurityFinding]
    public var router: RouterInfo?
    public var outerRing: OuterRing?

    enum CodingKeys: String, CodingKey {
        case generatedAt, score, findings, router, outerRing
    }

    public init(
        generatedAt: String = VigiaDate.nowString(),
        score: Int = 100,
        findings: [SecurityFinding] = [],
        router: RouterInfo? = nil,
        outerRing: OuterRing? = nil
    ) {
        self.generatedAt = generatedAt
        self.score = score
        self.findings = findings
        self.router = router
        self.outerRing = outerRing
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        generatedAt = try c.decodeIfPresent(String.self, forKey: .generatedAt) ?? VigiaDate.nowString()
        score = try c.decodeIfPresent(Int.self, forKey: .score) ?? 100
        findings = try c.decodeIfPresent([SecurityFinding].self, forKey: .findings) ?? []
        router = try c.decodeIfPresent(RouterInfo.self, forKey: .router)
        outerRing = try c.decodeIfPresent(OuterRing.self, forKey: .outerRing)
    }
}

/// Hallazgo individual de seguridad (`core.Finding`).
public struct SecurityFinding: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String
    public var kind: String
    public var severity: String
    public var points: Int
    public var title: String
    public var detail: String
    public var action: String
    public var deviceId: Int64?
    public var ip: String?
    public var port: Int?
    public var acknowledged: Bool
    public var acknowledgedAt: String?

    enum CodingKeys: String, CodingKey {
        case id, kind, severity, points, title, detail, action, deviceId, ip, port, acknowledged, acknowledgedAt
    }

    public init(
        id: String,
        kind: String,
        severity: String,
        points: Int,
        title: String,
        detail: String,
        action: String,
        deviceId: Int64? = nil,
        ip: String? = nil,
        port: Int? = nil,
        acknowledged: Bool = false,
        acknowledgedAt: String? = nil
    ) {
        self.id = id
        self.kind = kind
        self.severity = severity
        self.points = points
        self.title = title
        self.detail = detail
        self.action = action
        self.deviceId = deviceId
        self.ip = ip
        self.port = port
        self.acknowledged = acknowledged
        self.acknowledgedAt = acknowledgedAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? ""
        severity = try c.decodeIfPresent(String.self, forKey: .severity) ?? "info"
        points = try c.decodeIfPresent(Int.self, forKey: .points) ?? 0
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        detail = try c.decodeIfPresent(String.self, forKey: .detail) ?? ""
        action = try c.decodeIfPresent(String.self, forKey: .action) ?? ""
        deviceId = try c.decodeIfPresent(Int64.self, forKey: .deviceId)
        ip = try c.decodeIfPresent(String.self, forKey: .ip)
        port = try c.decodeIfPresent(Int.self, forKey: .port)
        acknowledged = try c.decodeIfPresent(Bool.self, forKey: .acknowledged) ?? false
        acknowledgedAt = try c.decodeIfPresent(String.self, forKey: .acknowledgedAt)
    }

    public var severityLevel: FindingSeverity {
        FindingSeverity.fromRaw(severity)
    }
}

/// Reconocimiento de hallazgo ("Ya lo sé, es a propósito", `core.Acknowledgement`).
public struct AcknowledgedFinding: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { findingId }

    public var findingId: String
    public var at: String
    public var note: String?

    enum CodingKeys: String, CodingKey {
        case findingId, at, note
    }

    public init(findingId: String, at: String = VigiaDate.nowString(), note: String? = nil) {
        self.findingId = findingId
        self.at = at
        self.note = note
    }
}

/// Anillo exterior visto por el agente (red entre router y módem/ONT, `core.OuterRing`).
public struct OuterRing: Codable, Equatable, Sendable {
    public var network: String
    public var subnet: String
    public var gateway: String
    public var lastScanAt: String?
    public var devices: [SecurityDevice]

    enum CodingKeys: String, CodingKey {
        case network, subnet, gateway, lastScanAt, devices
    }

    public init(
        network: String = "",
        subnet: String = "",
        gateway: String = "",
        lastScanAt: String? = nil,
        devices: [SecurityDevice] = []
    ) {
        self.network = network
        self.subnet = subnet
        self.gateway = gateway
        self.lastScanAt = lastScanAt
        self.devices = devices
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        network = try c.decodeIfPresent(String.self, forKey: .network) ?? ""
        subnet = try c.decodeIfPresent(String.self, forKey: .subnet) ?? ""
        gateway = try c.decodeIfPresent(String.self, forKey: .gateway) ?? ""
        lastScanAt = try c.decodeIfPresent(String.self, forKey: .lastScanAt)
        devices = try c.decodeIfPresent([SecurityDevice].self, forKey: .devices) ?? []
    }
}

/// Vista reducida de equipo usada para calcular `SecurityReport` (`core.SecurityDevice`).
public struct SecurityDevice: Codable, Equatable, Sendable {
    public var id: Int64?
    public var name: String
    public var online: Bool
    public var device: Device

    public init(id: Int64? = nil, name: String = "", online: Bool = true, device: Device) {
        self.id = id
        self.name = name
        self.online = online
        self.device = device
    }
}

/// Resumen mínimo de Wi-Fi enviado al motor de seguridad (`core.WifiInfo`).
public struct WifiInfo: Codable, Equatable, Sendable {
    public var ssid: String
    /// `open`, `wep`, `wpa`, `wpa2`, `wpa3`, `wpa2-wpa3`, o `""` si no se distingue (p. ej. iOS `.personal`).
    public var security: String
    public var wps: Bool

    public init(ssid: String = "", security: String = "", wps: Bool = false) {
        self.ssid = ssid
        self.security = security
        self.wps = wps
    }
}

/// Alerta mínima usada en `SecurityInput.recentAlerts` (`core.Alert`).
public struct SecurityAlertRef: Codable, Equatable, Sendable {
    public var type: String
    public var at: String

    public init(type: String, at: String) {
        self.type = type
        self.at = at
    }
}

/// Entrada serializada enviada a `MobileSecurityReport` (`core.SecurityInput`).
public struct SecurityInput: Codable, Equatable, Sendable {
    public var devices: [SecurityDevice]
    public var router: RouterInfo?
    public var outerRing: OuterRing?
    public var wifi: WifiInfo?
    public var recentAlerts: [SecurityAlertRef]
    public var acknowledged: [AcknowledgedFinding]

    public init(
        devices: [SecurityDevice] = [],
        router: RouterInfo? = nil,
        outerRing: OuterRing? = nil,
        wifi: WifiInfo? = nil,
        recentAlerts: [SecurityAlertRef] = [],
        acknowledged: [AcknowledgedFinding] = []
    ) {
        self.devices = devices
        self.router = router
        self.outerRing = outerRing
        self.wifi = wifi
        self.recentAlerts = recentAlerts
        self.acknowledged = acknowledged
    }
}
