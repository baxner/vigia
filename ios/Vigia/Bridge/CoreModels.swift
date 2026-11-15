import Foundation

/// Resultado de un escaneo completo producido por el motor Go (`core.ScanResult`).
public struct ScanResult: Codable, Equatable, Sendable {
    public var subnet: String
    public var selfIP: String
    public var gateway: String?
    public var devices: [Device]
    public var startedAt: String
    /// Duración en nanosegundos (`time.Duration` de Go).
    public var duration: Int64

    enum CodingKeys: String, CodingKey {
        case subnet
        case selfIP = "self"
        case gateway
        case devices
        case startedAt
        case duration
    }

    public init(
        subnet: String = "",
        selfIP: String = "",
        gateway: String? = nil,
        devices: [Device] = [],
        startedAt: String = VigiaDate.nowString(),
        duration: Int64 = 0
    ) {
        self.subnet = subnet
        self.selfIP = selfIP
        self.gateway = gateway
        self.devices = devices
        self.startedAt = startedAt
        self.duration = duration
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        subnet = try c.decodeIfPresent(String.self, forKey: .subnet) ?? ""
        selfIP = try c.decodeIfPresent(String.self, forKey: .selfIP) ?? ""
        gateway = try c.decodeIfPresent(String.self, forKey: .gateway)
        devices = try c.decodeIfPresent([Device].self, forKey: .devices) ?? []
        startedAt = try c.decodeIfPresent(String.self, forKey: .startedAt) ?? VigiaDate.nowString()
        duration = try c.decodeIfPresent(Int64.self, forKey: .duration) ?? 0
    }
}

/// Equipo descubierto en la red (`core.Device`).
public struct Device: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { ip }

    public var ip: String
    public var ipv6: [String]
    public var mac: String?
    public var vendor: String?
    public var hostname: String?
    public var identity: Identity
    public var services: [ServiceInfo]
    public var openPorts: [Int]
    public var sources: [String]
    public var extra: [String: String]
    public var lastSeen: String

    enum CodingKeys: String, CodingKey {
        case ip, ipv6, mac, vendor, hostname, identity, services, openPorts, sources, extra, lastSeen
    }

    public init(
        ip: String,
        ipv6: [String] = [],
        mac: String? = nil,
        vendor: String? = nil,
        hostname: String? = nil,
        identity: Identity = Identity(),
        services: [ServiceInfo] = [],
        openPorts: [Int] = [],
        sources: [String] = [],
        extra: [String: String] = [:],
        lastSeen: String = VigiaDate.nowString()
    ) {
        self.ip = ip
        self.ipv6 = ipv6
        self.mac = mac
        self.vendor = vendor
        self.hostname = hostname
        self.identity = identity
        self.services = services
        self.openPorts = openPorts
        self.sources = sources
        self.extra = extra
        self.lastSeen = lastSeen
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ip = try c.decodeIfPresent(String.self, forKey: .ip) ?? ""
        ipv6 = try c.decodeIfPresent([String].self, forKey: .ipv6) ?? []
        mac = try c.decodeIfPresent(String.self, forKey: .mac)
        vendor = try c.decodeIfPresent(String.self, forKey: .vendor)
        hostname = try c.decodeIfPresent(String.self, forKey: .hostname)
        identity = try c.decodeIfPresent(Identity.self, forKey: .identity) ?? Identity()
        services = try c.decodeIfPresent([ServiceInfo].self, forKey: .services) ?? []
        openPorts = try c.decodeIfPresent([Int].self, forKey: .openPorts) ?? []
        sources = try c.decodeIfPresent([String].self, forKey: .sources) ?? []
        extra = try c.decodeIfPresent([String: String].self, forKey: .extra) ?? [:]
        lastSeen = try c.decodeIfPresent(String.self, forKey: .lastSeen) ?? VigiaDate.nowString()
    }

    /// Nombre más expresivo que conoce el motor (sin personalización del usuario).
    public var displayName: String {
        if let hostname = hostname?.trimmingCharacters(in: .whitespacesAndNewlines), !hostname.isEmpty {
            return hostname
        }
        let brandModel = [identity.brand, identity.model]
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
            .joined(separator: " ")
        if !brandModel.isEmpty {
            return brandModel
        }
        if let vendor = vendor?.trimmingCharacters(in: .whitespacesAndNewlines), !vendor.isEmpty {
            return vendor
        }
        return ip
    }
}

