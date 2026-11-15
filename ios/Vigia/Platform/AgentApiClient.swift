import Foundation
import Combine
import CryptoKit
import Network
import Security

/// Errores de transporte, TLS o de negocio de la API v1.5 del Agente Vigía.
public enum AgentApiError: LocalizedError, Sendable {
    case notLinked
    case invalidURL(String)
    case httpError(statusCode: Int, message: String)
    case certPinMismatch(expected: String, actual: String)
    case tofuVerificationFailed(handshakeSha256: String, bodySha256: String)
    case transportError(String)
    case invalidResponse

    public var errorDescription: String? {
        switch self {
        case .notLinked:
            return VigiaL10n.tr("No hay un agente 24/7 vinculado.")
        case .invalidURL(let url):
            return VigiaL10n.fmt("Dirección de agente inválida: %@", url)
        case .httpError(let code, let message):
            return message.isEmpty ? VigiaL10n.fmt("Error HTTP %ld del agente", code) : message
        case .certPinMismatch(let expected, let actual):
            let expPrefix = String(expected.prefix(12))
            let actPrefix = String(actual.prefix(12))
            return VigiaL10n.fmt("El certificado TLS del agente no coincide con el fijado (esperado %@…, recibido %@…).", expPrefix, actPrefix)
        case .tofuVerificationFailed:
            return VigiaL10n.tr("La huella del certificado TLS recibido no coincide con la respuesta del emparejamiento.")
        case .transportError(let msg):
            return msg
        case .invalidResponse:
            return VigiaL10n.tr("Respuesta inesperada del agente.")
        }
    }
}

/// Calcula direcciones candidatas para alcanzar al agente dentro o fuera de casa (LAN + Tailscale).
public enum AgentReachability {
    public static func candidateBaseUrls(link: AgentLink, savedAddresses: [String]) -> [String] {
        var candidates: [String] = [httpsBaseUrl(host: link.host, port: link.port)]
        let others = savedAddresses
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty && $0 != link.host && isLanOrTailscale($0) }
            .sorted { lhs, rhs in
                let lt = isTailscale(lhs) ? 0 : 1
                let rt = isTailscale(rhs) ? 0 : 1
                return lt < rt
            }
            .map { httpsBaseUrl(host: $0, port: link.port) }

        for url in others where !candidates.contains(url) {
            candidates.append(url)
        }
        return candidates
    }

    public static func httpsBaseUrl(host: String, port: Int) -> String {
        let trimmed = host.trimmingCharacters(in: .whitespacesAndNewlines)
        let formattedHost = trimmed.contains(":") && !trimmed.hasPrefix("[") ? "[\(trimmed)]" : trimmed
        return "https://\(formattedHost):\(port)"
    }

    public static func isLanOrTailscale(_ ip: String) -> Bool {
        guard let o = ipv4Octets(ip) else { return false }
        return o[0] == 10
            || (o[0] == 172 && (16...31).contains(o[1]))
            || (o[0] == 192 && o[1] == 168)
            || isTailscale(ip)
    }

    /// Rango CGNAT 100.64.0.0/10 utilizado por Tailscale.
    public static func isTailscale(_ ip: String) -> Bool {
        guard let o = ipv4Octets(ip) else { return false }
        return o[0] == 100 && (64...127).contains(o[1])
    }

    private static func ipv4Octets(_ ip: String) -> [Int]? {
        let parts = ip.split(separator: ".").compactMap { Int($0) }
        guard parts.count == 4, parts.allSatisfy({ (0...255).contains($0) }) else { return nil }
        return parts
    }
}

/// Delegado TLS que valida el pin SHA-256 del certificado hoja (sin requerir CA pública ni SAN)
/// y captura la huella real durante el emparejamiento TOFU.
public final class AgentTlsPinningDelegate: NSObject, URLSessionDelegate, URLSessionDataDelegate, @unchecked Sendable {
    public enum Mode: Sendable {
        case pinned(expectedSha256: String)
        case tofu(optionalExpectedSha256: String?)
    }

