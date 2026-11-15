import Foundation
import SwiftUI

/// Estado centralizado de Vigía para iOS: orquesta el modo local (`VigiaEngineBridge`, `IfaddrsNetworkProvider`,
/// `HotspotWifiProvider`) y el modo Agente 24/7 (`AgentApiClient` + `KeychainLinkStore` + SSE + WidgetKit).
@MainActor
public final class AppStore: ObservableObject {
    // Red local y Wi-Fi del iPhone
    @Published public private(set) var networkSnapshot = IfaddrsNetworkProvider.shared.currentSnapshot()
    @Published public private(set) var wifiSnapshot = HotspotWifiProvider.shared.fetchCurrentWifiPlaceholder()

    // Modo de operación: local vs Agente 24/7 vinculado
    @Published public private(set) var agentLink: AgentLink?
    @Published public private(set) var agentInfo: AgentInfoDto?
    @Published public private(set) var agentNetworks: [AgentNetworkDto] = []
    @Published public var selectedNetworkKey: String?
    @Published public private(set) var agentOnline = false
    @Published public private(set) var agentReachError: String?

    // Escaneo y lista unificada de equipos
    @Published public private(set) var isScanning = false
    @Published public private(set) var scanStage = ""
    @Published public private(set) var scanDone = 0
    @Published public private(set) var scanTotal = 0
    @Published public private(set) var lastScan: ScanResult?
    @Published public private(set) var devices: [KnownDevice] = []
    @Published public private(set) var isLoadingDevices = false

    // Seguridad, Internet, Alertas y Webhooks
    @Published public private(set) var securityReport: SecurityReport?
    @Published public private(set) var isLoadingSecurity = false
    @Published public private(set) var localRouterInfo: RouterInfo?

    @Published public private(set) var internetStatus: InternetStatusDto?
    @Published public private(set) var internetSeries: [InternetSeriesPointDto] = []
    @Published public var internetRange: InternetSeriesRange = .hours24
    @Published public private(set) var internetSettings = InternetSettingsDto()
    @Published public private(set) var isRunningSpeedTest = false

    @Published public private(set) var alerts: [AlertDto] = []
    @Published public private(set) var alertSettings = AlertSettingsDto()
    @Published public private(set) var webhooks: [WebhookDto] = []
    @Published public private(set) var bannerError: String?

    public let bleEngine = RadarBleEngine()

    private var networkObserverId: UUID?
    private var sseTask: Task<Void, Never>?
    private var activeClient: AgentApiClient?

    public init() {
        self.agentLink = KeychainLinkStore.shared.load()
        self.selectedNetworkKey = AgentPreferencesStore.selectedNetworkKeyValue
        startObservers()
    }

    deinit {
        sseTask?.cancel()
        if let id = networkObserverId {
            IfaddrsNetworkProvider.shared.removeObserver(id)
        }
    }

    public var isLinked: Bool {
        agentLink != nil
    }

    public var unreadAlertCount: Int {
        alerts.filter { !$0.read }.count
    }

    public var onlineDeviceCount: Int {
        devices.filter(\.online).count
    }

    public var activeSubnetLabel: String {
        if let selectedNetworkKey,
           let net = agentNetworks.first(where: { $0.key == selectedNetworkKey }),
           !net.subnet.isEmpty {
            return net.subnet
        }
        return networkSnapshot.isAvailable ? networkSnapshot.cidr : VigiaL10n.tr("Sin Wi-Fi local")
    }

    public func startObservers() {
        networkObserverId = IfaddrsNetworkProvider.shared.addObserver { [weak self] snap in
            Task { @MainActor [weak self] in
                self?.networkSnapshot = snap
            }
        }
        Task {
            await refreshWifi()
            if agentLink != nil {
                await refreshAgentWorkspace(triggerSSE: true)
            } else {
                refreshLocalSecurityReport()
                syncWidgetSummary()
            }
        }
    }

    public func handleSceneActive() {
        Task {
            networkSnapshot = IfaddrsNetworkProvider.shared.currentSnapshot()
            await refreshWifi()
            if agentLink != nil {
                await refreshAgentWorkspace(triggerSSE: sseTask == nil)
            }
        }
    }

