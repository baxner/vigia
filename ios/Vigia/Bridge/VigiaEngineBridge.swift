import Foundation
import Darwin
#if canImport(VigiaCore)
import VigiaCore
#endif

/// Error lanzado por el puente al motor de red (`VigiaCore` o respaldo Swift).
public enum VigiaEngineError: LocalizedError, Sendable {
    case invalidInput(String)
    case engineFailure(String)
    case cancelled

    public var errorDescription: String? {
        switch self {
        case .invalidInput(let msg): return msg
        case .engineFailure(let msg): return msg
        case .cancelled: return VigiaL10n.tr("Escaneo cancelado")
        }
    }
}

/// Puente unificado hacia el motor Go compartido (`VigiaCore.xcframework`) mediante `gomobile`.
/// Cuando `VigiaCore` no está enlazado en el entorno de compilación, proporciona un respaldo
/// completo en Swift puro (catálogo de puertos, reporte de seguridad, OUI comunes y escaneo TCP local)
/// para que la app compile y opere de forma coherente.
public final class VigiaEngineBridge: @unchecked Sendable {
    public static let shared = VigiaEngineBridge()

    private let catalogByPort: [Int: PortInfo]
    private let catalogOrdered: [PortInfo]
    private let activeScannerLock = NSLock()
    #if canImport(VigiaCore)
    private var currentMobileScanner: MobileScanner?
    #endif
    private var cancelRequested = false

    private func setCancelRequested(_ value: Bool) {
        activeScannerLock.lock()
        cancelRequested = value
        activeScannerLock.unlock()
    }

    private func isCancelRequested() -> Bool {
        activeScannerLock.lock()
        defer { activeScannerLock.unlock() }
        return cancelRequested
    }

    private init() {
        #if canImport(VigiaCore)
        let rawJson = MobilePortCatalog()
        if let data = rawJson.data(using: .utf8),
           let decoded = try? JSONDecoder().decode([PortInfo].self, from: data),
           !decoded.isEmpty {
            catalogOrdered = decoded
            catalogByPort = Dictionary(uniqueKeysWithValues: decoded.map { ($0.port, $0) })
            return
        }
        #endif
        let entries = PurePortCatalog.entries
        catalogOrdered = entries
        catalogByPort = Dictionary(uniqueKeysWithValues: entries.map { ($0.port, $0) })
    }

    /// Indica si el XCFramework nativo en Go está enlazado en tiempo de compilación.
    public var isCoreEngineLinked: Bool {
        #if canImport(VigiaCore)
        return true
        #else
        return false
        #endif
    }

    public func portCatalog() -> [PortInfo] {
        catalogOrdered
    }

    public func portInfo(_ port: Int) -> PortInfo? {
        catalogByPort[port]
    }

    public func riskyPorts(among ports: [Int]) -> [PortInfo] {
        ports.compactMap { catalogByPort[$0] }.filter { $0.isRisky }
    }

    public func vendorForMAC(_ mac: String?) -> String? {
        guard let mac = mac?.trimmingCharacters(in: .whitespacesAndNewlines), !mac.isEmpty else {
            return nil
        }
        #if canImport(VigiaCore)
        let res = MobileVendorForMAC(mac)
        return res.isEmpty ? nil : res
        #else
        return PureOuiLookup.vendor(forMAC: mac)
        #endif
    }

    public func networkKey(gateway: String, subnet: String) -> String {
        #if canImport(VigiaCore)
        return MobileNetworkKey(gateway, subnet)
        #else
        return "\(gateway.trimmingCharacters(in: .whitespacesAndNewlines))|\(subnet.trimmingCharacters(in: .whitespacesAndNewlines))"
        #endif
    }

    public func brandForMAC(_ mac: String?) -> String? {
        guard let mac = mac?.trimmingCharacters(in: .whitespacesAndNewlines), !mac.isEmpty else {
            return nil
        }
        #if canImport(VigiaCore)
        let res = MobileBrandForMAC(mac)
        return res.isEmpty ? nil : res
        #else
        return PureOuiLookup.brand(forMAC: mac)
        #endif
    }