/// Clasificación inferida por el motor de identificación (`core.Identity`).
public struct Identity: Codable, Equatable, Hashable, Sendable {
    public var type: String
    public var brand: String
    public var model: String
    public var os: String
    public var confidence: Int
    public var reason: String

    enum CodingKeys: String, CodingKey {
        case type, brand, model, os, confidence, reason
    }

    public init(
        type: String = "unknown",
        brand: String = "",
        model: String = "",
        os: String = "",
        confidence: Int = 0,
        reason: String = ""
    ) {
        self.type = type
        self.brand = brand
        self.model = model
        self.os = os
        self.confidence = confidence
        self.reason = reason
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = try c.decodeIfPresent(String.self, forKey: .type) ?? "unknown"
        brand = try c.decodeIfPresent(String.self, forKey: .brand) ?? ""
        model = try c.decodeIfPresent(String.self, forKey: .model) ?? ""
        os = try c.decodeIfPresent(String.self, forKey: .os) ?? ""
        confidence = try c.decodeIfPresent(Int.self, forKey: .confidence) ?? 0
        reason = try c.decodeIfPresent(String.self, forKey: .reason) ?? ""
    }
}

/// Servicio anunciado por mDNS, SSDP u otros protocolos (`core.Service`).
public struct ServiceInfo: Codable, Equatable, Hashable, Sendable {
    public var proto: String
    public var name: String
    public var info: String?

    enum CodingKeys: String, CodingKey {
        case proto, name, info
    }

    public init(proto: String = "", name: String = "", info: String? = nil) {
        self.proto = proto
        self.name = name
        self.info = info
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        proto = try c.decodeIfPresent(String.self, forKey: .proto) ?? ""
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        info = try c.decodeIfPresent(String.self, forKey: .info)
    }
}

/// Estado del router (UPnP IGD / NAT-PMP / alcance exterior) reportado por `core.RouterInfo`.
public struct RouterInfo: Codable, Equatable, Sendable {
    public var ip: String
    public var mac: String?
    public var upnp: Bool
    public var natpmp: Bool
    public var externalIp: String
    public var reach: String
    public var portMappings: [PortMapping]

    enum CodingKeys: String, CodingKey {
        case ip, mac, upnp, natpmp, externalIp, reach, portMappings
    }

    public init(
        ip: String = "",
        mac: String? = nil,
        upnp: Bool = false,
        natpmp: Bool = false,
        externalIp: String = "",
        reach: String = "unknown",
        portMappings: [PortMapping] = []
    ) {
        self.ip = ip
        self.mac = mac
        self.upnp = upnp
        self.natpmp = natpmp
        self.externalIp = externalIp
        self.reach = reach
        self.portMappings = portMappings
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ip = try c.decodeIfPresent(String.self, forKey: .ip) ?? ""
        mac = try c.decodeIfPresent(String.self, forKey: .mac)
        upnp = try c.decodeIfPresent(Bool.self, forKey: .upnp) ?? false
        natpmp = try c.decodeIfPresent(Bool.self, forKey: .natpmp) ?? false
        externalIp = try c.decodeIfPresent(String.self, forKey: .externalIp) ?? ""
        reach = try c.decodeIfPresent(String.self, forKey: .reach) ?? "unknown"
        portMappings = try c.decodeIfPresent([PortMapping].self, forKey: .portMappings) ?? []
    }
}