    public func refreshWifi() async {
        wifiSnapshot = await HotspotWifiProvider.shared.fetchCurrentWifi()
        if !isLinked {
            refreshLocalSecurityReport()
        }
    }

    public func requestLocationForWifi() async {
        _ = await HotspotWifiProvider.shared.requestLocationPermission()
        await refreshWifi()
    }

    // MARK: - Escaneo (Local o Agente)

    public func triggerScan() async {
        bannerError = nil
        if let link = agentLink {
            isScanning = true
            scanStage = "agente"
            do {
                let client = await resolvedClient(for: link)
                _ = try await client.startScan()
                agentOnline = true
                agentReachError = nil
                try? await Task.sleep(nanoseconds: 1_200_000_000)
                await refreshDevicesAndSecurity(client: client)
            } catch {
                bannerError = error.localizedDescription
            }
            isScanning = false
            return
        }

        // Escaneo local con VigiaEngineBridge
        let snap = IfaddrsNetworkProvider.shared.currentSnapshot()
        networkSnapshot = snap
        guard snap.isAvailable else {
            bannerError = VigiaL10n.tr("Conéctate a una red Wi-Fi para escanear la red local.")
            return
        }

        isScanning = true
        scanStage = "hosts"
        scanDone = 0
        scanTotal = 254

        do {
            let result = try await VigiaEngineBridge.shared.scan(
                cidr: snap.cidr,
                gateway: snap.inferredGateway,
                selfIP: snap.ipv4,
                withPorts: true,
                bindIface: snap.interfaceName
            ) { [weak self] stage, done, total in
                Task { @MainActor [weak self] in
                    self?.scanStage = stage
                    self?.scanDone = done
                    self?.scanTotal = total
                }
            }
            lastScan = result
            applyLocalScanResult(result)

            if !snap.inferredGateway.isEmpty {
                localRouterInfo = try? await VigiaEngineBridge.shared.routerInfo(
                    gateway: snap.inferredGateway,
                    bindIface: snap.interfaceName
                )
            }
            refreshLocalSecurityReport()
        } catch VigiaEngineError.cancelled {
            // Cancelado por el usuario
        } catch {
            bannerError = error.localizedDescription
        }
        isScanning = false
    }

    public func cancelScan() {
        VigiaEngineBridge.shared.cancelScan()
        isScanning = false
    }

    private func applyLocalScanResult(_ result: ScanResult) {
        let mapped = result.devices.enumerated().map { idx, dev in
            KnownDevice(
                id: Int64(idx + 1),
                network: result.subnet,
                mac: dev.mac,
                lastIp: dev.ip,
                firstSeen: dev.lastSeen,
                lastSeen: dev.lastSeen,
                online: true,
                onlineSince: dev.lastSeen,
                last: dev,
                custom: DeviceCustomization()
            )
        }
        devices = mapped
        bleEngine.updateKnownLanDevices(mapped)
        syncWidgetSummary()
    }

    public func scanPortsOnDevice(_ known: KnownDevice, spec: String = "all") async -> [Int]? {
        do {
            let ports = try await VigiaEngineBridge.shared.scanPorts(
                ip: known.lastIp,
                spec: spec,
                bindIface: networkSnapshot.interfaceName
            )
            if !isLinked, let idx = devices.firstIndex(where: { $0.id == known.id }) {
                var updated = devices[idx]
                updated.last.openPorts = Array(Set(updated.last.openPorts + ports)).sorted()
                devices[idx] = updated
                refreshLocalSecurityReport()
            }
            return ports
        } catch {
            bannerError = error.localizedDescription
            return nil
        }
    }

    // MARK: - Vinculación y flujo del Agente 24/7

