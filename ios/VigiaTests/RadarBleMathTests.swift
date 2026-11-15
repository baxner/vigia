import XCTest
@testable import Vigia

final class RadarBleMathTests: XCTestCase {
    func testBluetoothCompanyCatalogParseTSVAndShortName() {
        let tsv = """
        76\tApple, Inc.
        117\tSamsung Electronics Co., Ltd.
        1551\tSignify Netherlands B.V.
        bad\tIgnored
        """
        let map = BluetoothCompanyCatalog.parseTSV(tsv)
        XCTAssertEqual(map.count, 3)
        XCTAssertEqual(map[76], "Apple, Inc.")
        XCTAssertEqual(map[117], "Samsung Electronics Co., Ltd.")
        XCTAssertEqual(map[1551], "Signify Netherlands B.V.")

        XCTAssertEqual(BluetoothCompanyCatalog.shortName("Apple, Inc."), "Apple")
        XCTAssertEqual(BluetoothCompanyCatalog.shortName("Samsung Electronics Co., Ltd."), "Samsung Electronics")
        XCTAssertEqual(BluetoothCompanyCatalog.shortName("Signify Netherlands B.V."), "Signify Netherlands")
    }

    func testKalmanRssiFilterSmoothingAndReset() {
        var filter = KalmanRssiFilter()
        XCTAssertNil(filter.estimate)

        let first = filter.update(rawRssi: -70.0)
        XCTAssertEqual(first, -70.0, accuracy: 0.001)

        // Sudden outlier shouldn't jump immediately to -40 in a single step
        let second = filter.update(rawRssi: -40.0)
        XCTAssertGreaterThan(second, -70.0)
        XCTAssertLessThan(second, -40.0)

        // Repeated stable readings converge toward -50
        var latest = second
        for _ in 0..<25 {
            latest = filter.update(rawRssi: -50.0)
        }
        XCTAssertEqual(latest, -50.0, accuracy: 1.5)

        filter.reset()
        XCTAssertNil(filter.estimate)
    }

    func testDistanceModelBoundsAndMonotonicity() {
        XCTAssertEqual(DistanceModel.proximityFraction(rssi: -110.0), 0.0, accuracy: 0.0001)
        XCTAssertEqual(DistanceModel.proximityFraction(rssi: -98.0), 0.0, accuracy: 0.0001)
        XCTAssertEqual(DistanceModel.proximityFraction(rssi: -35.0), 1.0, accuracy: 0.0001)
        XCTAssertEqual(DistanceModel.proximityFraction(rssi: -20.0), 1.0, accuracy: 0.0001)

        let near = DistanceModel.estimatedMeters(rssi: -50.0, txPowerAt1m: -59.0)
        let oneMeter = DistanceModel.estimatedMeters(rssi: -59.0, txPowerAt1m: -59.0)
        let far = DistanceModel.estimatedMeters(rssi: -85.0, txPowerAt1m: -59.0)
        XCTAssertEqual(oneMeter, 1.0, accuracy: 0.001)
        XCTAssertLessThan(near, oneMeter)
        XCTAssertGreaterThan(far, oneMeter)
        XCTAssertGreaterThanOrEqual(DistanceModel.estimatedMeters(rssi: -20.0), 0.2)
        XCTAssertLessThanOrEqual(DistanceModel.estimatedMeters(rssi: -120.0), 40.0)
    }

    func testTrendDetectorDetectsApproachingRecedingAndSteady() {
        var detector = TrendDetector()
        let base = 1_000.0

        // Fewer than 6 samples stays steady
        for i in 0..<4 {
            XCTAssertEqual(detector.add(rssi: -80.0 + Double(i) * 3.0, at: base + Double(i) * 0.4), .steady)
        }

        detector.reset()
        var trend: ProximityTrend = .steady
        for i in 0..<8 {
            trend = detector.add(rssi: -80.0 + Double(i) * 1.5, at: base + Double(i) * 0.4)
        }
        XCTAssertEqual(trend, .approaching)

        detector.reset()
        for i in 0..<8 {
            trend = detector.add(rssi: -55.0 - Double(i) * 1.5, at: base + Double(i) * 0.4)
        }
        XCTAssertEqual(trend, .receding)

        detector.reset()
        for i in 0..<8 {
            trend = detector.add(rssi: -65.0 + (i.isMultiple(of: 2) ? 0.3 : -0.3), at: base + Double(i) * 0.4)
        }
        XCTAssertEqual(trend, .steady)
    }

