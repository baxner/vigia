import Foundation
import Darwin
import Network

/// Instantánea de la red local obtenida desde las interfaces del iPhone (`getifaddrs` + `NWPathMonitor`).
public struct LocalNetworkSnapshot: Equatable, Sendable {
    public var interfaceName: String
    public var ipv4: String
    public var netmask: String
    public var prefixLength: Int
    public var cidr: String
    public var inferredGateway: String
    public var isWifi: Bool
    public var isVpnActive: Bool

    public var isAvailable: Bool {
        !ipv4.isEmpty && !cidr.isEmpty
    }
}

/// Lee IPv4, máscara de subred, CIDR e interfaz activa del iPhone mediante `<ifaddrs.h>` (`Darwin.getifaddrs`)
/// combinado con `NWPathMonitor` para detectar cambios de Wi-Fi y túneles VPN.
public final class IfaddrsNetworkProvider: @unchecked Sendable {
    public static let shared = IfaddrsNetworkProvider()

    private let monitor = NWPathMonitor()
    private let queue = DispatchQueue(label: "com.baxner.vigia.network-monitor", qos: .utility)
    private let lock = NSLock()

    private var latestSnapshot: LocalNetworkSnapshot?
    private var pathHasWifi = false
    private var started = false
    private var observers: [UUID: @Sendable (LocalNetworkSnapshot) -> Void] = [:]

    private init() {}

    public func startMonitoring() {
        lock.lock()
        let alreadyStarted = started
        started = true
        lock.unlock()
        guard !alreadyStarted else { return }

        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let hasWifi = path.usesInterfaceType(.wifi)
            self.lock.lock()
            self.pathHasWifi = hasWifi
            self.lock.unlock()
            let snap = self.currentSnapshot(overrideWifiFlag: hasWifi)
            self.notify(snap)
        }
        monitor.start(queue: queue)
    }

    @discardableResult
    public func addObserver(_ handler: @escaping @Sendable (LocalNetworkSnapshot) -> Void) -> UUID {
        let id = UUID()
        lock.lock()
        observers[id] = handler
        lock.unlock()
        startMonitoring()
        handler(currentSnapshot())
        return id
    }

    public func removeObserver(_ id: UUID) {
        lock.lock()
        observers.removeValue(forKey: id)
        lock.unlock()
    }

    private func notify(_ snapshot: LocalNetworkSnapshot) {
        lock.lock()
        latestSnapshot = snapshot
        let handlers = Array(observers.values)
        lock.unlock()
        for handler in handlers {
            handler(snapshot)
        }
    }

    public func currentSnapshot(overrideWifiFlag: Bool? = nil) -> LocalNetworkSnapshot {
        var ifaddrPtr: UnsafeMutablePointer<ifaddrs>?
        guard Darwin.getifaddrs(&ifaddrPtr) == 0, let firstAddr = ifaddrPtr else {
            return LocalNetworkSnapshot(
                interfaceName: "",
                ipv4: "",
                netmask: "",
                prefixLength: 0,
                cidr: "",
                inferredGateway: "",
                isWifi: overrideWifiFlag ?? pathHasWifi,
                isVpnActive: false
            )
        }
        defer { Darwin.freeifaddrs(ifaddrPtr) }

        var en0Match: (name: String, ip: UInt32, mask: UInt32)?
        var fallbackWifiMatch: (name: String, ip: UInt32, mask: UInt32)?
        var vpnDetected = false

        var cursor: UnsafeMutablePointer<ifaddrs>? = firstAddr
        while let entry = cursor?.pointee {
            defer { cursor = entry.ifa_next }
            let name = String(cString: entry.ifa_name)
            let flags = Int32(entry.ifa_flags)
            let isUp = (flags & IFF_UP) != 0
            let isLoopback = (flags & IFF_LOOPBACK) != 0
            guard isUp, !isLoopback else { continue }

            if name.hasPrefix("utun") || name.hasPrefix("ipsec") || name.hasPrefix("ppp") {
                vpnDetected = true
            }

            guard let sa = entry.ifa_addr, sa.pointee.sa_family == UInt8(AF_INET) else {
                continue
            }

            let ipStruct = sa.withMemoryRebound(to: sockaddr_in.self, capacity: 1) { $0.pointee }
            let ipHost = UInt32(bigEndian: ipStruct.sin_addr.s_addr)

            let maskHost: UInt32
            if let netmaskSA = entry.ifa_netmask, netmaskSA.pointee.sa_family == UInt8(AF_INET) {
                let maskStruct = netmaskSA.withMemoryRebound(to: sockaddr_in.self, capacity: 1) { $0.pointee }
                maskHost = UInt32(bigEndian: maskStruct.sin_addr.s_addr)
            } else {
                maskHost = 0xFFFF_FF00
            }

            // Evita enlaces link-local 169.254.0.0/16 si hay una interfaz LAN real.
            let isLinkLocal = (ipHost & 0xFFFF_0000) == 0xA9FE_0000
            guard !isLinkLocal else { continue }

            if name == "en0" {
                en0Match = (name, ipHost, maskHost)
            } else if name.hasPrefix("en") && fallbackWifiMatch == nil {
                fallbackWifiMatch = (name, ipHost, maskHost)
            }
        }

        let chosen = en0Match ?? fallbackWifiMatch
        let isWifi: Bool
        if let overrideWifiFlag {
            isWifi = overrideWifiFlag || en0Match != nil
        } else {
            lock.lock()
            let monitoredWifi = pathHasWifi
            lock.unlock()
            isWifi = monitoredWifi || en0Match != nil
        }

        guard let chosen else {
            return LocalNetworkSnapshot(
                interfaceName: "",
                ipv4: "",
                netmask: "",
                prefixLength: 0,
                cidr: "",
                inferredGateway: "",
                isWifi: isWifi,
                isVpnActive: vpnDetected
            )
        }

        let ipString = formatIPv4(chosen.ip)
        let maskString = formatIPv4(chosen.mask)
        let rawPrefix = chosen.mask.nonzeroBitCount
        // Para escaneo móvil en subredes grandes (p. ej. /16 de campus/hotel) limitamos
        // el rango práctico de barrido a /24 como en Android, conservando el prefijo real.
        let scanPrefix = max(24, rawPrefix)
        let scanMask: UInt32 = scanPrefix == 0 ? 0 : (~UInt32(0) << (32 - scanPrefix))
        let networkBase = chosen.ip & scanMask
        let cidr = "\(formatIPv4(networkBase))/\(scanPrefix)"
        let gateway = formatIPv4((chosen.ip & chosen.mask) | 1)

        return LocalNetworkSnapshot(
            interfaceName: chosen.name,
            ipv4: ipString,
            netmask: maskString,
            prefixLength: rawPrefix,
            cidr: cidr,
            inferredGateway: gateway,
            isWifi: isWifi,
            isVpnActive: vpnDetected
        )
    }

    private func formatIPv4(_ hostOrder: UInt32) -> String {
        let a = (hostOrder >> 24) & 0xFF
        let b = (hostOrder >> 16) & 0xFF
        let c = (hostOrder >> 8) & 0xFF
        let d = hostOrder & 0xFF
        return "\(a).\(b).\(c).\(d)"
    }
}