    private let mode: Mode
    private let lock = NSLock()
    private var capturedLeafSha256: String?
    private var lastPinMismatch: (expected: String, actual: String)?

    public init(mode: Mode) {
        self.mode = mode
    }

    public var handshakeLeafSha256: String? {
        lock.lock()
        defer { lock.unlock() }
        return capturedLeafSha256
    }

    public var pinMismatchError: AgentApiError? {
        lock.lock()
        defer { lock.unlock() }
        guard let mismatch = lastPinMismatch else { return nil }
        return .certPinMismatch(expected: mismatch.expected, actual: mismatch.actual)
    }

    public func urlSession(
        _ session: URLSession,
        didReceive challenge: URLAuthenticationChallenge,
        completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
    ) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let trust = challenge.protectionSpace.serverTrust,
              let leaf = Self.leafCertificate(from: trust) else {
            completionHandler(.performDefaultHandling, nil)
            return
        }

        let actualPin = Self.sha256Hex(of: leaf)
        lock.lock()
        capturedLeafSha256 = actualPin
        lock.unlock()

        switch mode {
        case .pinned(let expectedRaw):
            let expected = expectedRaw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if !expected.isEmpty && expected == actualPin {
                completionHandler(.useCredential, URLCredential(trust: trust))
            } else {
                lock.lock()
                lastPinMismatch = (expected: expected, actual: actualPin)
                lock.unlock()
                completionHandler(.cancelAuthenticationChallenge, nil)
            }

        case .tofu(let optionalPin):
            let expected = optionalPin?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
            if !expected.isEmpty && expected != actualPin {
                lock.lock()
                lastPinMismatch = (expected: expected, actual: actualPin)
                lock.unlock()
                completionHandler(.cancelAuthenticationChallenge, nil)
                return
            }
            completionHandler(.useCredential, URLCredential(trust: trust))
        }
    }

    public static func leafCertificate(from trust: SecTrust) -> SecCertificate? {
        if let chain = SecTrustCopyCertificateChain(trust) as? [SecCertificate], let first = chain.first {
            return first
        }
        return nil
    }

    public static func sha256Hex(of certificate: SecCertificate) -> String {
        let data = SecCertificateCopyData(certificate) as Data
        let digest = SHA256.hash(data: data)
        return digest.map { String(format: "%02x", $0) }.joined()
    }
}

/// Resultado de inspección TOFU previa al emparejamiento (`GET /api/v1/info`).
public struct AgentInfoProbe: Sendable {
    public var info: AgentInfoDto
    public var handshakeCertSha256: String
    public var host: String
    public var port: Int
}

/// Cliente HTTPS v1.5 para comunicarse con el Agente Vigía 24/7.
public final class AgentApiClient: @unchecked Sendable {
    public let baseUrl: String
    private let token: String?
    private let delegate: AgentTlsPinningDelegate
    private let session: URLSession
    private let streamSession: URLSession

    public init(baseUrl: String, token: String?, certSha256: String?) {
        self.baseUrl = baseUrl.hasSuffix("/") ? String(baseUrl.dropLast()) : baseUrl
        self.token = token
        let normalizedPin = certSha256?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        let mode: AgentTlsPinningDelegate.Mode = normalizedPin.isEmpty
            ? .tofu(optionalExpectedSha256: nil)
            : .pinned(expectedSha256: normalizedPin)
        self.delegate = AgentTlsPinningDelegate(mode: mode)

        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 10
        cfg.timeoutIntervalForResource = 25
        cfg.waitsForConnectivity = false
        self.session = URLSession(configuration: cfg, delegate: self.delegate, delegateQueue: nil)

        let sseCfg = URLSessionConfiguration.ephemeral
        sseCfg.timeoutIntervalForRequest = 75
        sseCfg.timeoutIntervalForResource = 60 * 60 * 24
        self.streamSession = URLSession(configuration: sseCfg, delegate: self.delegate, delegateQueue: nil)
    }

