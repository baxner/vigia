import XCTest
@testable import Vigia

final class AgentModelsAndSecurityTests: XCTestCase {
    func testPairRequestAndResponseCodingKeys() throws {
        let req = PairRequest(code: "123456", client: "My iPhone")
        let encoded = try VigiaJSON.makeEncoder().encode(req)
        let rawObj = try XCTUnwrap(try JSONSerialization.jsonObject(with: encoded) as? [String: Any])
        XCTAssertEqual(rawObj["code"] as? String, "123456")
        XCTAssertEqual(rawObj["client"] as? String, "My iPhone")

        let respJson = """
        {
          "token": "tok_abc123",
          "agentId": "agent-01",
          "agentName": "Vigia Pi",
          "certSha256": "deadbeef"
        }
        """
        let resp = try VigiaJSON.makeDecoder().decode(PairResponseDto.self, from: XCTUnwrap(respJson.data(using: .utf8)))
        XCTAssertEqual(resp.token, "tok_abc123")
        XCTAssertEqual(resp.agentId, "agent-01")
        XCTAssertEqual(resp.agentName, "Vigia Pi")
        XCTAssertEqual(resp.certSha256, "deadbeef")
    }

    func testAlertSettingsToggleAndDecoding() throws {
        var settings = AlertSettingsDto(newDevice: true, arpConflict: false, internetDown: true)
        XCTAssertTrue(settings.isEnabled(.newDevice))
        XCTAssertFalse(settings.isEnabled(.arpConflict))

        settings.setEnabled(true, for: .arpConflict)
        XCTAssertTrue(settings.isEnabled(.arpConflict))
        settings.setEnabled(false, for: .newDevice)
        XCTAssertFalse(settings.isEnabled(.newDevice))

        let json = """
        {"NEW_DEVICE": false, "ARP_CONFLICT": true, "INTERNET_DOWN": true}
        """
        let decoded = try VigiaJSON.makeDecoder().decode(AlertSettingsDto.self, from: XCTUnwrap(json.data(using: .utf8)))
        XCTAssertFalse(decoded.isEnabled(.newDevice))
        XCTAssertTrue(decoded.isEnabled(.arpConflict))
        XCTAssertTrue(decoded.isEnabled(.internetDown))
    }

    func testSecurityReportCleanNetworkScores100() throws {
        let input = SecurityInput(
            devices: [
                SecurityDevice(
                    id: 1,
                    name: "Apple TV",
                    online: true,
                    device: Device(
                        ip: "192.168.1.20",
                        mac: "3C:22:FB:11:22:33",
                        vendor: "Apple, Inc.",
                        identity: Identity(type: "tv", brand: "Apple", model: "Apple TV", confidence: 95),
                        openPorts: [443]
                    )
                )
            ],
            router: RouterInfo(ip: "192.168.1.1", upnp: false, natpmp: false, reach: "internet"),
            wifi: WifiInfo(ssid: "HomeNet", security: "wpa2", wps: false)
        )

        let report = try VigiaEngineBridge.shared.securityReport(input: input)
        XCTAssertEqual(report.score, 100)
        // Only informational findings (0 points)
        XCTAssertEqual(report.findings.filter { $0.points > 0 }.count, 0)
    }

    func testSecurityReportPenalizesRisksAndExcludesAcknowledgedFindings() throws {
        let input = SecurityInput(
            devices: [
                SecurityDevice(
                    id: 5,
                    name: "Old Camera",
                    online: true,
                    device: Device(
                        ip: "192.168.1.50",
                        mac: "00:11:22:33:44:55",
                        identity: Identity(type: "camera", brand: "Generic", confidence: 80),
                        openPorts: [23, 554]
                    )
                )
            ],
            router: RouterInfo(
                ip: "192.168.1.1",
                upnp: true,
                natpmp: false,
                reach: "internet",
                portMappings: [
                    PortMapping(
                        transportProtocol: "TCP",
                        externalPort: 8080,
                        internalIp: "192.168.1.50",
                        internalPort: 80,
                        description: "Cam",
                        source: "upnp",
                        deviceId: 5
                    )
                ]
            ),
            wifi: WifiInfo(ssid: "GuestOpen", security: "open", wps: true),
            recentAlerts: [
                SecurityAlertRef(type: AlertType.arpConflict.rawValue, at: VigiaDate.nowString())
            ]
        )

        let unackReport = try VigiaEngineBridge.shared.securityReport(input: input)
        // Total penalty: WIFI_OPEN(30) + WIFI_WPS(10) + UPNP(10) + PORT_MAPPING(8) + Telnet(15) + RTSP(8) + ARP_CONFLICT(30) = 111 -> clamped at 0
        XCTAssertEqual(unackReport.score, 0)
        let ids = Set(unackReport.findings.map(\.id))
        XCTAssertTrue(ids.contains("WIFI_OPEN"))
        XCTAssertTrue(ids.contains("WIFI_WPS"))
        XCTAssertTrue(ids.contains("UPNP_ENABLED"))
        XCTAssertTrue(unackReport.findings.contains(where: { $0.kind == "ARP_CONFLICT" }))
        XCTAssertTrue(unackReport.findings.contains(where: { $0.port == 23 && $0.severity == "high" }))

        // Acknowledge WIFI_OPEN (30 pts) and UPNP_ENABLED (10 pts): remaining penalty 71 -> score becomes 29
        var ackInput = input
        ackInput.acknowledged = [
            AcknowledgedFinding(findingId: "WIFI_OPEN"),
            AcknowledgedFinding(findingId: "UPNP_ENABLED")
        ]
        let ackReport = try VigiaEngineBridge.shared.securityReport(input: ackInput)
        XCTAssertEqual(ackReport.score, 29)
        XCTAssertTrue(ackReport.findings.first(where: { $0.id == "WIFI_OPEN" })?.acknowledged == true)
        XCTAssertTrue(ackReport.findings.first(where: { $0.id == "UPNP_ENABLED" })?.acknowledged == true)
    }

    func testSecurityReportNeverDropsBelowZeroAndRoundTripsJSON() throws {
        var devices: [SecurityDevice] = []
        for idx in 1...8 {
            devices.append(SecurityDevice(
                id: Int64(idx),
                name: "Host \(idx)",
                online: true,
                device: Device(ip: "192.168.1.\(10 + idx)", mac: "00:11:22:33:44:0\(idx)", openPorts: [23, 21, 445, 3389, 554])
            ))
        }
        let input = SecurityInput(
            devices: devices,
            router: RouterInfo(ip: "192.168.1.1", upnp: true, reach: "internet"),
            wifi: WifiInfo(ssid: "OpenNet", security: "open", wps: true)
        )
        let report = try VigiaEngineBridge.shared.securityReport(input: input)
        XCTAssertGreaterThanOrEqual(report.score, 0)

        let data = try VigiaJSON.makeEncoder().encode(report)
        let decoded = try VigiaJSON.makeDecoder().decode(SecurityReport.self, from: data)
        XCTAssertEqual(decoded.score, report.score)
        XCTAssertEqual(decoded.findings.count, report.findings.count)
    }
}
