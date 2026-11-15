import XCTest
@testable import Vigia

final class CoreModelsTests: XCTestCase {
    func testScanResultFullJSONDecoding() throws {
        let json = """
        {
          "startedAt": "2026-11-14T18:30:00Z",
          "duration": 1250000000,
          "subnet": "192.168.1.0/24",
          "self": "192.168.1.42",
          "gateway": "192.168.1.1",
          "devices": [
            {
              "ip": "192.168.1.1",
              "mac": "AA:BB:CC:11:22:33",
              "vendor": "TP-Link",
              "hostname": "router.lan",
              "openPorts": [80, 443],
              "sources": ["arp", "tcp"],
              "services": [
                {"port": 80, "proto": "tcp", "name": "http", "banner": "nginx"}
              ],
              "identity": {
                "type": "router",
                "brand": "TP-Link",
                "model": "Archer AX55",
                "os": "OpenWrt",
                "confidence": 95,
                "reason": "Gateway IP + UPnP root"
              },
              "lastSeen": "2026-11-14T18:30:00Z"
            }
          ]
        }
        """
        let data = try XCTUnwrap(json.data(using: .utf8))
        let result = try VigiaJSON.makeDecoder().decode(ScanResult.self, from: data)

        XCTAssertEqual(result.subnet, "192.168.1.0/24")
        XCTAssertEqual(result.selfIP, "192.168.1.42")
        XCTAssertEqual(result.gateway, "192.168.1.1")
        XCTAssertEqual(result.duration, 1_250_000_000)
        XCTAssertEqual(result.devices.count, 1)

        let dev = try XCTUnwrap(result.devices.first)
        XCTAssertEqual(dev.ip, "192.168.1.1")
        XCTAssertEqual(dev.displayName, "router.lan")
        XCTAssertEqual(dev.sources, ["arp", "tcp"])
        XCTAssertEqual(dev.openPorts, [80, 443])
        XCTAssertEqual(dev.services.first?.name, "http")
        XCTAssertEqual(dev.identity.type, "router")
        XCTAssertEqual(dev.identity.confidence, 95)
    }

    func testDeviceDecodingWithMinimalFieldsAndFallbackDisplayName() throws {
        let json = """
        {
          "ip": "192.168.1.42",
          "mac": "02:11:22:33:44:55",
          "vendor": "Raspberry Pi",
          "identity": {
            "brand": "Raspberry",
            "model": "Pi 5"
          }
        }
        """
        let data = try XCTUnwrap(json.data(using: .utf8))
        let dev = try VigiaJSON.makeDecoder().decode(Device.self, from: data)

        XCTAssertEqual(dev.displayName, "Raspberry Pi 5")
        XCTAssertEqual(dev.identity.type, "unknown")
        XCTAssertEqual(dev.openPorts, [])
        XCTAssertEqual(dev.services, [])
    }

    func testKnownDeviceDisplayNameAndResolvedTypePriority() {
        let custom = DeviceCustomization(name: "Living Room TV", typeId: "tv", location: "Living Room", important: true)
        let scanDev = Device(
            ip: "192.168.1.88",
            mac: "10:20:30:40:50:60",
            vendor: "LG Electronics",
            hostname: "LGwebOSTV",
            identity: Identity(type: "smart", brand: "LG", model: "webOS", confidence: 70)
        )
        let known = KnownDevice(
            id: 7,
            network: "192.168.1.1|192.168.1.0/24",
            mac: "10:20:30:40:50:60",
            lastIp: "192.168.1.88",
            firstSeen: "2026-11-14T10:00:00Z",
            lastSeen: "2026-11-14T18:00:00Z",
            online: true,
            last: scanDev,
            custom: custom
        )

        XCTAssertEqual(known.displayName, "Living Room TV")
        XCTAssertEqual(DeviceType(rawValue: known.custom.typeId ?? ""), .tv)
        XCTAssertTrue(known.custom.important)
        XCTAssertEqual(known.custom.location, "Living Room")
    }

    func testPortCatalogAndBridgeLookups() {
        let bridge = VigiaEngineBridge.shared
        let catalog = bridge.portCatalog()
        XCTAssertFalse(catalog.isEmpty)

        let telnet = bridge.portInfo(23)
        XCTAssertEqual(telnet?.name, "Telnet")
        XCTAssertEqual(telnet?.risk, "high")
        XCTAssertTrue(telnet?.isRisky == true)

        let https = bridge.portInfo(443)
        XCTAssertEqual(https?.risk, "none")
        XCTAssertFalse(https?.isRisky == true)

        let risky = bridge.riskyPorts(among: [22, 23, 80, 443, 445])
        XCTAssertEqual(Set(risky.map(\.port)), [23, 445])
    }

    func testMacVendorLookupIgnoresRandomizedLocallyAdministeredAddresses() {
        let bridge = VigiaEngineBridge.shared
        // Locally administered bit set (second hex char 2, 6, A, E) -> randomized privacy MAC
        XCTAssertNil(bridge.vendorForMAC("02:11:22:33:44:55"))
        XCTAssertNil(bridge.vendorForMAC("DA:11:22:33:44:55"))

        XCTAssertEqual(bridge.vendorForMAC("3C:22:FB:11:22:33"), "Apple, Inc.")
        XCTAssertEqual(bridge.brandForMAC("3C:22:FB:11:22:33"), "Apple")
    }

    func testNetworkKeyFormat() {
        XCTAssertEqual(
            VigiaEngineBridge.shared.networkKey(gateway: "192.168.1.1", subnet: "192.168.1.0/24"),
            "192.168.1.1|192.168.1.0/24"
        )
    }
}
