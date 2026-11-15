import Foundation

/// Vínculo persistido en Keychain hacia un Agente Vigía 24/7 emparejado.
public struct AgentLink: Codable, Equatable, Sendable, CustomStringConvertible {
    public var agentId: String
    public var agentName: String
    public var host: String
    public var port: Int
    public var token: String
    public var certSha256: String

    public init(
        agentId: String,
        agentName: String,
        host: String,
        port: Int,
        token: String,
        certSha256: String
    ) {
        self.agentId = agentId
        self.agentName = agentName
        self.host = host
        self.port = port
        self.token = token
        self.certSha256 = certSha256
    }

    public var baseUrl: String {
        "https://\(host):\(port)"
    }

    public var normalizedCertPin: String {
        certSha256.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    /// Nunca expone el `token` en logs ni diagnósticos.
    public var description: String {
        "AgentLink(id=\(agentId), name=\(agentName), host=\(host):\(port), pin=\(normalizedCertPin.prefix(12))…)"
    }
}

/// Respuesta de `GET /api/v1/info`.
public struct AgentInfoDto: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var version: String
    public var scanIntervalSec: Int
    public var lastScanAt: String?
    public var warning: String?
    public var addresses: [String]
    public var certSha256: String

    enum CodingKeys: String, CodingKey {
        case id, name, version, scanIntervalSec, lastScanAt, warning, addresses, certSha256
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? ""
        scanIntervalSec = try c.decodeIfPresent(Int.self, forKey: .scanIntervalSec) ?? 0
        lastScanAt = try c.decodeIfPresent(String.self, forKey: .lastScanAt)
        warning = try c.decodeIfPresent(String.self, forKey: .warning)
        addresses = try c.decodeIfPresent([String].self, forKey: .addresses) ?? []
        certSha256 = try c.decodeIfPresent(String.self, forKey: .certSha256) ?? ""
    }
}

/// Red conocida por el agente (`GET /api/v1/networks`).
public struct AgentNetworkDto: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { key }

    public var key: String
    public var subnet: String
    public var gateway: String
    public var gatewayMac: String?
    public var firstSeen: String?
    public var lastSeen: String?
    public var lastScanAt: String?
    public var role: String

    enum CodingKeys: String, CodingKey {
        case key, subnet, gateway, gatewayMac, firstSeen, lastSeen, lastScanAt, role
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        key = try c.decode(String.self, forKey: .key)
        subnet = try c.decodeIfPresent(String.self, forKey: .subnet) ?? ""
        gateway = try c.decodeIfPresent(String.self, forKey: .gateway) ?? ""
        gatewayMac = try c.decodeIfPresent(String.self, forKey: .gatewayMac)
        firstSeen = try c.decodeIfPresent(String.self, forKey: .firstSeen)
        lastSeen = try c.decodeIfPresent(String.self, forKey: .lastSeen)
        lastScanAt = try c.decodeIfPresent(String.self, forKey: .lastScanAt)
        role = try c.decodeIfPresent(String.self, forKey: .role) ?? "inner"
    }
}

public struct PairRequest: Codable, Sendable {
    public var code: String
    public var client: String

    public init(code: String, client: String) {
        self.code = code
        self.client = client
    }
}

public struct PairResponseDto: Codable, Equatable, Sendable {
    public var token: String
    public var agentId: String
    public var agentName: String
    public var certSha256: String

    enum CodingKeys: String, CodingKey {
        case token, agentId, agentName, certSha256
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        token = try c.decode(String.self, forKey: .token)
        agentId = try c.decode(String.self, forKey: .agentId)
        agentName = try c.decode(String.self, forKey: .agentName)
        certSha256 = try c.decodeIfPresent(String.self, forKey: .certSha256) ?? ""
    }
}

public struct ApiErrorBody: Codable, Sendable {
    public var error: String
}

public struct AcknowledgeFindingRequest: Codable, Sendable {
    public var note: String?

    public init(note: String? = nil) {
        self.note = note
    }
}

public struct ScanStartedDto: Codable, Sendable {
    public var started: Bool
}

public struct AlertsReadRequest: Codable, Sendable {
    public var upTo: Int64

    public init(upTo: Int64) {
        self.upTo = upTo
    }
}