    public convenience init(link: AgentLink) {
        self.init(
            baseUrl: link.baseUrl,
            token: link.token,
            certSha256: link.certSha256
        )
    }

    deinit {
        session.invalidateAndCancel()
        streamSession.invalidateAndCancel()
    }

    /// Construye un cliente usando la primera URL candidata que responda a `/api/v1/info`
    /// con el pin TLS fijado en `link`.
    public static func connectReachable(
        link: AgentLink,
        savedAddresses: [String] = AgentPreferencesStore.savedAddresses
    ) async -> AgentApiClient {
        let candidates = AgentReachability.candidateBaseUrls(link: link, savedAddresses: savedAddresses)
        for url in candidates {
            let candidate = AgentApiClient(baseUrl: url, token: link.token, certSha256: link.certSha256)
            if (try? await candidate.info()) != nil {
                return candidate
            }
        }
        return AgentApiClient(link: link)
    }

    // MARK: - Emparejamiento TOFU (sin token previo)

    public static func probeInfo(host: String, port: Int, expectedSha256: String? = nil) async throws -> AgentInfoProbe {
        let baseUrl = AgentReachability.httpsBaseUrl(host: host, port: port)
        let delegate = AgentTlsPinningDelegate(mode: .tofu(optionalExpectedSha256: expectedSha256))
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 8
        let session = URLSession(configuration: cfg, delegate: delegate, delegateQueue: nil)
        defer { session.invalidateAndCancel() }

        guard let url = URL(string: "\(baseUrl)/api/v1/info") else {
            throw AgentApiError.invalidURL(baseUrl)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"

        let (data, response) = try await perform(session: session, delegate: delegate, request: request)
        try validateHttp(response: response, data: data)
        let info = try VigiaJSON.makeDecoder().decode(AgentInfoDto.self, from: data)
        let handshakeSha = delegate.handshakeLeafSha256 ?? ""
        if !info.certSha256.isEmpty && !handshakeSha.isEmpty && info.certSha256.lowercased() != handshakeSha.lowercased() {
            throw AgentApiError.tofuVerificationFailed(handshakeSha256: handshakeSha, bodySha256: info.certSha256)
        }
        let finalSha = handshakeSha.isEmpty ? info.certSha256.lowercased() : handshakeSha.lowercased()
        return AgentInfoProbe(info: info, handshakeCertSha256: finalSha, host: host, port: port)
    }

    public static func pair(
        host: String,
        port: Int,
        code: String,
        clientName: String,
        expectedSha256: String? = nil
    ) async throws -> AgentLink {
        let baseUrl = AgentReachability.httpsBaseUrl(host: host, port: port)
        let delegate = AgentTlsPinningDelegate(mode: .tofu(optionalExpectedSha256: expectedSha256))
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 12
        let session = URLSession(configuration: cfg, delegate: delegate, delegateQueue: nil)
        defer { session.invalidateAndCancel() }

        guard let url = URL(string: "\(baseUrl)/api/v1/pair") else {
            throw AgentApiError.invalidURL(baseUrl)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try VigiaJSON.makeEncoder().encode(PairRequest(code: code, client: clientName))

        let (data, response) = try await perform(session: session, delegate: delegate, request: request)
        try validateHttp(response: response, data: data)

        let pairRes = try VigiaJSON.makeDecoder().decode(PairResponseDto.self, from: data)
        let handshakeSha = (delegate.handshakeLeafSha256 ?? "").lowercased()
        let bodySha = pairRes.certSha256.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()

        // Verificación TOFU: si ambos están presentes, el cert del handshake TLS y el del JSON deben coincidir.
        if !handshakeSha.isEmpty && !bodySha.isEmpty && handshakeSha != bodySha {
            throw AgentApiError.tofuVerificationFailed(handshakeSha256: handshakeSha, bodySha256: bodySha)
        }
        let verifiedPin = !bodySha.isEmpty ? bodySha : handshakeSha
        return AgentLink(
            agentId: pairRes.agentId,
            agentName: pairRes.agentName,
            host: host,
            port: port,
            token: pairRes.token,
            certSha256: verifiedPin
        )
    }

    // MARK: - Endpoints REST v1.5

    public func info() async throws -> AgentInfoDto {
        try await requestJSON(method: "GET", path: "/api/v1/info", authenticated: false)
    }

    public func networks() async throws -> [AgentNetworkDto] {
        try await requestJSON(method: "GET", path: "/api/v1/networks")
    }

    public func devices(networkKey: String) async throws -> [KnownDevice] {
        let encoded = Self.encodePathSegment(networkKey)
        return try await requestJSON(method: "GET", path: "/api/v1/networks/\(encoded)/devices")
    }

    public func device(id: Int64) async throws -> KnownDevice {
        try await requestJSON(method: "GET", path: "/api/v1/devices/\(id)")
    }

    public func events(deviceId: Int64, limit: Int = 200) async throws -> [DeviceEvent] {
        try await requestJSON(method: "GET", path: "/api/v1/devices/\(deviceId)/events?limit=\(limit)")
    }

    public func patchCustomization(deviceId: Int64, patch: DeviceCustomizationPatch) async throws -> KnownDevice {
        let body = try patch.jsonData()
        return try await requestJSON(method: "PATCH", path: "/api/v1/devices/\(deviceId)", bodyData: body)
    }

    public func forgetDevice(id: Int64) async throws {
        try await requestNoContent(method: "DELETE", path: "/api/v1/devices/\(id)")
    }

    @discardableResult
    public func startScan() async throws -> Bool {
        let (data, status) = try await rawRequest(method: "POST", path: "/api/v1/scan")
        if status == 409 {
            return false
        }
        if !(200...299).contains(status) {
            throw Self.makeHttpError(status: status, data: data)
        }
        let dto = try? VigiaJSON.makeDecoder().decode(ScanStartedDto.self, from: data)
        return dto?.started ?? true
    }

    // MARK: - Seguridad v1.3

    public func securityReport() async throws -> SecurityReport {
        try await requestJSON(method: "GET", path: "/api/v1/security")
    }

    public func acknowledgeFinding(findingId: String, note: String? = nil) async throws {
        let encoded = Self.encodePathSegment(findingId)
        let body = try VigiaJSON.makeEncoder().encode(AcknowledgeFindingRequest(note: note))
        try await requestNoContent(method: "PUT", path: "/api/v1/security/acknowledged/\(encoded)", bodyData: body)
    }

    public func unacknowledgeFinding(findingId: String) async throws {
        let encoded = Self.encodePathSegment(findingId)
        try await requestNoContent(method: "DELETE", path: "/api/v1/security/acknowledged/\(encoded)")
    }

    // MARK: - Internet y Speedtest v1.4

    public func internetStatus() async throws -> InternetStatusDto {
        try await requestJSON(method: "GET", path: "/api/v1/internet")
    }

    public func internetSeries(range: InternetSeriesRange) async throws -> [InternetSeriesPointDto] {
        try await requestJSON(method: "GET", path: "/api/v1/internet/series?range=\(range.rawValue)")
    }

    @discardableResult
    public func startSpeedTest() async throws -> Bool {
        let (data, status) = try await rawRequest(method: "POST", path: "/api/v1/internet/speedtest")
        if status == 409 {
            return false
        }
        if !(200...299).contains(status) {
            throw Self.makeHttpError(status: status, data: data)
        }
        let dto = try? VigiaJSON.makeDecoder().decode(ScanStartedDto.self, from: data)
        return dto?.started ?? true
    }

    public func internetSettings() async throws -> InternetSettingsDto {
        try await requestJSON(method: "GET", path: "/api/v1/internet/settings")
    }

    public func putInternetSettings(_ settings: InternetSettingsDto) async throws -> InternetSettingsDto {
        let body = try VigiaJSON.makeEncoder().encode(settings)
        return try await requestJSON(method: "PUT", path: "/api/v1/internet/settings", bodyData: body)
    }

    // MARK: - Alertas y Webhooks

    public func alerts(after: Int64 = 0, limit: Int = 100) async throws -> [AlertDto] {
        try await requestJSON(method: "GET", path: "/api/v1/alerts?after=\(after)&limit=\(limit)")
    }

    public func markAlertsRead(upTo: Int64) async throws {
        let body = try VigiaJSON.makeEncoder().encode(AlertsReadRequest(upTo: upTo))
        try await requestNoContent(method: "POST", path: "/api/v1/alerts/read", bodyData: body)
    }

    public func alertSettings() async throws -> AlertSettingsDto {
        try await requestJSON(method: "GET", path: "/api/v1/alert-settings")
    }

    public func putAlertSettings(_ settings: AlertSettingsDto) async throws -> AlertSettingsDto {
        let body = try VigiaJSON.makeEncoder().encode(settings)
        return try await requestJSON(method: "PUT", path: "/api/v1/alert-settings", bodyData: body)
    }

    public func webhooks() async throws -> [WebhookDto] {
        try await requestJSON(method: "GET", path: "/api/v1/webhooks")
    }

    public func addWebhook(url: String, types: [String] = [], secret: String? = nil) async throws -> WebhookDto {
        let body = try VigiaJSON.makeEncoder().encode(AddWebhookRequest(url: url, types: types, secret: secret))
        return try await requestJSON(method: "POST", path: "/api/v1/webhooks", bodyData: body)
    }

    public func deleteWebhook(id: Int64) async throws {
        try await requestNoContent(method: "DELETE", path: "/api/v1/webhooks/\(id)")
    }

    public func testWebhook(id: Int64) async throws -> WebhookTestResultDto {
        try await requestJSON(method: "POST", path: "/api/v1/webhooks/\(id)/test", bodyData: Data("{}".utf8))
    }

    // MARK: - Stream SSE en vivo (/api/v1/stream) con reconexión exponencial

    public func streamEvents() -> AsyncStream<AgentSSEEvent> {
        AsyncStream { continuation in
            let task = Task { [weak self] in
                guard let self else {
                    continuation.finish()
                    return
                }
                var backoffSec: Double = 1.0
                while !Task.isCancelled {
                    do {
                        guard let url = URL(string: "\(self.baseUrl)/api/v1/stream") else {
                            continuation.finish()
                            return
                        }
                        var req = URLRequest(url: url)
                        req.httpMethod = "GET"
                        req.setValue("text/event-stream", forHTTPHeaderField: "Accept")
                        if let token = self.token {
                            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                        }

                        let (bytes, response) = try await self.streamSession.bytes(for: req)
                        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
                            throw AgentApiError.invalidResponse
                        }

                        backoffSec = 1.0
                        var currentEvent = ""
                        var currentDataLines: [String] = []

                        for try await line in bytes.lines {
                            if Task.isCancelled { break }
                            let cleaned = line.hasSuffix("\r") ? String(line.dropLast()) : line
                            if cleaned.isEmpty {
                                if !currentDataLines.isEmpty || !currentEvent.isEmpty {
                                    let payload = currentDataLines.joined(separator: "\n")
                                    if let parsed = Self.parseSSEEvent(name: currentEvent, data: payload) {
                                        continuation.yield(parsed)
                                    }
                                    currentEvent = ""
                                    currentDataLines.removeAll()
                                }
                                continue
                            }
                            if cleaned.hasPrefix(":") {
                                continue
                            }
                            if cleaned.hasPrefix("event:") {
                                currentEvent = String(cleaned.dropFirst(6)).trimmingCharacters(in: .whitespaces)
                            } else if cleaned.hasPrefix("data:") {
                                let value = String(cleaned.dropFirst(5)).trimmingCharacters(in: .whitespaces)
                                currentDataLines.append(value)
                            }
                        }
                    } catch {
                        if Task.isCancelled { break }
                    }

                    let delayNs = UInt64(backoffSec * 1_000_000_000)
                    try? await Task.sleep(nanoseconds: delayNs)
                    backoffSec = min(30.0, backoffSec * 2.0)
                }
                continuation.finish()
            }

            continuation.onTermination = { _ in
                task.cancel()
            }
        }
    }

    private static func parseSSEEvent(name: String, data: String) -> AgentSSEEvent? {
        switch name {
        case "scan":
            return .scanCompleted(data: data)
        case "device":
            return .deviceChanged(data: data)
        case "alert":
            if let jsonData = data.data(using: .utf8),
               let dto = try? VigiaJSON.makeDecoder().decode(AlertDto.self, from: jsonData) {
                return .alert(dto)
            }
            return .other(name: name, data: data)
        default:
            return data.isEmpty ? nil : .other(name: name, data: data)
        }
    }

    // MARK: - Helpers HTTP internos

    private func requestJSON<T: Decodable>(
        method: String,
        path: String,
        bodyData: Data? = nil,
        authenticated: Bool = true
    ) async throws -> T {
        let (data, status) = try await rawRequest(
            method: method,
            path: path,
            bodyData: bodyData,
            authenticated: authenticated
        )
        guard (200...299).contains(status) else {
            throw Self.makeHttpError(status: status, data: data)
        }
        return try VigiaJSON.makeDecoder().decode(T.self, from: data)
    }

    private func requestNoContent(
        method: String,
        path: String,
        bodyData: Data? = nil,
        authenticated: Bool = true
    ) async throws {
        let (data, status) = try await rawRequest(
            method: method,
            path: path,
            bodyData: bodyData,
            authenticated: authenticated
        )
        guard (200...299).contains(status) else {
            throw Self.makeHttpError(status: status, data: data)
        }
    }

    private func rawRequest(
        method: String,
        path: String,
        bodyData: Data? = nil,
        authenticated: Bool = true
    ) async throws -> (Data, Int) {
        guard let url = URL(string: "\(baseUrl)\(path)") else {
            throw AgentApiError.invalidURL("\(baseUrl)\(path)")
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        if authenticated {
            guard let token else { throw AgentApiError.notLinked }
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let bodyData {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = bodyData
        }
        let (data, response) = try await Self.perform(session: session, delegate: delegate, request: request)
        guard let http = response as? HTTPURLResponse else {
            throw AgentApiError.invalidResponse
        }
        return (data, http.statusCode)
    }

    private static func perform(
        session: URLSession,
        delegate: AgentTlsPinningDelegate,
        request: URLRequest
    ) async throws -> (Data, URLResponse) {
        do {
            return try await session.data(for: request)
        } catch {
            if let mismatch = delegate.pinMismatchError {
                throw mismatch
            }
            throw AgentApiError.transportError(error.localizedDescription)
        }
    }

    private static func validateHttp(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else {
            throw AgentApiError.invalidResponse
        }
        guard (200...299).contains(http.statusCode) else {
            throw makeHttpError(status: http.statusCode, data: data)
        }
    }

    private static func makeHttpError(status: Int, data: Data) -> AgentApiError {
        if let parsed = try? JSONDecoder().decode(ApiErrorBody.self, from: data), !parsed.error.isEmpty {
            return .httpError(statusCode: status, message: parsed.error)
        }
        let raw = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return .httpError(statusCode: status, message: raw)
    }

    /// Codifica estrictamente un segmento de ruta incluyendo `|` y `/` (`networkKey`, `findingId`).
    public static func encodePathSegment(_ segment: String) -> String {
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: "-._~"))
        return segment.addingPercentEncoding(withAllowedCharacters: allowed) ?? segment
    }
}

// MARK: - Descubrimiento Bonjour (_vigia._tcp) con NWBrowser

public struct DiscoveredAgentService: Identifiable, Equatable, Hashable, Sendable {
    public var id: String
    public var agentId: String
    public var name: String
    public var host: String
    public var port: Int
    public var certPrefix: String
    public var tlsEnabled: Bool
}

@MainActor
public final class AgentBonjourBrowser: ObservableObject {
    @Published public private(set) var discovered: [DiscoveredAgentService] = []
    @Published public private(set) var isSearching = false
    @Published public private(set) var lastError: String?