/// Regla de redirección de puertos abierta en el router por UPnP o NAT-PMP (`core.PortMapping`).
public struct PortMapping: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { "\(transportProtocol)-\(externalPort)-\(internalIp)-\(internalPort)" }

    public var transportProtocol: String
    public var externalPort: Int
    public var internalIp: String
    public var internalPort: Int
    public var description: String
    public var source: String
    public var deviceId: Int64?
    public var firstSeen: String?
    public var lastSeen: String?

    enum CodingKeys: String, CodingKey {
        case transportProtocol = "protocol"
        case externalPort
        case internalIp
        case internalPort
        case description
        case source
        case deviceId
        case firstSeen
        case lastSeen
    }

    public init(
        transportProtocol: String = "TCP",
        externalPort: Int = 0,
        internalIp: String = "",
        internalPort: Int = 0,
        description: String = "",
        source: String = "upnp",
        deviceId: Int64? = nil,
        firstSeen: String? = nil,
        lastSeen: String? = nil
    ) {
        self.transportProtocol = transportProtocol
        self.externalPort = externalPort
        self.internalIp = internalIp
        self.internalPort = internalPort
        self.description = description
        self.source = source
        self.deviceId = deviceId
        self.firstSeen = firstSeen
        self.lastSeen = lastSeen
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        transportProtocol = try c.decodeIfPresent(String.self, forKey: .transportProtocol) ?? "TCP"
        externalPort = try c.decodeIfPresent(Int.self, forKey: .externalPort) ?? 0
        internalIp = try c.decodeIfPresent(String.self, forKey: .internalIp) ?? ""
        internalPort = try c.decodeIfPresent(Int.self, forKey: .internalPort) ?? 0
        description = try c.decodeIfPresent(String.self, forKey: .description) ?? ""
        source = try c.decodeIfPresent(String.self, forKey: .source) ?? "upnp"
        deviceId = try c.decodeIfPresent(Int64.self, forKey: .deviceId)
        firstSeen = try c.decodeIfPresent(String.self, forKey: .firstSeen)
        lastSeen = try c.decodeIfPresent(String.self, forKey: .lastSeen)
    }
}

/// Entrada del catálogo de puertos (`core.PortEntry`).
public struct PortInfo: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: Int { port }

    public var port: Int
    public var name: String
    public var risk: String
    public var why: String
    public var action: String

    enum CodingKeys: String, CodingKey {
        case port, name, risk, why, action
    }

    public init(port: Int, name: String, risk: String = "none", why: String = "", action: String = "") {
        self.port = port
        self.name = name
        self.risk = risk
        self.why = why
        self.action = action
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        port = try c.decode(Int.self, forKey: .port)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        risk = try c.decodeIfPresent(String.self, forKey: .risk) ?? "none"
        why = try c.decodeIfPresent(String.self, forKey: .why) ?? ""
        action = try c.decodeIfPresent(String.self, forKey: .action) ?? ""
    }

    public var isRisky: Bool {
        risk == "medium" || risk == "high"
    }
}

/// Etiquetas personalizadas por el usuario sobre un equipo (`agent/history.DeviceCustomization`).
public struct DeviceCustomization: Codable, Equatable, Hashable, Sendable {
    public var name: String?
    public var typeId: String?
    public var notes: String?
    public var location: String?
    public var important: Bool

    enum CodingKeys: String, CodingKey {
        case name, typeId, notes, location, important
    }

    public init(
        name: String? = nil,
        typeId: String? = nil,
        notes: String? = nil,
        location: String? = nil,
        important: Bool = false
    ) {
        self.name = name
        self.typeId = typeId
        self.notes = notes
        self.location = location
        self.important = important
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decodeIfPresent(String.self, forKey: .name)
        typeId = try c.decodeIfPresent(String.self, forKey: .typeId)
        notes = try c.decodeIfPresent(String.self, forKey: .notes)
        location = try c.decodeIfPresent(String.self, forKey: .location)
        important = try c.decodeIfPresent(Bool.self, forKey: .important) ?? false
    }

    public var isEmpty: Bool {
        name == nil && typeId == nil && notes == nil && location == nil && !important
    }
}

/// Equipo persistido en el inventario del Agente 24/7 (`agent/history.KnownDevice`).
public struct KnownDevice: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: Int64
    public var network: String
    public var mac: String?
    public var lastIp: String
    public var firstSeen: String
    public var lastSeen: String
    public var online: Bool
    public var onlineSince: String?
    public var last: Device
    public var custom: DeviceCustomization

    enum CodingKeys: String, CodingKey {
        case id, network, mac, lastIp, firstSeen, lastSeen, online, onlineSince, last, custom
    }

    public init(
        id: Int64,
        network: String = "",
        mac: String? = nil,
        lastIp: String = "",
        firstSeen: String = "",
        lastSeen: String = "",
        online: Bool = true,
        onlineSince: String? = nil,
        last: Device,
        custom: DeviceCustomization = DeviceCustomization()
    ) {
        self.id = id
        self.network = network
        self.mac = mac
        self.lastIp = lastIp
        self.firstSeen = firstSeen
        self.lastSeen = lastSeen
        self.online = online
        self.onlineSince = onlineSince
        self.last = last
        self.custom = custom
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int64.self, forKey: .id)
        network = try c.decodeIfPresent(String.self, forKey: .network) ?? ""
        mac = try c.decodeIfPresent(String.self, forKey: .mac)
        lastIp = try c.decodeIfPresent(String.self, forKey: .lastIp) ?? ""
        firstSeen = try c.decodeIfPresent(String.self, forKey: .firstSeen) ?? ""
        lastSeen = try c.decodeIfPresent(String.self, forKey: .lastSeen) ?? ""
        online = try c.decodeIfPresent(Bool.self, forKey: .online) ?? false
        onlineSince = try c.decodeIfPresent(String.self, forKey: .onlineSince)
        last = try c.decodeIfPresent(Device.self, forKey: .last) ?? Device(ip: lastIp)
        custom = try c.decodeIfPresent(DeviceCustomization.self, forKey: .custom) ?? DeviceCustomization()
    }

    public var displayName: String {
        if let customName = custom.name?.trimmingCharacters(in: .whitespacesAndNewlines), !customName.isEmpty {
            return customName
        }
        return last.displayName
    }
}