/// Tipos de alerta configurables en `GET/PUT /api/v1/alert-settings`.
public enum AlertType: String, CaseIterable, Codable, Identifiable, Sendable {
    case newDevice = "NEW_DEVICE"
    case importantOffline = "IMPORTANT_OFFLINE"
    case importantOnline = "IMPORTANT_ONLINE"
    case agentBlind = "AGENT_BLIND"
    case agentRecovered = "AGENT_RECOVERED"
    case routerChanged = "ROUTER_CHANGED"
    case arpConflict = "ARP_CONFLICT"
    case portMappingAdded = "PORT_MAPPING_ADDED"
    case riskyService = "RISKY_SERVICE"
    case internetDown = "INTERNET_DOWN"
    case internetRestored = "INTERNET_RESTORED"
    case test = "TEST"

    public var id: String { rawValue }

    /// Los 11 tipos reales que el usuario puede activar/desactivar (excluye `TEST`).
    public static var configurableCases: [AlertType] {
        allCases.filter { $0 != .test }
    }

    public var label: String {
        switch self {
        case .newDevice: return VigiaL10n.tr("Equipo nuevo en la red")
        case .importantOffline: return VigiaL10n.tr("Equipo importante desconectado")
        case .importantOnline: return VigiaL10n.tr("Equipo importante conectado")
        case .agentBlind: return VigiaL10n.tr("Agente sin visibilidad de la red")
        case .agentRecovered: return VigiaL10n.tr("Agente recuperó visibilidad")
        case .routerChanged: return VigiaL10n.tr("Cambio de router o gateway")
        case .arpConflict: return VigiaL10n.tr("Posible conflicto o suplantación ARP")
        case .portMappingAdded: return VigiaL10n.tr("Puerto expuesto en el router (UPnP)")
        case .riskyService: return VigiaL10n.tr("Servicio de riesgo detectado")
        case .internetDown: return VigiaL10n.tr("Caída de internet")
        case .internetRestored: return VigiaL10n.tr("Internet restablecido")
        case .test: return VigiaL10n.tr("Prueba de notificación")
        }
    }

    public var labelEs: String { label }
}

public enum AlertSeverity: String, Sendable {
    case critical
    case warning
    case info

    public static func fromRaw(_ raw: String) -> AlertSeverity {
        AlertSeverity(rawValue: raw.lowercased()) ?? .info
    }
}

/// Alerta generada por el Agente 24/7 (`GET /api/v1/alerts`).
public struct AlertDto: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: Int64
    public var type: String
    public var severity: String
    public var at: String
    public var title: String
    public var body: String
    public var deviceId: Int64?
    public var network: String?
    public var read: Bool

    enum CodingKeys: String, CodingKey {
        case id, type, severity, at, title, body, deviceId, network, read
    }

    public init(
        id: Int64,
        type: String,
        severity: String,
        at: String,
        title: String,
        body: String,
        deviceId: Int64? = nil,
        network: String? = nil,
        read: Bool = false
    ) {
        self.id = id
        self.type = type
        self.severity = severity
        self.at = at
        self.title = title
        self.body = body
        self.deviceId = deviceId
        self.network = network
        self.read = read
    }

    public var severityLevel: AlertSeverity {
        AlertSeverity.fromRaw(severity)
    }
}

/// Preferencias por tipo de alerta (`GET/PUT /api/v1/alert-settings`).
public struct AlertSettingsDto: Codable, Equatable, Sendable {
    public var newDevice: Bool
    public var importantOffline: Bool
    public var importantOnline: Bool
    public var agentBlind: Bool
    public var agentRecovered: Bool
    public var routerChanged: Bool
    public var arpConflict: Bool
    public var portMappingAdded: Bool
    public var riskyService: Bool
    public var internetDown: Bool
    public var internetRestored: Bool

    enum CodingKeys: String, CodingKey {
        case newDevice = "NEW_DEVICE"
        case importantOffline = "IMPORTANT_OFFLINE"
        case importantOnline = "IMPORTANT_ONLINE"
        case agentBlind = "AGENT_BLIND"
        case agentRecovered = "AGENT_RECOVERED"
        case routerChanged = "ROUTER_CHANGED"
        case arpConflict = "ARP_CONFLICT"
        case portMappingAdded = "PORT_MAPPING_ADDED"
        case riskyService = "RISKY_SERVICE"
        case internetDown = "INTERNET_DOWN"
        case internetRestored = "INTERNET_RESTORED"
    }

