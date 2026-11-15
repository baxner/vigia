import Foundation
import CoreLocation
import NetworkExtension

/// Estado de la red Wi-Fi actual devuelto por `NEHotspotNetwork.fetchCurrent`.
public struct HotspotWifiSnapshot: Equatable, Sendable {
    public var ssid: String
    public var bssid: String
    /// Cadena enviada al motor de seguridad (`open`, `wep`, o `""` cuando es `.personal` / `.enterprise` / desconocida).
    public var securityWireValue: String
    /// Etiqueta legible en español para la vista Wi-Fi.
    public var securityDisplayLabel: String
    public var signalStrength: Double?
    public var permissionBlocked: Bool

    public init(
        ssid: String = "",
        bssid: String = "",
        securityWireValue: String = "",
        securityDisplayLabel: String = VigiaL10n.tr("Desconocida"),
        signalStrength: Double? = nil,
        permissionBlocked: Bool = false
    ) {
        self.ssid = ssid
        self.bssid = bssid
        self.securityWireValue = securityWireValue
        self.securityDisplayLabel = securityDisplayLabel
        self.signalStrength = signalStrength
        self.permissionBlocked = permissionBlocked
    }

    public var isAvailable: Bool {
        !ssid.isEmpty || !bssid.isEmpty
    }

    public func toWifiInfo() -> WifiInfo? {
        guard isAvailable else { return nil }
        return WifiInfo(ssid: ssid, security: securityWireValue, wps: false)
    }
}

/// Proveedor Wi-Fi para iOS basado en `NEHotspotNetwork.fetchCurrent`.
///
/// Notas importantes de plataforma Apple:
/// 1. `NEHotspotNetworkSecurityType.personal` agrupa tanto WPA2 Personal como WPA3 Personal (SAE).
///    Para no disparar falsos positivos `WIFI_WPA2` en routers WPA3 modernos, `.personal` se mapea a `""`
///    en `securityWireValue`, mientras que `.open` ("open") y `.wep` ("wep") sí se reportan al motor.
/// 2. Apple restringe la enumeración de puntos de acceso Wi-Fi vecinos (`NEHotspotHelper` requiere un
///    entitlement especial de Apple dirigido a operadores/hotspots, y `CoreWLAN` es exclusivo de macOS).
public final class HotspotWifiProvider: NSObject, CLLocationManagerDelegate, @unchecked Sendable {
    public static let shared = HotspotWifiProvider()

    public static var iosLimitationExplanation: String {
        VigiaL10n.tr("ios.wifi.limitation_explanation")
    }

    private let locationManager = CLLocationManager()
    private var locationContinuation: CheckedContinuation<CLAuthorizationStatus, Never>?
    private let lock = NSLock()

    private override init() {
        super.init()
        locationManager.delegate = self
    }

    public var locationAuthorizationStatus: CLAuthorizationStatus {
        locationManager.authorizationStatus
    }

    public var hasLocationPermissionForWifi: Bool {
        let status = locationManager.authorizationStatus
        return status == .authorizedWhenInUse || status == .authorizedAlways
    }

    @MainActor
    public func requestLocationPermission() async -> CLAuthorizationStatus {
        let current = locationManager.authorizationStatus
        if current != .notDetermined {
            return current
        }
        return await withCheckedContinuation { continuation in
            self.lock.lock()
            self.locationContinuation = continuation
            self.lock.unlock()
            self.locationManager.requestWhenInUseAuthorization()
        }
    }

    public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        guard status != .notDetermined else { return }
        lock.lock()
        let cont = locationContinuation
        locationContinuation = nil
        lock.unlock()
        cont?.resume(returning: status)
    }

    /// Consulta la red Wi-Fi conectada actualmente usando `NEHotspotNetwork.fetchCurrent`.
    public func fetchCurrentWifi() async -> HotspotWifiSnapshot {
        let auth = locationManager.authorizationStatus
        let authorized = (auth == .authorizedWhenInUse || auth == .authorizedAlways)

        return await withCheckedContinuation { continuation in
            NEHotspotNetwork.fetchCurrent { network in
                guard let network else {
                    continuation.resume(returning: HotspotWifiSnapshot(
                        permissionBlocked: !authorized
                    ))
                    return
                }

                let mapped = Self.mapSecurityType(network.securityType)
                let signal: Double? = network.signalStrength > 0 ? network.signalStrength : nil

                continuation.resume(returning: HotspotWifiSnapshot(
                    ssid: network.ssid,
                    bssid: network.bssid,
                    securityWireValue: mapped.wire,
                    securityDisplayLabel: mapped.label,
                    signalStrength: signal,
                    permissionBlocked: false
                ))
            }
        }
    }

    /// Mapeo explícito requerido por el motor de seguridad sin falsos positivos WPA2 en WPA3:
    /// `.open -> "open"`, `.wep -> "wep"`, `.personal -> ""`, `.enterprise -> ""`, `.unknown -> ""`.
    public static func mapSecurityType(_ type: NEHotspotNetworkSecurityType) -> (wire: String, label: String) {
        switch type {
        case .open:
            return ("open", VigiaL10n.tr("Abierta (sin contraseña)"))
        case .WEP:
            return ("wep", VigiaL10n.tr("WEP (insegura)"))
        case .personal:
            return ("", VigiaL10n.tr("WPA2 / WPA3 Personal"))
        case .enterprise:
            return ("", VigiaL10n.tr("WPA2 / WPA3 Empresarial"))
        case .unknown:
            return ("", VigiaL10n.tr("Desconocida"))
        @unknown default:
            return ("", VigiaL10n.tr("Desconocida"))
        }
    }
}