/// Evento histórico de un equipo (`agent/history.DeviceEvent`).
public struct DeviceEvent: Codable, Equatable, Hashable, Identifiable, Sendable {
    public var id: String { "\(deviceId)-\(type)-\(at)" }

    public var deviceId: Int64
    public var type: String
    public var at: String
    public var detail: String?

    enum CodingKeys: String, CodingKey {
        case deviceId, type, at, detail
    }

    public init(deviceId: Int64, type: String, at: String, detail: String? = nil) {
        self.deviceId = deviceId
        self.type = type
        self.at = at
        self.detail = detail
    }

    public var label: String {
        switch type {
        case "FIRST_SEEN": return VigiaL10n.tr("Visto por primera vez")
        case "CAME_ONLINE": return VigiaL10n.tr("Se conectó")
        case "WENT_OFFLINE": return VigiaL10n.tr("Se desconectó")
        case "IP_CHANGED": return VigiaL10n.tr("Cambió de IP")
        default: return type
        }
    }

    public var labelEs: String { label }
}

/// Categorías de equipo reconocidas por el motor (`core/identity`).
public enum DeviceType: String, CaseIterable, Codable, Identifiable, Sendable {
    case phone
    case computer
    case tv
    case router
    case printer
    case camera
    case speaker
    case console
    case nas
    case iot
    case wearable
    case tablet
    case unknown

    public var id: String { rawValue }

    public static func fromRaw(_ raw: String?) -> DeviceType {
        guard let raw = raw?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
              !raw.isEmpty else {
            return .unknown
        }
        return DeviceType(rawValue: raw) ?? .unknown
    }

    public var label: String {
        switch self {
        case .phone: return VigiaL10n.tr("Teléfono")
        case .computer: return VigiaL10n.tr("Computadora")
        case .tv: return VigiaL10n.tr("TV")
        case .router: return VigiaL10n.tr("Router")
        case .printer: return VigiaL10n.tr("Impresora")
        case .camera: return VigiaL10n.tr("Cámara")
        case .speaker: return VigiaL10n.tr("Bocina")
        case .console: return VigiaL10n.tr("Consola")
        case .nas: return VigiaL10n.tr("NAS")
        case .iot: return VigiaL10n.tr("Inteligente")
        case .wearable: return VigiaL10n.tr("Reloj")
        case .tablet: return VigiaL10n.tr("Tablet")
        case .unknown: return VigiaL10n.tr("Sin identificar")
        }
    }

    public var labelEs: String { label }

    public var sfSymbol: String {
        switch self {
        case .phone: return "iphone"
        case .computer: return "laptopcomputer"
        case .tv: return "tv"
        case .router: return "wifi.router"
        case .printer: return "printer"
        case .camera: return "video"
        case .speaker: return "hifispeaker"
        case .console: return "gamecontroller"
        case .nas: return "externaldrive.connected.to.line.below"
        case .iot: return "sensor"
        case .wearable: return "applewatch"
        case .tablet: return "ipad"
        case .unknown: return "questionmark.circle"
        }
    }

    /// Anillo en el radar del Panel (0 centro -> 3 periferia).
    public var radarRing: Int {
        switch self {
        case .router: return 0
        case .computer, .phone, .tablet: return 1
        case .tv, .speaker, .console, .printer, .nas: return 2
        case .camera, .iot, .wearable, .unknown: return 3
        }
    }
}