    public init(
        newDevice: Bool = true,
        importantOffline: Bool = true,
        importantOnline: Bool = false,
        agentBlind: Bool = true,
        agentRecovered: Bool = true,
        routerChanged: Bool = true,
        arpConflict: Bool = true,
        portMappingAdded: Bool = true,
        riskyService: Bool = true,
        internetDown: Bool = true,
        internetRestored: Bool = true
    ) {
        self.newDevice = newDevice
        self.importantOffline = importantOffline
        self.importantOnline = importantOnline
        self.agentBlind = agentBlind
        self.agentRecovered = agentRecovered
        self.routerChanged = routerChanged
        self.arpConflict = arpConflict
        self.portMappingAdded = portMappingAdded
        self.riskyService = riskyService
        self.internetDown = internetDown
        self.internetRestored = internetRestored
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        newDevice = try c.decodeIfPresent(Bool.self, forKey: .newDevice) ?? true
        importantOffline = try c.decodeIfPresent(Bool.self, forKey: .importantOffline) ?? true
        importantOnline = try c.decodeIfPresent(Bool.self, forKey: .importantOnline) ?? false
        agentBlind = try c.decodeIfPresent(Bool.self, forKey: .agentBlind) ?? true
        agentRecovered = try c.decodeIfPresent(Bool.self, forKey: .agentRecovered) ?? true
        routerChanged = try c.decodeIfPresent(Bool.self, forKey: .routerChanged) ?? true
        arpConflict = try c.decodeIfPresent(Bool.self, forKey: .arpConflict) ?? true
        portMappingAdded = try c.decodeIfPresent(Bool.self, forKey: .portMappingAdded) ?? true
        riskyService = try c.decodeIfPresent(Bool.self, forKey: .riskyService) ?? true
        internetDown = try c.decodeIfPresent(Bool.self, forKey: .internetDown) ?? true
        internetRestored = try c.decodeIfPresent(Bool.self, forKey: .internetRestored) ?? true
    }

    public func isEnabled(_ type: AlertType) -> Bool {
        switch type {
        case .newDevice: return newDevice
        case .importantOffline: return importantOffline
        case .importantOnline: return importantOnline
        case .agentBlind: return agentBlind
        case .agentRecovered: return agentRecovered
        case .routerChanged: return routerChanged
        case .arpConflict: return arpConflict
        case .portMappingAdded: return portMappingAdded
        case .riskyService: return riskyService
        case .internetDown: return internetDown
        case .internetRestored: return internetRestored
        case .test: return true
        }
    }

    public mutating func setEnabled(_ enabled: Bool, for type: AlertType) {
        switch type {
        case .newDevice: newDevice = enabled
        case .importantOffline: importantOffline = enabled
        case .importantOnline: importantOnline = enabled
        case .agentBlind: agentBlind = enabled
        case .agentRecovered: agentRecovered = enabled
        case .routerChanged: routerChanged = enabled
        case .arpConflict: arpConflict = enabled
        case .portMappingAdded: portMappingAdded = enabled
        case .riskyService: riskyService = enabled
        case .internetDown: internetDown = enabled
        case .internetRestored: internetRestored = enabled
        case .test: break
        }
    }
}

/// Webhook registrado en el agente (`GET/POST /api/v1/webhooks`).
public struct WebhookDto: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: Int64
    public var url: String
    public var types: [String]
    public var createdAt: String?
    public var lastStatus: Int?
    public var lastError: String?

    enum CodingKeys: String, CodingKey {
        case id, url, types, createdAt, lastStatus, lastError
    }

    public init(
        id: Int64,
        url: String,
        types: [String] = [],
        createdAt: String? = nil,
        lastStatus: Int? = nil,
        lastError: String? = nil
    ) {
        self.id = id
        self.url = url
        self.types = types
        self.createdAt = createdAt
        self.lastStatus = lastStatus
        self.lastError = lastError
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int64.self, forKey: .id)
        url = try c.decode(String.self, forKey: .url)
        types = try c.decodeIfPresent([String].self, forKey: .types) ?? []
        createdAt = try c.decodeIfPresent(String.self, forKey: .createdAt)
        lastStatus = try c.decodeIfPresent(Int.self, forKey: .lastStatus)
        lastError = try c.decodeIfPresent(String.self, forKey: .lastError)
    }
}