    public func linkAgent(_ newLink: AgentLink) async {
        do {
            try KeychainLinkStore.shared.save(newLink)
            agentLink = newLink
            activeClient = AgentApiClient(link: newLink)
            bannerError = nil
            await BackgroundAlertManager.shared.requestNotificationAuthorization()
            BackgroundAlertManager.shared.scheduleNextRefresh()
            await refreshAgentWorkspace(triggerSSE: true)
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func unlinkAgent() {
        sseTask?.cancel()
        sseTask = nil
        activeClient = nil
        try? KeychainLinkStore.shared.clear()
        AgentPreferencesStore.clearAgentSessionData()
        agentLink = nil
        agentInfo = nil
        agentNetworks = []
        selectedNetworkKey = nil
        agentOnline = false
        agentReachError = nil
        alerts = []
        webhooks = []
        internetStatus = nil
        internetSeries = []
        devices = []
        securityReport = nil
        refreshLocalSecurityReport()
        syncWidgetSummary()
    }

    public func refreshAgentWorkspace(triggerSSE: Bool = false) async {
        guard let link = agentLink else { return }
        let client = await resolvedClient(for: link)

        do {
            let info = try await client.info()
            agentInfo = info
            agentOnline = true
            agentReachError = nil
            if !info.addresses.isEmpty {
                AgentPreferencesStore.savedAddresses = info.addresses
            }

            let nets = try await client.networks()
            agentNetworks = nets
            if selectedNetworkKey == nil || !nets.contains(where: { $0.key == selectedNetworkKey }) {
                let defaultNet = nets.first(where: { $0.role == "inner" })?.key ?? nets.first?.key
                selectedNetworkKey = defaultNet
                AgentPreferencesStore.selectedNetworkKeyValue = defaultNet
            }

            await refreshDevicesAndSecurity(client: client)
            await refreshAlerts(client: client)
            await refreshInternet(client: client)
            webhooks = (try? await client.webhooks()) ?? webhooks
            alertSettings = (try? await client.alertSettings()) ?? alertSettings
            internetSettings = (try? await client.internetSettings()) ?? internetSettings

            if triggerSSE {
                startSSE(client: client)
            }
            syncWidgetSummary()
        } catch {
            agentOnline = false
            agentReachError = error.localizedDescription
        }
    }

    public func selectNetwork(key: String) async {
        selectedNetworkKey = key
        AgentPreferencesStore.selectedNetworkKeyValue = key
        guard let link = agentLink else { return }
        let client = await resolvedClient(for: link)
        await refreshDevicesAndSecurity(client: client)
    }

    private func refreshDevicesAndSecurity(client: AgentApiClient) async {
        isLoadingDevices = true
        defer { isLoadingDevices = false }
        if let key = selectedNetworkKey {
            if let fetched = try? await client.devices(networkKey: key) {
                devices = fetched
                bleEngine.updateKnownLanDevices(fetched)
            }
        }
        await refreshSecurity()
    }

    public func refreshSecurity() async {
        guard let link = agentLink else {
            refreshLocalSecurityReport()
            return
        }
        isLoadingSecurity = true
        defer { isLoadingSecurity = false }
        let client = await resolvedClient(for: link)
        do {
            securityReport = try await client.securityReport()
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func probeLocalRouterFromHere() async {
        let gw = networkSnapshot.inferredGateway
        guard !gw.isEmpty else {
            bannerError = VigiaL10n.tr("Conéctate a una red Wi-Fi para sondear el router local.")
            return
        }
        do {
            localRouterInfo = try await VigiaEngineBridge.shared.routerInfo(
                gateway: gw,
                bindIface: networkSnapshot.interfaceName
            )
            if !isLinked {
                refreshLocalSecurityReport()
            }
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func toggleAcknowledgeFinding(_ finding: SecurityFinding, note: String? = nil) async {
        if let link = agentLink {
            let client = await resolvedClient(for: link)
            do {
                if finding.acknowledged {
                    try await client.unacknowledgeFinding(findingId: finding.id)
                } else {
                    try await client.acknowledgeFinding(findingId: finding.id, note: note)
                }
                securityReport = try await client.securityReport()
            } catch {
                bannerError = error.localizedDescription
            }
            return
        }

        // Persistencia local sin agente
        var list = AgentPreferencesStore.localAcknowledgedFindings
        if finding.acknowledged {
            list.removeAll { $0.findingId == finding.id }
        } else if !list.contains(where: { $0.findingId == finding.id }) {
            list.append(AcknowledgedFinding(findingId: finding.id, at: VigiaDate.nowString(), note: note))
        }
        AgentPreferencesStore.localAcknowledgedFindings = list
        refreshLocalSecurityReport()
    }

    private func refreshLocalSecurityReport() {
        let secDevices = devices.map {
            SecurityDevice(id: $0.id, name: $0.displayName, online: $0.online, device: $0.last)
        }
        let input = SecurityInput(
            devices: secDevices,
            router: localRouterInfo,
            outerRing: nil,
            wifi: wifiSnapshot.toWifiInfo(),
            recentAlerts: [],
            acknowledged: AgentPreferencesStore.localAcknowledgedFindings
        )
        securityReport = try? VigiaEngineBridge.shared.securityReport(input: input)
    }

    // MARK: - Edición de equipo y eventos

    public func fetchDeviceEvents(deviceId: Int64) async -> [DeviceEvent] {
        guard let link = agentLink else { return [] }
        let client = await resolvedClient(for: link)
        return (try? await client.events(deviceId: deviceId, limit: 200)) ?? []
    }

    public func updateDeviceCustomization(deviceId: Int64, custom: DeviceCustomization) async -> KnownDevice? {
        guard let link = agentLink else { return nil }
        let client = await resolvedClient(for: link)
        do {
            let updated = try await client.patchCustomization(
                deviceId: deviceId,
                patch: DeviceCustomizationPatch.replaceAll(custom)
            )
            if let idx = devices.firstIndex(where: { $0.id == deviceId }) {
                devices[idx] = updated
            }
            bleEngine.updateKnownLanDevices(devices)
            return updated
        } catch {
            bannerError = error.localizedDescription
            return nil
        }
    }

    public func forgetDevice(id: Int64) async -> Bool {
        guard let link = agentLink else { return false }
        let client = await resolvedClient(for: link)
        do {
            try await client.forgetDevice(id: id)
            devices.removeAll { $0.id == id }
            syncWidgetSummary()
            return true
        } catch {
            bannerError = error.localizedDescription
            return false
        }
    }

    // MARK: - Alertas, Ajustes y Webhooks

    public func refreshAlerts(client overrideClient: AgentApiClient? = nil) async {
        guard let link = agentLink else { return }
        let client: AgentApiClient
        if let overrideClient {
            client = overrideClient
        } else {
            client = await resolvedClient(for: link)
        }
        if let fetched = try? await client.alerts(after: 0, limit: 150) {
            alerts = fetched
            if let maxId = fetched.map(\.id).max(), maxId > AgentPreferencesStore.lastNotifiedAlertId {
                AgentPreferencesStore.lastNotifiedAlertId = maxId
            }
            syncWidgetSummary()
        }
    }

    public func markAllAlertsRead() async {
        guard let link = agentLink, let maxId = alerts.map(\.id).max(), maxId > 0 else { return }
        let client = await resolvedClient(for: link)
        do {
            try await client.markAlertsRead(upTo: maxId)
            alerts = alerts.map { item in
                var copy = item
                copy.read = true
                return copy
            }
            syncWidgetSummary()
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func updateAlertSetting(_ type: AlertType, enabled: Bool) async {
        guard let link = agentLink else { return }
        var next = alertSettings
        next.setEnabled(enabled, for: type)
        alertSettings = next
        let client = await resolvedClient(for: link)
        do {
            alertSettings = try await client.putAlertSettings(next)
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func refreshWebhooks() async {
        guard let link = agentLink else { return }
        let client = await resolvedClient(for: link)
        do {
            webhooks = try await client.webhooks()
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func addWebhook(url: String, types: [String], secret: String?) async -> Bool {
        if case .invalid(let reason) = WebhookUrlValidator.validate(url) {
            bannerError = reason
            return false
        }
        guard let link = agentLink else { return false }
        let client = await resolvedClient(for: link)
        do {
            let created = try await client.addWebhook(url: url, types: types, secret: secret)
            webhooks.append(created)
            return true
        } catch {
            bannerError = error.localizedDescription
            return false
        }
    }

    public func deleteWebhook(id: Int64) async {
        guard let link = agentLink else { return }
        let client = await resolvedClient(for: link)
        do {
            try await client.deleteWebhook(id: id)
            webhooks.removeAll { $0.id == id }
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func testWebhook(id: Int64) async -> WebhookTestResultDto? {
        guard let link = agentLink else { return nil }
        let client = await resolvedClient(for: link)
        do {
            let result = try await client.testWebhook(id: id)
            await refreshWebhooks()
            return result
        } catch {
            bannerError = error.localizedDescription
            return nil
        }
    }

    // MARK: - Internet y Speedtest

    public func refreshInternet(client overrideClient: AgentApiClient? = nil) async {
        guard let link = agentLink else { return }
        let client: AgentApiClient
        if let overrideClient {
            client = overrideClient
        } else {
            client = await resolvedClient(for: link)
        }
        if let status = try? await client.internetStatus() {
            internetStatus = status
        }
        if let series = try? await client.internetSeries(range: internetRange) {
            internetSeries = series
        }
        syncWidgetSummary()
    }

    public func changeInternetRange(_ range: InternetSeriesRange) async {
        internetRange = range
        guard let link = agentLink else { return }
        let client = await resolvedClient(for: link)
        if let series = try? await client.internetSeries(range: range) {
            internetSeries = series
        }
    }

    public func triggerSpeedTest() async {
        guard let link = agentLink else { return }
        isRunningSpeedTest = true
        defer { isRunningSpeedTest = false }
        let client = await resolvedClient(for: link)
        do {
            _ = try await client.startSpeedTest()
            for _ in 0..<8 {
                try? await Task.sleep(nanoseconds: 2_500_000_000)
                if let status = try? await client.internetStatus() {
                    internetStatus = status
                    if let latest = status.speedTests.first,
                       let atDate = VigiaDate.parse(latest.at),
                       Date().timeIntervalSince(atDate) < 35 {
                        break
                    }
                }
            }
        } catch {
            bannerError = error.localizedDescription
        }
    }

    public func updateSpeedTestsPerDay(_ count: Int) async {
        guard let link = agentLink else { return }
        let clamped = max(0, min(6, count))
        internetSettings.speedTestsPerDay = clamped
        let client = await resolvedClient(for: link)
        do {
            internetSettings = try await client.putInternetSettings(InternetSettingsDto(speedTestsPerDay: clamped))
        } catch {
            bannerError = error.localizedDescription
        }
    }

    // MARK: - SSE & Widget

    private func startSSE(client: AgentApiClient) {
        sseTask?.cancel()
        sseTask = Task { [weak self] in
            for await event in client.streamEvents() {
                guard !Task.isCancelled, let self else { break }
                switch event {
                case .scanCompleted:
                    await self.refreshDevicesAndSecurity(client: client)
                case .deviceChanged:
                    if let key = self.selectedNetworkKey,
                       let updated = try? await client.devices(networkKey: key) {
                        self.devices = updated
                        self.bleEngine.updateKnownLanDevices(updated)
                        self.syncWidgetSummary()
                    }
                case .alert(let alert):
                    if !self.alerts.contains(where: { $0.id == alert.id }) {
                        self.alerts.insert(alert, at: 0)
                        if alert.id > AgentPreferencesStore.lastNotifiedAlertId {
                            AgentPreferencesStore.lastNotifiedAlertId = alert.id
                        }
                        self.syncWidgetSummary()
                    }
                case .other:
                    break
                }
            }
        }
    }

    private func resolvedClient(for link: AgentLink) async -> AgentApiClient {
        if let activeClient {
            return activeClient
        }
        let client = await AgentApiClient.connectReachable(link: link)
        activeClient = client
        return client
    }

    private func syncWidgetSummary() {
        let latencyMs = internetStatus?.now.internetMs.map { Int($0.rounded()) }
        BackgroundAlertManager.shared.updateWidgetSummary(
            linked: isLinked,
            onlineDevices: onlineDeviceCount,
            unreadAlerts: unreadAlertCount,
            internetState: internetStatus?.state ?? (networkSnapshot.isAvailable ? "up" : "unknown"),
            internetLatencyMs: latencyMs
        )
    }

    public func clearBannerError() {
        bannerError = nil
    }
}

private extension HotspotWifiProvider {
    func fetchCurrentWifiPlaceholder() -> HotspotWifiSnapshot {
        HotspotWifiSnapshot()
    }
}