    private var browser: NWBrowser?
    private var activeResolvers: [String: NWConnection] = [:]

    public init() {}

    public func start() {
        stop()
        isSearching = true
        lastError = nil

        let descriptor = NWBrowser.Descriptor.bonjourWithTXTRecord(type: "_vigia._tcp", domain: "local.")
        let params = NWParameters.tcp
        params.includePeerToPeer = false

        let browser = NWBrowser(for: descriptor, using: params)
        browser.stateUpdateHandler = { [weak self] state in
            Task { @MainActor [weak self] in
                guard let self else { return }
                switch state {
                case .failed(let err):
                    self.isSearching = false
                    self.lastError = err.localizedDescription
                case .cancelled:
                    self.isSearching = false
                default:
                    break
                }
            }
        }

        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor [weak self] in
                self?.handleResults(results)
            }
        }

        self.browser = browser
        browser.start(queue: DispatchQueue(label: "com.baxner.vigia.bonjour", qos: .userInitiated))
    }

    public func stop() {
        browser?.cancel()
        browser = nil
        activeResolvers.values.forEach { $0.cancel() }
        activeResolvers.removeAll()
        isSearching = false
    }

    private func handleResults(_ results: Set<NWBrowser.Result>) {
        var currentIds = Set<String>()
        for result in results {
            guard case let .service(serviceName, _, _, _) = result.endpoint else { continue }
            currentIds.insert(serviceName)

            var agentId = ""
            var displayName = serviceName
            var certPrefix = ""
            var tlsEnabled = true

            if case let .bonjour(txt) = result.metadata {
                if case let .string(v) = txt.getEntry(for: "id") { agentId = v }
                if case let .string(v) = txt.getEntry(for: "name"), !v.isEmpty { displayName = v }
                if case let .string(v) = txt.getEntry(for: "cert") { certPrefix = v.lowercased() }
                if case let .string(v) = txt.getEntry(for: "tls") { tlsEnabled = (v != "0") }
            }

            if activeResolvers[serviceName] == nil && !discovered.contains(where: { $0.id == serviceName && !$0.host.isEmpty }) {
                resolveEndpoint(
                    serviceKey: serviceName,
                    endpoint: result.endpoint,
                    agentId: agentId,
                    name: displayName,
                    certPrefix: certPrefix,
                    tlsEnabled: tlsEnabled
                )
            }
        }
        discovered.removeAll { !currentIds.contains($0.id) }
    }

    private func resolveEndpoint(
        serviceKey: String,
        endpoint: NWEndpoint,
        agentId: String,
        name: String,
        certPrefix: String,
        tlsEnabled: Bool
    ) {
        let conn = NWConnection(to: endpoint, using: .tcp)
        activeResolvers[serviceKey] = conn
        conn.stateUpdateHandler = { [weak self] state in
            guard case .ready = state,
                  let remote = conn.currentPath?.remoteEndpoint,
                  case let .hostPort(host, port) = remote else {
                if case .failed = state {
                    conn.cancel()
                    Task { @MainActor [weak self] in
                        self?.activeResolvers.removeValue(forKey: serviceKey)
                    }
                }
                return
            }

            var hostStr = "\(host)"
            if let pct = hostStr.firstIndex(of: "%") {
                hostStr = String(hostStr[..<pct])
            }
            let item = DiscoveredAgentService(
                id: serviceKey,
                agentId: agentId,
                name: name,
                host: hostStr,
                port: Int(port.rawValue),
                certPrefix: certPrefix,
                tlsEnabled: tlsEnabled
            )
            conn.cancel()
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.activeResolvers.removeValue(forKey: serviceKey)
                if let idx = self.discovered.firstIndex(where: { $0.id == item.id }) {
                    self.discovered[idx] = item
                } else {
                    self.discovered.append(item)
                }
            }
        }
        conn.start(queue: DispatchQueue(label: "com.baxner.vigia.bonjour-resolve"))
    }
}