    /// Ejecuta un escaneo completo de la subred usando `MobileScanner` (si `VigiaCore` está presente)
    /// o el escáner TCP concurrente en Swift puro en caso contrario.
    /// - Parameter bindIface: Interfaz local a la que vincular sockets (p. ej. `"en0"` cuando hay VPN/Tailscale activa).
    public func scan(
        cidr: String,
        gateway: String,
        selfIP: String = "",
        withPorts: Bool = true,
        bindIface: String = "",
        onProgress: @escaping @Sendable (_ stage: String, _ done: Int, _ total: Int) -> Void
    ) async throws -> ScanResult {
        setCancelRequested(false)

        #if canImport(VigiaCore)
        return try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async { [weak self] in
                guard let self else {
                    continuation.resume(throwing: VigiaEngineError.cancelled)
                    return
                }
                guard let scanner = MobileNewScanner() else {
                    continuation.resume(throwing: VigiaEngineError.engineFailure(VigiaL10n.tr("No se pudo iniciar MobileScanner")))
                    return
                }
                self.activeScannerLock.lock()
                self.currentMobileScanner = scanner
                self.activeScannerLock.unlock()

                defer {
                    self.activeScannerLock.lock()
                    self.currentMobileScanner = nil
                    self.activeScannerLock.unlock()
                }

                let listener = MobileProgressBridge(handler: onProgress)
                do {
                    var err: NSError?
                    let json: String
                    if let config = MobileNewScanConfig() {
                        config.cidr = cidr
                        config.gateway = gateway
                        config.withPorts = withPorts
                        config.bindIface = bindIface
                        json = scanner.scan(with: config, listener: listener, error: &err)
                    } else if !bindIface.isEmpty {
                        json = scanner.scan(onInterface: cidr, gateway: gateway, withPorts: withPorts, bindIface: bindIface, listener: listener, error: &err)
                    } else {
                        json = scanner.scan(cidr, gateway: gateway, withPorts: withPorts, listener: listener, error: &err)
                    }
                    if let err {
                        throw err
                    }
                    guard let data = json.data(using: .utf8) else {
                        continuation.resume(throwing: VigiaEngineError.engineFailure(VigiaL10n.tr("Salida inválida de MobileScanner")))
                        return
                    }
                    let result = try VigiaJSON.makeDecoder().decode(ScanResult.self, from: data)
                    continuation.resume(returning: result)
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
        #else
        return try await PureSwiftScanner.scan(
            cidr: cidr,
            gateway: gateway,
            selfIP: selfIP,
            withPorts: withPorts,
            bindIface: bindIface,
            isCancelled: { [weak self] in self?.isCancelRequested() ?? true },
            onProgress: onProgress
        )
        #endif
    }

    /// Escanea puertos específicos sobre una IP (por defecto `all` equivale al catálogo completo conocido).
    public func scanPorts(ip: String, spec: String = "all", bindIface: String = "") async throws -> [Int] {
        guard !ip.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw VigiaEngineError.invalidInput(VigiaL10n.tr("IP vacía"))
        }
        #if canImport(VigiaCore)
        return try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                guard let scanner = MobileNewScanner() else {
                    continuation.resume(throwing: VigiaEngineError.engineFailure(VigiaL10n.tr("No se pudo iniciar MobileScanner")))
                    return
                }
                do {
                    var err: NSError?
                    let json: String
                    if bindIface.isEmpty {
                        json = scanner.scanPorts(ip, spec: spec, error: &err)
                    } else {
                        json = scanner.scanPorts(onInterface: ip, spec: spec, bindIface: bindIface, error: &err)
                    }
                    if let err {
                        throw err
                    }
                    guard let data = json.data(using: .utf8) else {
                        continuation.resume(returning: [])
                        return
                    }
                    let ports = try JSONDecoder().decode([Int].self, from: data)
                    continuation.resume(returning: ports)
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
        #else
        let targets: [Int]
        if spec == "all" || spec.isEmpty {
            targets = PurePortCatalog.entries.map(\.port)
        } else {
            targets = PureSwiftScanner.parsePortSpec(spec)
        }
        return await PureSwiftScanner.scanPorts(ip: ip, ports: targets, bindIface: bindIface)
        #endif
    }

    public func cancelScan() {
        setCancelRequested(true)
        #if canImport(VigiaCore)
        activeScannerLock.lock()
        currentMobileScanner?.cancel()
        activeScannerLock.unlock()
        #endif
    }

    /// Inspecciona UPnP / NAT-PMP / puertos del router desde el dispositivo local.
    public func routerInfo(gateway: String, bindIface: String = "") async throws -> RouterInfo {
        guard !gateway.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw VigiaEngineError.invalidInput(VigiaL10n.tr("Gateway vacío"))
        }
        #if canImport(VigiaCore)
        return try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .utility).async {
                var err: NSError?
                let json = bindIface.isEmpty
                    ? MobileRouterInfo(gateway, &err)
                    : MobileRouterInfoOnInterface(gateway, bindIface, &err)
                if let err {
                    continuation.resume(throwing: err)
                    return
                }
                do {
                    guard let data = json.data(using: .utf8) else {
                        continuation.resume(returning: RouterInfo(ip: gateway))
                        return
                    }
                    let info = try VigiaJSON.makeDecoder().decode(RouterInfo.self, from: data)
                    continuation.resume(returning: info)
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
        #else
        return await PureSwiftScanner.probeRouter(gateway: gateway, bindIface: bindIface)
        #endif
    }

    /// Calcula el informe de seguridad local llamando a `MobileSecurityReport` si está disponible,
    /// o con el evaluador equivalente en Swift puro.
    public func securityReport(input: SecurityInput) throws -> SecurityReport {
        #if canImport(VigiaCore)
        let data = try VigiaJSON.makeEncoder().encode(input)
        guard let inputJson = String(data: data, encoding: .utf8) else {
            throw VigiaEngineError.invalidInput("No se pudo serializar SecurityInput")
        }
        var err: NSError?
        let reportJson = MobileSecurityReport(inputJson, &err)
        if let err {
            throw err
        }
        guard let outData = reportJson.data(using: .utf8) else {
            throw VigiaEngineError.engineFailure("Respuesta inválida de MobileSecurityReport")
        }
        return try VigiaJSON.makeDecoder().decode(SecurityReport.self, from: outData)
        #else
        return PureSecurityReportBuilder.build(input: input, portLookup: { portInfo($0) })
        #endif
    }
}

#if canImport(VigiaCore)
private final class MobileProgressBridge: NSObject, MobileProgressListenerProtocol {
    private let handler: @Sendable (_ stage: String, _ done: Int, _ total: Int) -> Void

    init(handler: @escaping @Sendable (_ stage: String, _ done: Int, _ total: Int) -> Void) {
        self.handler = handler
    }

    func onProgress(_ stage: String?, done: Int, total: Int) {
        handler(stage ?? "", done, total)
    }
}
#endif

// MARK: - Respaldo en Swift puro (catálogo, OUI, seguridad y escaneo TCP local)

private enum PurePortCatalog {
    static let entries: [PortInfo] = [
        PortInfo(port: 21, name: "FTP", risk: "medium",
                 why: "Transfiere archivos y contraseñas sin cifrar: cualquiera en tu red puede leerlos.",
                 action: "Si no lo usas, desactívalo en ese equipo; si lo necesitas, usa SFTP (por SSH)."),
        PortInfo(port: 22, name: "SSH", risk: "low",
                 why: "Permite entrar a la terminal del equipo; es seguro solo con una contraseña fuerte o con llaves.",
                 action: "Usa una contraseña fuerte o llaves SSH, y apágalo si no entras por ahí."),
        PortInfo(port: 23, name: "Telnet", risk: "high",
                 why: "Da acceso a la terminal del equipo sin cifrar: la contraseña viaja a la vista de cualquiera en tu red.",
                 action: "Desactívalo; si el aparato no lo permite, cambia su contraseña de fábrica y actualízalo."),
        PortInfo(port: 25, name: "SMTP", risk: "low",
                 why: "Es un servidor de correo, algo raro en un equipo de casa.",
                 action: "Si no sabes por qué ese equipo envía correo, revisa qué programa abrió el puerto."),
        PortInfo(port: 53, name: "DNS", risk: "none"),
        PortInfo(port: 80, name: "HTTP", risk: "low",
                 why: "Es un panel web sin cifrar: lo que escribas ahí, contraseñas incluidas, viaja a la vista en tu red.",
                 action: "Cambia la contraseña de fábrica del equipo y, si lo permite, entra por HTTPS."),
        PortInfo(port: 81, name: "HTTP alt.", risk: "low",
                 why: "Panel web sin cifrar en un puerto alterno, común en cámaras y grabadores.",
                 action: "Cambia la contraseña de fábrica y mantén el equipo actualizado."),
        PortInfo(port: 110, name: "POP3", risk: "low",
                 why: "Servidor de correo sin cifrar, algo raro en un equipo de casa.",
                 action: "Si no sabes qué programa lo abrió, revísalo o desactívalo."),
        PortInfo(port: 111, name: "rpcbind", risk: "medium",
                 why: "Anuncia servicios de Linux como NFS; suele ser la puerta a carpetas compartidas.",
                 action: "Si ese equipo no comparte carpetas por NFS, desactiva rpcbind."),
        PortInfo(port: 135, name: "RPC de Windows", risk: "low",
                 why: "Lo abre Windows para administrarse en red; es normal, pero es blanco habitual de gusanos.",
                 action: "Mantén Windows actualizado y su firewall encendido."),
        PortInfo(port: 139, name: "NetBIOS", risk: "medium",
                 why: "Comparte archivos e impresoras al estilo del Windows antiguo y revela el nombre del equipo y sus carpetas.",
                 action: "Si no compartes carpetas, desactiva el uso compartido de archivos e impresoras."),
        PortInfo(port: 143, name: "IMAP", risk: "low",
                 why: "Servidor de correo sin cifrar, algo raro en un equipo de casa.",
                 action: "Si no sabes qué programa lo abrió, revísalo o desactívalo."),
        PortInfo(port: 389, name: "LDAP", risk: "medium",
                 why: "Directorio de usuarios sin cifrar; en una casa casi nunca hace falta.",
                 action: "Si no administras un directorio de usuarios, desactívalo en ese equipo."),
        PortInfo(port: 443, name: "HTTPS", risk: "none"),
        PortInfo(port: 445, name: "SMB", risk: "medium",
                 why: "Comparte archivos de Windows y es el blanco favorito de los virus que se propagan por la red.",
                 action: "Si no compartes carpetas desde ese equipo, desactiva el uso compartido; si sí, ponles contraseña."),
        PortInfo(port: 465, name: "SMTP (cifrado)", risk: "none"),
        PortInfo(port: 515, name: "Impresora (LPD)", risk: "low",
                 why: "Cualquiera en tu red puede mandar trabajos de impresión.",
                 action: "Es normal en impresoras; si no imprimes por red, desactiva LPD en su panel."),
        PortInfo(port: 548, name: "AFP (Mac)", risk: "low",
                 why: "Comparte archivos de una Mac.",
                 action: "Usa contraseñas fuertes en las cuentas que comparten o apaga Compartir archivos si no lo usas."),
        PortInfo(port: 554, name: "RTSP", risk: "medium",
                 why: "Es el video de una cámara; muchas lo entregan sin contraseña a cualquiera en tu red.",
                 action: "En la configuración de la cámara, pon contraseña al video y cambia la de fábrica."),
        PortInfo(port: 587, name: "SMTP (envío)", risk: "none"),
        PortInfo(port: 631, name: "IPP (impresora)", risk: "low",
                 why: "Impresión y panel por red; algunas impresoras dejan cambiar su configuración sin contraseña.",
                 action: "Ponle contraseña de administrador a la impresora."),
        PortInfo(port: 636, name: "LDAP (cifrado)", risk: "none"),
        PortInfo(port: 873, name: "rsync", risk: "medium",
                 why: "Sincroniza carpetas; sin contraseña, cualquiera en tu red puede copiarlas.",
                 action: "Ponle usuario y contraseña al módulo de rsync o úsalo solo por SSH."),
        PortInfo(port: 993, name: "IMAP (cifrado)", risk: "none"),
        PortInfo(port: 995, name: "POP3 (cifrado)", risk: "none"),
        PortInfo(port: 1080, name: "Proxy SOCKS", risk: "medium",
                 why: "Es un proxy: deja que otros salgan a internet a través de ese equipo, y a veces lo abre un programa malicioso.",
                 action: "Si no instalaste un proxy en ese equipo, revisa qué programa abrió el puerto."),
        PortInfo(port: 1194, name: "OpenVPN", risk: "none"),
        PortInfo(port: 1433, name: "SQL Server", risk: "medium",
                 why: "Base de datos de Microsoft; con la contraseña de fábrica, cualquiera en tu red puede leerla.",
                 action: "Si solo la usa ese equipo, desactiva el acceso por red; si no, usa una contraseña fuerte."),
        PortInfo(port: 1723, name: "VPN PPTP", risk: "high",
                 why: "VPN antigua con un cifrado que ya se considera roto.",
                 action: "Cámbiala por WireGuard u OpenVPN."),
        PortInfo(port: 1883, name: "MQTT", risk: "medium",
                 why: "Mensajes de equipos inteligentes sin cifrar; sin contraseña, cualquiera puede leerlos o mandar órdenes.",
                 action: "Ponle usuario y contraseña al servidor MQTT, o usa el puerto cifrado (8883)."),
        PortInfo(port: 1900, name: "UPnP", risk: "none"),
        PortInfo(port: 2049, name: "NFS", risk: "medium",
                 why: "Carpetas compartidas de Linux o de un NAS, que suelen confiar en cualquier equipo de la red.",
                 action: "Limita las carpetas compartidas a las IP de tus equipos, o desactiva NFS si no lo usas."),
        PortInfo(port: 3000, name: "Panel web", risk: "low",
                 why: "Panel o servidor de desarrollo (Grafana, Node y similares).",
                 action: "Protégelo con contraseña o apágalo cuando no lo uses."),
        PortInfo(port: 3306, name: "MySQL", risk: "medium",
                 why: "Base de datos; con una contraseña débil, cualquiera en tu red puede leerla.",
                 action: "Si solo la usa ese equipo, haz que escuche solo en sí mismo (127.0.0.1)."),
        PortInfo(port: 3389, name: "RDP", risk: "medium",
                 why: "Escritorio remoto de Windows: con una contraseña débil, alguien en tu red puede tomar el control del equipo.",
                 action: "Si no lo usas, desactiva Escritorio remoto; si sí, usa una contraseña fuerte."),
        PortInfo(port: 3689, name: "iTunes/DAAP", risk: "none"),
        PortInfo(port: 5000, name: "UPnP/HTTP", risk: "low",
                 why: "Panel web o servicio UPnP, por ejemplo de un NAS o del receptor AirPlay de una Mac.",
                 action: "Si es un panel, protégelo con una contraseña fuerte."),
        PortInfo(port: 5001, name: "HTTPS alt. (NAS)", risk: "low",
                 why: "Panel web cifrado, típico de un NAS.",
                 action: "Protégelo con una contraseña fuerte y mantenlo actualizado."),
        PortInfo(port: 5040, name: "Windows (CDP)", risk: "none"),
        PortInfo(port: 5353, name: "mDNS", risk: "none"),
        PortInfo(port: 5357, name: "WSD (Windows)", risk: "none"),
        PortInfo(port: 5432, name: "PostgreSQL", risk: "medium",
                 why: "Base de datos; con una contraseña débil, cualquiera en tu red puede leerla.",
                 action: "Si solo la usa ese equipo, haz que escuche solo en sí mismo (127.0.0.1)."),
        PortInfo(port: 5555, name: "ADB inalámbrico", risk: "high",
                 why: "Depuración de Android abierta: da control total del teléfono o de la TV a quien se conecte.",
                 action: "Apaga la depuración en Opciones de desarrollador cuando no la uses."),
        PortInfo(port: 5900, name: "VNC", risk: "high",
                 why: "Control remoto de la pantalla; muchos lo dejan sin contraseña o con una muy corta.",
                 action: "Si no lo usas, apágalo (en Mac: Compartir pantalla); si sí, pon una contraseña fuerte."),
        PortInfo(port: 6379, name: "Redis", risk: "high",
                 why: "Base de datos que por defecto no pide contraseña: cualquiera en tu red puede leerla o borrarla.",
                 action: "Haz que escuche solo en ese equipo (127.0.0.1) o ponle contraseña."),
        PortInfo(port: 6668, name: "Tuya (control local)", risk: "none"),
        PortInfo(port: 7000, name: "AirPlay", risk: "none"),
        PortInfo(port: 7680, name: "Windows Update (P2P)", risk: "none"),
        PortInfo(port: 8000, name: "HTTP alt.", risk: "low",
                 why: "Panel web sin cifrar en un puerto alterno.",
                 action: "Cambia la contraseña de fábrica del equipo."),
        PortInfo(port: 8001, name: "Control de TV Samsung", risk: "low",
                 why: "Permite controlar la TV desde la red; la TV pide aprobar cada control nuevo.",
                 action: "Si la TV te pide aprobar un control que no reconoces, recházalo."),
        PortInfo(port: 8008, name: "Chromecast", risk: "none"),
        PortInfo(port: 8009, name: "Chromecast", risk: "none"),
        PortInfo(port: 8060, name: "Control de Roku", risk: "low",
                 why: "Cualquiera en tu red puede controlar el Roku sin contraseña.",
                 action: "En el Roku, en Configuración avanzada del sistema › Control por apps móviles, limita el acceso por red."),
        PortInfo(port: 8080, name: "HTTP alt.", risk: "low",
                 why: "Panel web sin cifrar en un puerto alterno, muy común en aplicaciones.",
                 action: "Protégelo con una contraseña fuerte o entra por HTTPS si lo permite."),
        PortInfo(port: 8081, name: "HTTP alt.", risk: "low",
                 why: "Panel web sin cifrar en un puerto alterno.",
                 action: "Protégelo con una contraseña fuerte o entra por HTTPS si lo permite."),
        PortInfo(port: 8123, name: "Home Assistant", risk: "low",
                 why: "Panel de Home Assistant; pide usuario y contraseña.",
                 action: "Usa una contraseña fuerte y mantenlo actualizado."),
        PortInfo(port: 8443, name: "HTTPS alt.", risk: "none"),
        PortInfo(port: 8554, name: "RTSP alt.", risk: "medium",
                 why: "Es el video de una cámara; muchas lo entregan sin contraseña a cualquiera en tu red.",
                 action: "En la configuración de la cámara, pon contraseña al video y cambia la de fábrica."),
        PortInfo(port: 8883, name: "MQTT (cifrado)", risk: "none"),
        PortInfo(port: 8888, name: "HTTP alt.", risk: "low",
                 why: "Panel web sin cifrar, común en herramientas de desarrollo.",
                 action: "Protégelo con contraseña o apágalo cuando no lo uses."),
        PortInfo(port: 8899, name: "ONVIF", risk: "medium",
                 why: "Control de cámaras (moverlas, configurarlas); muchas lo aceptan con la contraseña de fábrica.",
                 action: "Cambia la contraseña de fábrica de la cámara y desactiva ONVIF si ningún programa lo usa."),
        PortInfo(port: 9000, name: "HTTP alt.", risk: "low",
                 why: "Panel web en un puerto alterno, muy común en aplicaciones.",
                 action: "Protégelo con una contraseña fuerte."),
        PortInfo(port: 9090, name: "HTTP alt.", risk: "low",
                 why: "Panel web en un puerto alterno, usado por herramientas de monitoreo.",
                 action: "Protégelo con una contraseña fuerte."),
        PortInfo(port: 9100, name: "Impresora (RAW)", risk: "low",
                 why: "Cualquiera en tu red puede mandar a imprimir.",
                 action: "Es normal en impresoras de red; si no imprimes por red, desactívalo en su panel."),
        PortInfo(port: 9200, name: "Elasticsearch", risk: "medium",
                 why: "Buscador de datos que en versiones viejas no pide contraseña.",
                 action: "Actualízalo y activa su seguridad (usuario y contraseña)."),
        PortInfo(port: 9443, name: "HTTPS alt.", risk: "none"),
        PortInfo(port: 10000, name: "Webmin", risk: "medium",
                 why: "Panel que administra todo el servidor.",
                 action: "Si no lo usas, desinstálalo; si sí, contraseña fuerte y siempre actualizado."),
        PortInfo(port: 27017, name: "MongoDB", risk: "high",
                 why: "Base de datos que en versiones viejas no pide contraseña: cualquiera en tu red puede leerla o borrarla.",
                 action: "Haz que escuche solo en ese equipo (127.0.0.1) o activa su autenticación."),
        PortInfo(port: 32400, name: "Plex", risk: "none"),
        PortInfo(port: 49152, name: "UPnP", risk: "none"),
        PortInfo(port: 49153, name: "UPnP", risk: "none"),
        PortInfo(port: 49154, name: "UPnP", risk: "none"),
        PortInfo(port: 49155, name: "UPnP", risk: "none"),
        PortInfo(port: 62078, name: "Sincronización iPhone", risk: "none")
    ]
}

private enum PureOuiLookup {
    static let prefixes: [String: (vendor: String, brand: String)] = [
        "00:17:88": ("Philips Lighting", "Philips Hue"),
        "B8:27:EB": ("Raspberry Pi Foundation", "Raspberry Pi"),
        "DC:A6:32": ("Raspberry Pi Trading", "Raspberry Pi"),
        "E4:5F:01": ("Raspberry Pi Trading", "Raspberry Pi"),
        "2C:CF:67": ("Raspberry Pi Ltd", "Raspberry Pi"),
        "3C:22:FB": ("Apple, Inc.", "Apple"),
        "A4:83:E7": ("Apple, Inc.", "Apple"),
        "F0:18:98": ("Apple, Inc.", "Apple"),
        "28:CF:E9": ("Apple, Inc.", "Apple"),
        "B4:2E:99": ("Giga-Byte Technology", "Gigabyte"),
        "18:B4:30": ("Nest Labs", "Google Nest"),
        "54:60:09": ("Google, Inc.", "Google"),
        "F4:F5:D8": ("Google, Inc.", "Google"),
        "30:FD:38": ("Google, Inc.", "Google"),
        "50:C7:BF": ("TP-Link Systems", "TP-Link"),
        "B0:BE:76": ("TP-Link Corporation", "TP-Link"),
        "24:6F:28": ("Espressif Inc.", "Espressif"),
        "A4:CF:12": ("Espressif Inc.", "Espressif"),
        "84:CC:A8": ("Espressif Inc.", "Espressif"),
        "EC:FA:BC": ("Espressif Inc.", "Espressif"),
        "00:11:32": ("Synology Incorporated", "Synology"),
        "24:5E:BE": ("QNAP Systems", "QNAP"),
        "78:45:58": ("Ubiquiti Inc", "Ubiquiti"),
        "24:5A:4C": ("Ubiquiti Inc", "Ubiquiti"),
        "FC:EC:DA": ("Ubiquiti Inc", "Ubiquiti"),
        "68:54:FD": ("Amazon Technologies", "Amazon"),
        "F0:81:73": ("Amazon Technologies", "Amazon"),
        "C8:3A:6B": ("Roku, Inc", "Roku"),
        "B8:3E:59": ("Roku, Inc", "Roku"),
        "64:16:66": ("Nest Labs", "Nest")
    ]

    private static func normalizedPrefix(_ mac: String) -> String? {
        let hex = mac.uppercased().filter { $0.isHexDigit }
        guard hex.count >= 6 else { return nil }
        let idx0 = hex.startIndex
        let a = hex[idx0..<hex.index(idx0, offsetBy: 2)]
        let b = hex[hex.index(idx0, offsetBy: 2)..<hex.index(idx0, offsetBy: 4)]
        let c = hex[hex.index(idx0, offsetBy: 4)..<hex.index(idx0, offsetBy: 6)]
        return "\(a):\(b):\(c)"
    }

    static func vendor(forMAC mac: String) -> String? {
        guard let prefix = normalizedPrefix(mac) else { return nil }
        return prefixes[prefix]?.vendor
    }

    static func brand(forMAC mac: String) -> String? {
        guard let prefix = normalizedPrefix(mac) else { return nil }
        return prefixes[prefix]?.brand
    }
}

private enum PureSecurityReportBuilder {
    static func build(input: SecurityInput, portLookup: (Int) -> PortInfo?) -> SecurityReport {
        var findings: [SecurityFinding] = []
        let ackMap: [String: AcknowledgedFinding] = Dictionary(
            input.acknowledged.map { ($0.findingId, $0) },
            uniquingKeysWith: { first, _ in first }
        )

        func makeFinding(
            id: String,
            kind: String,
            severity: FindingSeverity,
            points: Int,
            title: String,
            detail: String,
            action: String,
            deviceId: Int64? = nil,
            ip: String? = nil,
            port: Int? = nil
        ) -> SecurityFinding {
            let ack = ackMap[id]
            return SecurityFinding(
                id: id,
                kind: kind,
                severity: severity.rawValue,
                points: points,
                title: title,
                detail: detail,
                action: action,
                deviceId: deviceId,
                ip: ip,
                port: port,
                acknowledged: ack != nil,
                acknowledgedAt: ack?.at
            )
        }

        // 1. Hallazgos Wi-Fi
        if let wifi = input.wifi {
            switch wifi.security.lowercased() {
            case "open":
                let ssidLabel = wifi.ssid.isEmpty ? VigiaL10n.tr("actual") : wifi.ssid
                findings.append(makeFinding(
                    id: "WIFI_OPEN",
                    kind: "WIFI_OPEN",
                    severity: .high,
                    points: 30,
                    title: VigiaL10n.tr("Red Wi-Fi abierta (sin contraseña)"),
                    detail: VigiaL10n.fmt("La red %@ no cifra el tráfico.", ssidLabel),
                    action: VigiaL10n.tr("Activa cifrado WPA2 o WPA3 en el router cuanto antes.")
                ))
            case "wep":
                findings.append(makeFinding(
                    id: "WIFI_WEP",
                    kind: "WIFI_WEP",
                    severity: .high,
                    points: 25,
                    title: VigiaL10n.tr("Red Wi-Fi con cifrado WEP obsoleto"),
                    detail: VigiaL10n.tr("WEP puede descifrarse en minutos desde cualquier equipo cercano."),
                    action: VigiaL10n.tr("Cambia la seguridad del router a WPA2 o WPA3.")
                ))
            case "wpa2":
                findings.append(makeFinding(
                    id: "WIFI_WPA2",
                    kind: "WIFI_WPA2",
                    severity: .info,
                    points: 0,
                    title: VigiaL10n.tr("Red Wi-Fi protegida con WPA2"),
                    detail: VigiaL10n.tr("Es segura con una contraseña fuerte; si tu router ofrece WPA3, es recomendable."),
                    action: VigiaL10n.tr("Mantén una contraseña larga y única.")
                ))
            default:
                break
            }
            if wifi.wps {
                findings.append(makeFinding(
                    id: "WIFI_WPS",
                    kind: "WIFI_WPS",
                    severity: .medium,
                    points: 10,
                    title: VigiaL10n.tr("WPS activo en la red Wi-Fi"),
                    detail: VigiaL10n.tr("El PIN de WPS puede forzarse sin conocer la contraseña principal."),
                    action: VigiaL10n.tr("Desactiva WPS en el panel de tu router.")
                ))
            }
        }

        // 2. Router / UPnP / Port Mappings
        if let router = input.router {
            if router.upnp {
                let hasMappings = !router.portMappings.isEmpty
                findings.append(makeFinding(
                    id: "UPNP_ENABLED",
                    kind: "UPNP_ENABLED",
                    severity: hasMappings ? .medium : .low,
                    points: hasMappings ? 10 : 5,
                    title: VigiaL10n.tr("UPnP activo en el router"),
                    detail: hasMappings
                        ? VigiaL10n.fmt("Hay %ld puerto(s) abiertos automáticamente hacia la red local.", router.portMappings.count)
                        : VigiaL10n.tr("Cualquier aplicación en tu red puede abrir puertos en el router sin pedir permiso."),
                    action: VigiaL10n.tr("Desactiva UPnP en el router si no lo necesitas para juegos o servicios locales."),
                    ip: router.ip
                ))
            }
            for mapping in router.portMappings {
                let reach = RouterReach.fromRaw(router.reach)
                let exposed = reach == .internet || reach == .outerRing
                let mappingDesc = mapping.description.isEmpty ? mapping.source.uppercased() : mapping.description
                findings.append(makeFinding(
                    id: "PORT_MAPPING|\(mapping.transportProtocol)|\(mapping.externalPort)|\(mapping.internalIp)",
                    kind: "PORT_MAPPING",
                    severity: exposed ? .medium : .low,
                    points: exposed ? 8 : 3,
                    title: VigiaL10n.fmt("Puerto %ld/%@ redirigido", mapping.externalPort, mapping.transportProtocol),
                    detail: VigiaL10n.fmt("Apunta a %@:%ld (%@).", mapping.internalIp, mapping.internalPort, mappingDesc),
                    action: VigiaL10n.tr("Si no reconoces esta regla, elimínala de la tabla de puertos del router."),
                    deviceId: mapping.deviceId,
                    ip: mapping.internalIp,
                    port: mapping.externalPort
                ))
            }
            if !router.upnp && !router.natpmp && router.portMappings.isEmpty {
                findings.append(makeFinding(
                    id: "NO_INBOUND",
                    kind: "NO_INBOUND",
                    severity: .info,
                    points: 0,
                    title: VigiaL10n.tr("Sin puertos entrantes abiertos"),
                    detail: VigiaL10n.tr("No se detectaron redirecciones UPnP ni NAT-PMP activas en el router."),
                    action: VigiaL10n.tr("Todo bien en este apartado."),
                    ip: router.ip
                ))
            }
        }

        // 3. Puertos riesgosos en equipos y desconocidos
        var unknownCount = 0
        for sd in input.devices {
            if DeviceType.fromRaw(sd.device.identity.type) == .unknown {
                unknownCount += 1
            }
            for port in sd.device.openPorts {
                guard let info = portLookup(port), info.isRisky else { continue }
                let high = info.risk == "high"
                let label = sd.name.isEmpty ? sd.device.displayName : sd.name
                findings.append(makeFinding(
                    id: "RISKY_SERVICE|\(sd.device.ip)|\(port)",
                    kind: "RISKY_SERVICE",
                    severity: high ? .high : .medium,
                    points: high ? 15 : 8,
                    title: VigiaL10n.fmt("%@ abierto en %@ (:%ld)", info.name, label, port),
                    detail: info.why,
                    action: info.action,
                    deviceId: sd.id,
                    ip: sd.device.ip,
                    port: port
                ))
            }
        }
        if unknownCount > 0 {
            findings.append(makeFinding(
                id: "UNKNOWN_DEVICES",
                kind: "UNKNOWN_DEVICES",
                severity: .low,
                points: min(8, unknownCount * 2),
                title: VigiaL10n.fmt("%ld equipo(s) sin identificar", unknownCount),
                detail: VigiaL10n.tr("Hay equipos en la red cuya marca o rol aún no están identificados."),
                action: VigiaL10n.tr("Revisa el listado de Equipos y asígnales un nombre o tipo.")
            ))
        }

        // 4. Alertas críticas recientes (24 h)
        let cutoff = Date().addingTimeInterval(-86_400)
        for alert in input.recentAlerts {
            guard let at = VigiaDate.parse(alert.at), at >= cutoff else { continue }
            if alert.type == AlertType.routerChanged.rawValue {
                findings.append(makeFinding(
                    id: "ROUTER_CHANGED|\(alert.at)",
                    kind: "ROUTER_CHANGED",
                    severity: .high,
                    points: 30,
                    title: VigiaL10n.tr("Cambio reciente de router"),
                    detail: VigiaL10n.tr("Se detectó un cambio de gateway en las últimas 24 horas."),
                    action: VigiaL10n.tr("Confirma que cambiaste de router o proveedor intencionalmente.")
                ))
            } else if alert.type == AlertType.arpConflict.rawValue {
                findings.append(makeFinding(
                    id: "ARP_CONFLICT|\(alert.at)",
                    kind: "ARP_CONFLICT",
                    severity: .high,
                    points: 30,
                    title: VigiaL10n.tr("Conflicto ARP reciente"),
                    detail: VigiaL10n.tr("Dos equipos anunciaron la misma IP en las últimas 24 horas."),
                    action: VigiaL10n.tr("Revisa si algún equipo sospechoso o repetidor mal configurado causó el conflicto.")
                ))
            }
        }

        // 5. Anillo exterior
        if let outer = input.outerRing, !outer.devices.isEmpty {
            findings.append(makeFinding(
                id: "OUTER_RING_DEVICES",
                kind: "OUTER_RING_DEVICES",
                severity: .info,
                points: 0,
                title: VigiaL10n.fmt("%ld equipo(s) delante del router", outer.devices.count),
                detail: VigiaL10n.fmt("Detectados en la subred exterior %@.", outer.subnet),
                action: VigiaL10n.tr("Verifica que solo el módem/ONT y tu router compartan ese enlace.")
            ))
        }

        findings.sort { lhs, rhs in
            if lhs.acknowledged != rhs.acknowledged {
                return !lhs.acknowledged
            }
            if lhs.severityLevel != rhs.severityLevel {
                return lhs.severityLevel < rhs.severityLevel
            }
            if lhs.points != rhs.points {
                return lhs.points > rhs.points
            }
            return lhs.id < rhs.id
        }

        let deducted = findings.filter { !$0.acknowledged }.reduce(0) { $0 + $1.points }
        return SecurityReport(
            generatedAt: VigiaDate.nowString(),
            score: max(0, 100 - deducted),
            findings: findings,
            router: input.router,
            outerRing: input.outerRing
        )
    }
}

private enum PureSwiftScanner {
    static let discoveryPorts: [Int] = [80, 443, 22, 445, 548, 554, 631, 3389, 5000, 7000, 8008, 8080, 8123, 62078]

    static func parsePortSpec(_ spec: String) -> [Int] {
        var out: [Int] = []
        for token in spec.split(separator: ",") {
            let trimmed = token.trimmingCharacters(in: .whitespaces)
            if trimmed.contains("-") {
                let parts = trimmed.split(separator: "-").compactMap { Int($0) }
                if parts.count == 2, parts[0] <= parts[1] {
                    let lo = max(1, parts[0])
                    let hi = min(65535, parts[1])
                    if hi - lo <= 1024 {
                        out.append(contentsOf: lo...hi)
                    }
                }
            } else if let p = Int(trimmed), (1...65535).contains(p) {
                out.append(p)
            }
        }
        return Array(Set(out)).sorted()
    }

    static func scan(
        cidr: String,
        gateway: String,
        selfIP: String,
        withPorts: Bool,
        bindIface: String,
        isCancelled: @escaping @Sendable () -> Bool,
        onProgress: @escaping @Sendable (_ stage: String, _ done: Int, _ total: Int) -> Void
    ) async throws -> ScanResult {
        let start = DispatchTime.now().uptimeNanoseconds
        let ips = hostsForCIDR(cidr, gateway: gateway, selfIP: selfIP)
        let total = max(1, ips.count)
        onProgress(VigiaL10n.tr("hosts"), 0, total)

        let batchSize = 24
        var discovered: [Device] = []
        var completed = 0

        for startIdx in stride(from: 0, to: ips.count, by: batchSize) {
            if isCancelled() {
                throw VigiaEngineError.cancelled
            }
            let endIdx = min(ips.count, startIdx + batchSize)
            let chunk = Array(ips[startIdx..<endIdx])
            let chunkDevices = await withTaskGroup(of: Device?.self) { group in
                for ip in chunk {
                    group.addTask {
                        if ip == selfIP {
                            return Device(
                                ip: ip,
                                hostname: ProcessInfo.processInfo.hostName,
                                identity: Identity(type: "phone", brand: "Apple", model: "iPhone", os: "iOS", confidence: 90, reason: VigiaL10n.tr("Este iPhone")),
                                sources: ["self"],
                                lastSeen: VigiaDate.nowString()
                            )
                        }
                        let probePorts = withPorts ? discoveryPorts : [80, 443, 22, 53]
                        let open = await probeHostPorts(ip: ip, ports: probePorts, timeoutMs: 260, bindIface: bindIface)
                        let isGateway = (ip == gateway)
                        if open.isEmpty && !isGateway {
                            return nil
                        }
                        return classifyDevice(ip: ip, openPorts: open, isGateway: isGateway)
                    }
                }
                var out: [Device] = []
                for await dev in group {
                    if let dev { out.append(dev) }
                }
                return out
            }
            discovered.append(contentsOf: chunkDevices)
            completed += chunk.count
            onProgress(withPorts ? VigiaL10n.tr("puertos") : VigiaL10n.tr("hosts"), completed, total)
        }

        discovered.sort { ipNumeric($0.ip) < ipNumeric($1.ip) }
        let elapsed = Int64(DispatchTime.now().uptimeNanoseconds - start)
        return ScanResult(
            subnet: cidr,
            selfIP: selfIP,
            gateway: gateway.isEmpty ? nil : gateway,
            devices: discovered,
            startedAt: VigiaDate.nowString(),
            duration: elapsed
        )
    }

    static func scanPorts(ip: String, ports: [Int], bindIface: String = "") async -> [Int] {
        var open: [Int] = []
        let batchSize = 24
        for startIdx in stride(from: 0, to: ports.count, by: batchSize) {
            let chunk = Array(ports[startIdx..<min(ports.count, startIdx + batchSize)])
            let hits = await probeHostPorts(ip: ip, ports: chunk, timeoutMs: 320, bindIface: bindIface)
            open.append(contentsOf: hits)
        }
        return Array(Set(open)).sorted()
    }

    static func probeRouter(gateway: String, bindIface: String = "") async -> RouterInfo {
        let open = await probeHostPorts(ip: gateway, ports: [80, 443, 1900, 5000, 5351], timeoutMs: 450, bindIface: bindIface)
        return RouterInfo(
            ip: gateway,
            mac: nil,
            upnp: open.contains(1900) || open.contains(5000),
            natpmp: open.contains(5351),
            externalIp: "",
            reach: "unknown",
            portMappings: []
        )
    }

    private static func classifyDevice(ip: String, openPorts: [Int], isGateway: Bool) -> Device {
        let type: DeviceType
        var brand = ""
        var reason = ""
        if isGateway {
            type = .router
            reason = VigiaL10n.tr("Gateway de la subred")
        } else if openPorts.contains(62078) {
            type = .phone
            brand = "Apple"
            reason = VigiaL10n.tr("Puerto 62078 (sincronización Apple)")
        } else if openPorts.contains(631) {
            type = .printer
            reason = VigiaL10n.tr("Puerto IPP 631")
        } else if openPorts.contains(8008) || openPorts.contains(7000) || openPorts.contains(8060) {
            type = .tv
            reason = VigiaL10n.tr("Servicio de streaming local")
        } else if openPorts.contains(554) || openPorts.contains(8554) || openPorts.contains(8899) {
            type = .camera
            reason = VigiaL10n.tr("Transmisión RTSP/ONVIF")
        } else if openPorts.contains(445) || openPorts.contains(548) || openPorts.contains(22) || openPorts.contains(3389) {
            type = .computer
            reason = VigiaL10n.tr("Servicios de sistema/archivos")
        } else if openPorts.contains(8123) || openPorts.contains(1883) {
            type = .iot
            reason = VigiaL10n.tr("Control inteligente local")
        } else {
            type = .unknown
            reason = openPorts.isEmpty
                ? VigiaL10n.tr("Host activo")
                : VigiaL10n.fmt("Puerto(s) TCP: %@", openPorts.map(String.init).joined(separator: ", "))
        }
        return Device(
            ip: ip,
            identity: Identity(
                type: type.rawValue,
                brand: brand,
                confidence: type == .unknown ? 25 : 65,
                reason: reason
            ),
            openPorts: openPorts,
            sources: isGateway ? ["gateway", "tcp"] : ["tcp"],
            lastSeen: VigiaDate.nowString()
        )
    }

    private static func probeHostPorts(ip: String, ports: [Int], timeoutMs: Int, bindIface: String) async -> [Int] {
        await withTaskGroup(of: Int?.self) { group in
            for port in ports {
                group.addTask {
                    tcpConnect(ip: ip, port: port, timeoutMs: timeoutMs, bindIface: bindIface) ? port : nil
                }
            }
            var hits: [Int] = []
            for await hit in group {
                if let hit { hits.append(hit) }
            }
            return hits.sorted()
        }
    }

    private static func tcpConnect(ip: String, port: Int, timeoutMs: Int, bindIface: String = "") -> Bool {
        let fd = Darwin.socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { return false }
        defer { Darwin.close(fd) }

        if !bindIface.isEmpty {
            var ifIndex = Darwin.if_nametoindex(bindIface)
            if ifIndex > 0 {
                // IP_BOUND_IF (25 en Darwin/iOS) evita que una VPN/Tailscale secuestre el tráfico hacia la LAN local.
                _ = Darwin.setsockopt(fd, IPPROTO_IP, 25, &ifIndex, socklen_t(MemoryLayout<UInt32>.size))
            }
        }

        let flags = Darwin.fcntl(fd, F_GETFL, 0)
        _ = Darwin.fcntl(fd, F_SETFL, flags | O_NONBLOCK)

        var addr = sockaddr_in()
        addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        addr.sin_family = sa_family_t(AF_INET)
        addr.sin_port = UInt16(port).bigEndian
        guard Darwin.inet_pton(AF_INET, ip, &addr.sin_addr) == 1 else {
            return false
        }

        let res = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        if res == 0 { return true }
        guard errno == EINPROGRESS else { return false }

        var pfd = pollfd(fd: fd, events: Int16(POLLOUT), revents: 0)
        let ready = Darwin.poll(&pfd, 1, Int32(timeoutMs))
        guard ready > 0 else { return false }

        var sockErr: Int32 = 0
        var len = socklen_t(MemoryLayout<Int32>.size)
        if Darwin.getsockopt(fd, SOL_SOCKET, SO_ERROR, &sockErr, &len) == 0 && sockErr == 0 {
            return true
        }
        return false
    }

    private static func hostsForCIDR(_ cidr: String, gateway: String, selfIP: String) -> [String] {
        let parts = cidr.split(separator: "/")
        let baseIP = String(parts.first ?? "")
        let octets = baseIP.split(separator: ".").compactMap { Int($0) }
        guard octets.count == 4 else {
            var fallback: [String] = []
            if !gateway.isEmpty { fallback.append(gateway) }
            if !selfIP.isEmpty && selfIP != gateway { fallback.append(selfIP) }
            return fallback
        }
        let prefix = "\(octets[0]).\(octets[1]).\(octets[2])"
        var list = (1...254).map { "\(prefix).\($0)" }
        if !gateway.isEmpty && !list.contains(gateway) {
            list.insert(gateway, at: 0)
        }
        return list
    }

    private static func ipNumeric(_ ip: String) -> UInt32 {
        ip.split(separator: ".").reduce(0) { acc, part in
            (acc << 8) | (UInt32(part) ?? 0)
        }
    }
}