public struct AddWebhookRequest: Codable, Sendable {
    public var url: String
    public var types: [String]
    public var secret: String?

    public init(url: String, types: [String] = [], secret: String? = nil) {
        self.url = url
        self.types = types
        self.secret = secret
    }
}

public struct WebhookTestResultDto: Codable, Equatable, Sendable {
    public var ok: Bool
    public var status: Int?
    public var error: String?

    enum CodingKeys: String, CodingKey {
        case ok, status, error
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ok = try c.decodeIfPresent(Bool.self, forKey: .ok) ?? false
        status = try c.decodeIfPresent(Int.self, forKey: .status)
        error = try c.decodeIfPresent(String.self, forKey: .error)
    }
}

/// Parche explícito para `PATCH /api/v1/devices/{id}`: permite enviar `null` explícito al borrar un campo.
public struct DeviceCustomizationPatch: Sendable {
    public enum Field<T: Sendable>: Sendable {
        case absent
        case present(T?)
    }

    public var name: Field<String> = .absent
    public var typeId: Field<String> = .absent
    public var notes: Field<String> = .absent
    public var location: Field<String> = .absent
    public var important: Field<Bool> = .absent

    public init() {}

    public static func replaceAll(_ custom: DeviceCustomization) -> DeviceCustomizationPatch {
        var patch = DeviceCustomizationPatch()
        patch.name = .present(custom.name)
        patch.typeId = .present(custom.typeId)
        patch.notes = .present(custom.notes)
        patch.location = .present(custom.location)
        patch.important = .present(custom.important)
        return patch
    }

    public func jsonData() throws -> Data {
        var dict: [String: Any] = [:]
        switch name {
        case .absent: break
        case .present(let value): dict["name"] = value as Any? ?? NSNull()
        }
        switch typeId {
        case .absent: break
        case .present(let value): dict["typeId"] = value as Any? ?? NSNull()
        }
        switch notes {
        case .absent: break
        case .present(let value): dict["notes"] = value as Any? ?? NSNull()
        }
        switch location {
        case .absent: break
        case .present(let value): dict["location"] = value as Any? ?? NSNull()
        }
        switch important {
        case .absent: break
        case .present(let value): dict["important"] = value ?? false
        }
        return try JSONSerialization.data(withJSONObject: dict)
    }
}

// MARK: - Monitor de Internet (v1.4+)

public struct InternetNowDto: Codable, Equatable, Sendable {
    public var routerMs: Double?
    public var modemMs: Double?
    public var internetMs: Double?
    public var dnsMs: Double?
    public var lossPct: Double

    enum CodingKeys: String, CodingKey {
        case routerMs, modemMs, internetMs, dnsMs, lossPct
    }

    public init(
        routerMs: Double? = nil,
        modemMs: Double? = nil,
        internetMs: Double? = nil,
        dnsMs: Double? = nil,
        lossPct: Double = 0
    ) {
        self.routerMs = routerMs
        self.modemMs = modemMs
        self.internetMs = internetMs
        self.dnsMs = dnsMs
        self.lossPct = lossPct
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        routerMs = try c.decodeIfPresent(Double.self, forKey: .routerMs)
        modemMs = try c.decodeIfPresent(Double.self, forKey: .modemMs)
        internetMs = try c.decodeIfPresent(Double.self, forKey: .internetMs)
        dnsMs = try c.decodeIfPresent(Double.self, forKey: .dnsMs)
        lossPct = try c.decodeIfPresent(Double.self, forKey: .lossPct) ?? 0
    }
}