    func testCosineBearingEstimatorWraparoundAndTargetRecovery() throws {
        XCTAssertEqual(CosineBearingEstimator.normalizeDegrees(-45), 315, accuracy: 0.001)
        XCTAssertEqual(CosineBearingEstimator.normalizeDegrees(725), 5, accuracy: 0.001)
        XCTAssertEqual(CosineBearingEstimator.shortestAngleDiff(from: 350, to: 10), 20, accuracy: 0.001)
        XCTAssertEqual(CosineBearingEstimator.shortestAngleDiff(from: 10, to: 350), -20, accuracy: 0.001)

        var estimator = CosineBearingEstimator()
        // Only 4 sectors covered -> not enough for a bearing yet
        for idx in 0..<4 {
            estimator.addSample(headingDegrees: Double(idx) * 45.0, smoothedRssi: -60.0)
        }
        let partial = estimator.estimate()
        XCTAssertNil(partial.bearingDegrees)
        XCTAssertEqual(partial.coveredCount, 4)

        // All 8 sectors with peak at 90° (East)
        estimator.reset()
        let targetRad = 90.0 * Double.pi / 180.0
        for sector in 0..<8 {
            let deg = Double(sector) * 45.0
            let rad = deg * Double.pi / 180.0
            let rssi = -68.0 + 8.0 * cos(rad - targetRad)
            estimator.addSample(headingDegrees: deg, smoothedRssi: rssi)
        }
        let est = estimator.estimate()
        let bearing = try XCTUnwrap(est.bearingDegrees)
        XCTAssertEqual(abs(CosineBearingEstimator.shortestAngleDiff(from: bearing, to: 90.0)), 0.0, accuracy: 2.0)
        XCTAssertEqual(est.targetSector, 2)
        XCTAssertGreaterThan(est.confidence, 0.8)

        // Flat isotropic signal -> amplitude too low -> nil bearing
        estimator.reset()
        for sector in 0..<8 {
            estimator.addSample(headingDegrees: Double(sector) * 45.0, smoothedRssi: -65.0)
        }
        let flat = estimator.estimate()
        XCTAssertNil(flat.bearingDegrees)
    }

    func testBleLanKinshipMatchesUniqueNameOrBrand() {
        let appleTv = KnownDevice(
            id: 1,
            network: "192.168.1.1|192.168.1.0/24",
            mac: "3C:22:FB:11:22:33",
            lastIp: "192.168.1.15",
            firstSeen: "2026-11-14T10:00:00Z",
            lastSeen: "2026-11-14T18:00:00Z",
            online: true,
            last: Device(
                ip: "192.168.1.15",
                mac: "3C:22:FB:11:22:33",
                vendor: "Apple, Inc.",
                hostname: "livingroom-tv",
                identity: Identity(type: "tv", brand: "Apple", confidence: 90)
            )
        )
        let sonos = KnownDevice(
            id: 2,
            network: "192.168.1.1|192.168.1.0/24",
            mac: "00:0E:58:11:22:33",
            lastIp: "192.168.1.25",
            firstSeen: "2026-11-14T10:00:00Z",
            lastSeen: "2026-11-14T18:00:00Z",
            online: true,
            last: Device(
                ip: "192.168.1.25",
                mac: "00:0E:58:11:22:33",
                vendor: "Sonos, Inc.",
                hostname: "Sonos-Beam",
                identity: Identity(type: "speaker", brand: "Sonos", confidence: 90)
            )
        )

        let matchedByName = BleLanKinship.matchLanDevice(bleName: "Sonos Beam", bleBrand: "Sonos", lanDevices: [appleTv, sonos])
        XCTAssertEqual(matchedByName?.id, 2)

        let matchedByBrand = BleLanKinship.matchLanDevice(bleName: nil, bleBrand: "Apple", lanDevices: [appleTv, sonos])
        XCTAssertEqual(matchedByBrand?.id, 1)

        XCTAssertNil(BleLanKinship.matchLanDevice(bleName: "Unknown Gadget", bleBrand: "Unknown", lanDevices: [appleTv, sonos]))
    }
}
