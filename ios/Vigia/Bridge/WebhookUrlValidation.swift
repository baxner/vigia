import Foundation
import Darwin

/// Resultado detallado de validar una URL de webhook antes de enviarla al Agente.
public enum WebhookUrlValidationResult: Equatable, Sendable {
    case valid
    case invalid(reason: String)

    public var isValid: Bool {
        if case .valid = self { return true }
        return false
    }

    public var errorMessage: String? {
        if case .invalid(let reason) = self { return reason }
        return nil
    }
}

/// Valida URLs de webhooks para prevenir SSRF y esquemas inseguros (paridad con `WebhookUrlValidation.kt` en Android).
///
/// Exige esquema `http` o `https` con host válido y rechaza hosts de loopback (`localhost`, `127.0.0.0/8`, `::1`),
/// direcciones no especificadas (`0.0.0.0`, `::`) y direcciones link-local (`169.254.0.0/16`, `fe80::/10`).
public enum WebhookUrlValidator {
    public static func isValid(_ raw: String) -> Bool {
        validate(raw).isValid
    }

    public static func validate(_ raw: String) -> WebhookUrlValidationResult {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return .invalid(reason: VigiaL10n.tr("La URL del webhook no puede estar vacía."))
        }

        // URLComponents maneja correctamente direcciones IPv6 entre corchetes y valida sintaxis RFC 3986.
        guard let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased(),
              let rawHost = components.host,
              !rawHost.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return .invalid(reason: VigiaL10n.tr("Ingresa una URL válida con http:// o https://."))
        }

        guard scheme == "http" || scheme == "https" else {
            return .invalid(reason: VigiaL10n.tr("Solo se permiten URLs con esquema http:// o https://."))
        }

        if let port = components.port, !(1...65535).contains(port) {
            return .invalid(reason: VigiaL10n.tr("El puerto de la URL debe estar entre 1 y 65535."))
        }

        if isDisallowedHost(rawHost) {
            return .invalid(reason: VigiaL10n.tr("Por seguridad, no se permiten direcciones locales del dispositivo (localhost, loopback o enlace local)."))
        }

        return .valid
    }

    public static func isDisallowedHost(_ rawHost: String) -> Bool {
        var host = rawHost.trimmingCharacters(in: .whitespacesAndNewlines)
        if host.hasPrefix("[") && host.hasSuffix("]") && host.count >= 2 {
            host = String(host.dropFirst().dropLast())
        }
        if let zoneIdx = host.firstIndex(of: "%") {
            host = String(host[..<zoneIdx])
        }
        while host.hasSuffix(".") {
            host.removeLast()
        }
        host = host.lowercased()

        guard !host.isEmpty else { return true }

        if host == "localhost"
            || host.hasSuffix(".localhost")
            || host == "ip6-localhost"
            || host == "ip6-loopback" {
            return true
        }

        // Literal IPv6 (parseo local sin disparar resolución DNS)
        if host.contains(":") {
            var addr6 = in6_addr()
            guard Darwin.inet_pton(AF_INET6, host, &addr6) == 1 else {
                return true
            }
            return isForbiddenIPv6(addr6)
        }

        // Hex IP (p. ej. 0x7f000001) o numérico compacto (p. ej. 2130706433)
        if host.range(of: #"^0x[0-9a-f]+$"#, options: .regularExpression) != nil {
            return true
        }
        if host.range(of: #"^[\d.]+$"#, options: .regularExpression) != nil {
            guard host.range(of: #"^\d{1,3}(\.\d{1,3}){3}$"#, options: .regularExpression) != nil else {
                return true
            }
            let octets = host.split(separator: ".").compactMap { Int($0) }
            guard octets.count == 4, octets.allSatisfy({ (0...255).contains($0) }) else {
                return true
            }
            var addr4 = in_addr()
            guard Darwin.inet_pton(AF_INET, host, &addr4) == 1 else {
                return true
            }
            return isForbiddenIPv4(octets: octets)
        }

        return false
    }

    private static func isForbiddenIPv4(octets: [Int]) -> Bool {
        guard octets.count == 4 else { return true }
        // 0.0.0.0/8 (any / unspecified)
        if octets[0] == 0 { return true }
        // 127.0.0.0/8 (loopback)
        if octets[0] == 127 { return true }
        // 169.254.0.0/16 (link-local / cloud metadata)
        if octets[0] == 169 && octets[1] == 254 { return true }
        return false
    }

    private static func isForbiddenIPv6(_ addr: in6_addr) -> Bool {
        let bytes = withUnsafeBytes(of: addr) { Array($0) }
        guard bytes.count == 16 else { return true }

        // :: (unspecified)
        if bytes.allSatisfy({ $0 == 0 }) { return true }

        // ::1 (loopback)
        if bytes[0..<15].allSatisfy({ $0 == 0 }) && bytes[15] == 1 { return true }

        // fe80::/10 (link-local)
        if bytes[0] == 0xFE && (bytes[1] & 0xC0) == 0x80 { return true }

        // ::ffff:0:0/96 (IPv4-mapped IPv6)
        let isIPv4Mapped = bytes[0..<10].allSatisfy({ $0 == 0 }) && bytes[10] == 0xFF && bytes[11] == 0xFF
        if isIPv4Mapped {
            return isForbiddenIPv4(octets: [Int(bytes[12]), Int(bytes[13]), Int(bytes[14]), Int(bytes[15])])
        }

        return false
    }
}