public struct OutageDto: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { "\(start)-\(blame)" }

    public var start: String
    public var end: String?
    public var durationSec: Double
    public var blame: String

    enum CodingKeys: String, CodingKey {
        case start, end, durationSec, blame
    }

    public init(start: String, end: String? = nil, durationSec: Double = 0, blame: String = "unknown") {
        self.start = start
        self.end = end
        self.durationSec = durationSec
        self.blame = blame
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        start = try c.decode(String.self, forKey: .start)
        end = try c.decodeIfPresent(String.self, forKey: .end)
        durationSec = try c.decodeIfPresent(Double.self, forKey: .durationSec) ?? 0
        blame = try c.decodeIfPresent(String.self, forKey: .blame) ?? "unknown"
    }

    public var blameLabel: String {
        switch blame {
        case "router": return VigiaL10n.tr("Tu router no responde")
        case "isp": return VigiaL10n.tr("Falla del proveedor (ISP)")
        case "dns": return VigiaL10n.tr("Falla de resolución DNS")
        default: return VigiaL10n.tr("Causa indeterminada")
        }
    }

    public var blameLabelEs: String { blameLabel }
}

public struct SpeedTestDto: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { at }

    public var at: String
    public var downMbps: Double?
    public var upMbps: Double?
    public var latencyMs: Double?
    public var loadedLatencyMs: Double?
    public var error: String?

    enum CodingKeys: String, CodingKey {
        case at, downMbps, upMbps, latencyMs, loadedLatencyMs, error
    }

    public init(
        at: String,
        downMbps: Double? = nil,
        upMbps: Double? = nil,
        latencyMs: Double? = nil,
        loadedLatencyMs: Double? = nil,
        error: String? = nil
    ) {
        self.at = at
        self.downMbps = downMbps
        self.upMbps = upMbps
        self.latencyMs = latencyMs
        self.loadedLatencyMs = loadedLatencyMs
        self.error = error
    }
}

public struct InternetStatusDto: Codable, Equatable, Sendable {
    public var state: String
    public var since: String?
    public var now: InternetNowDto
    public var ipv6: String
    public var outages: [OutageDto]
    public var speedTests: [SpeedTestDto]

    enum CodingKeys: String, CodingKey {
        case state, since, now, ipv6, outages, speedTests
    }

    public init(
        state: String = "unknown",
        since: String? = nil,
        now: InternetNowDto = InternetNowDto(),
        ipv6: String = "unknown",
        outages: [OutageDto] = [],
        speedTests: [SpeedTestDto] = []
    ) {
        self.state = state
        self.since = since
        self.now = now
        self.ipv6 = ipv6
        self.outages = outages
        self.speedTests = speedTests
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        state = try c.decodeIfPresent(String.self, forKey: .state) ?? "unknown"
        since = try c.decodeIfPresent(String.self, forKey: .since)
        now = try c.decodeIfPresent(InternetNowDto.self, forKey: .now) ?? InternetNowDto()
        ipv6 = try c.decodeIfPresent(String.self, forKey: .ipv6) ?? "unknown"
        outages = try c.decodeIfPresent([OutageDto].self, forKey: .outages) ?? []
        speedTests = try c.decodeIfPresent([SpeedTestDto].self, forKey: .speedTests) ?? []
    }
}

public struct InternetSeriesPointDto: Codable, Equatable, Identifiable, Sendable {
    public var id: String { at }

    public var at: String
    public var routerMs: Double?
    public var modemMs: Double?
    public var internetMs: Double?
    public var dnsMs: Double?
    public var lossPct: Double?

    enum CodingKeys: String, CodingKey {
        case at, routerMs, modemMs, internetMs, dnsMs, lossPct
    }
}

public struct InternetSettingsDto: Codable, Equatable, Sendable {
    public var speedTestsPerDay: Int

    public init(speedTestsPerDay: Int = 2) {
        self.speedTestsPerDay = speedTestsPerDay
    }
}

public enum InternetSeriesRange: String, CaseIterable, Identifiable, Sendable {
    case hours24 = "24h"
    case days7 = "7d"

    public var id: String { rawValue }
    public var label: String { rawValue }
}

/// Evento emitido por `GET /api/v1/stream` (Server-Sent Events).
public enum AgentSSEEvent: Sendable {
    case scanCompleted(data: String)
    case deviceChanged(data: String)
    case alert(AlertDto)
    case other(name: String, data: String)
}
