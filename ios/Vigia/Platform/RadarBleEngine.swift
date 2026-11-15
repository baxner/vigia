import Foundation
import AudioToolbox
import Combine
import CoreBluetooth
import CoreLocation
import CoreMotion
#if canImport(UIKit)
import UIKit
#endif
#if canImport(VigiaCore)
import VigiaCore
#endif

// MARK: - Catálogo de fabricantes Bluetooth SIG

public final class BluetoothCompanyCatalog: @unchecked Sendable {
    public static let shared = BluetoothCompanyCatalog()

    private let companies: [Int: String]

    private init() {
        var map = Self.builtInFallback
        if let url = Bundle.main.url(forResource: "bt_companies", withExtension: "tsv"),
           let content = try? String(contentsOf: url, encoding: .utf8) {
            let parsed = Self.parseTSV(content)
            for (k, v) in parsed {
                map[k] = v
            }
        }
        self.companies = map
    }

    public func fullName(for companyId: Int) -> String? {
        #if canImport(VigiaCore)
        let coreName = MobileBluetoothCompanyFullName(companyId)
        if !coreName.isEmpty { return coreName }
        #endif
        return companies[companyId]
    }

    public func brand(for companyId: Int) -> String? {
        #if canImport(VigiaCore)
        let coreBrand = MobileBluetoothCompanyBrand(companyId)
        if !coreBrand.isEmpty { return coreBrand }
        #endif
        return companies[companyId].map(Self.shortName)
    }

    public static func parseTSV(_ raw: String) -> [Int: String] {
        var result: [Int: String] = [:]
        for line in raw.split(whereSeparator: \.isNewline) {
            guard let tabIdx = line.firstIndex(of: "\t") else { continue }
            guard let id = Int(line[..<tabIdx].trimmingCharacters(in: .whitespaces)) else { continue }
            let name = line[line.index(after: tabIdx)...].trimmingCharacters(in: .whitespacesAndNewlines)
            if !name.isEmpty {
                result[id] = name
            }
        }
        return result
    }

    public static func shortName(_ fullName: String) -> String {
        var name = fullName.trimmingCharacters(in: .whitespacesAndNewlines)
        let suffixes = [
            "Incorporated", "Inc", "LLC", "L.L.C", "Limited", "Ltd",
            "Corporation", "Corp", "Company", "Co", "GmbH", "AG", "KG",
            "S.A.U", "S.A", "S.L", "S.p.A", "B.V", "N.V", "AB", "Oy", "PLC"
        ]
        for _ in 0..<3 {
            var trimmed = false
            for suffix in suffixes {
                let pattern = "(?i)(,\\s*|\\s+)\(NSRegularExpression.escapedPattern(for: suffix))\\.?\\s*$"
                if let range = name.range(of: pattern, options: .regularExpression) {
                    let candidate = String(name[..<range.lowerBound])
                        .trimmingCharacters(in: CharacterSet(charactersIn: " ,."))
                    if !candidate.isEmpty {
                        name = candidate
                        trimmed = true
                        break
                    }
                }
            }
            if !trimmed { break }
        }
        return name
    }

    private static let builtInFallback: [Int: String] = [
        0x0006: "Microsoft",
        0x004C: "Apple, Inc.",
        0x0059: "Nordic Semiconductor ASA",
        0x0075: "Samsung Electronics Co. Ltd.",
        0x0087: "Garmin International, Inc.",
        0x009E: "Bose Corporation",
        0x00E0: "Google",
        0x012D: "Sony Corporation",
        0x0157: "Anhui Huami Information Technology Co., Ltd.",
        0x0171: "Amazon.com Services LLC",
        0x02E5: "Espressif Inc.",
        0x038F: "Xiaomi Inc.",
        0x0499: "Ruuvi Innovations Ltd.",
        0x05A7: "Sonos Inc.",
        0x060F: "Signify Netherlands B.V.",
        0x0822: "Shenzhen Goodix Technology"
    ]
}

// MARK: - Matemáticas del Radar (Kalman, distancia, tendencia y ajuste de coseno en 8 sectores)

/// Filtro Kalman 1D para suavizar la lectura ruidosa de RSSI Bluetooth LE.
public struct KalmanRssiFilter: Sendable {
    public private(set) var estimate: Double?
    private var errorCovariance: Double = 6.0
    private let processNoise: Double
    private let measurementNoise: Double

    public init(processNoise: Double = 0.08, measurementNoise: Double = 6.0) {
        self.processNoise = processNoise
        self.measurementNoise = measurementNoise
    }

    public mutating func reset() {
        estimate = nil
        errorCovariance = measurementNoise
    }

    @discardableResult
    public mutating func update(rawRssi: Double) -> Double {
        guard let current = estimate else {
            estimate = rawRssi
            errorCovariance = measurementNoise
            return rawRssi
        }
        let predictedCov = errorCovariance + processNoise
        let gain = predictedCov / (predictedCov + measurementNoise)
        let updated = current + gain * (rawRssi - current)
        errorCovariance = (1.0 - gain) * predictedCov
        estimate = updated
        return updated
    }
}

public enum ProximityTrend: String, Sendable {
    case approaching
    case receding
    case steady

    public var label: String {
        switch self {
        case .approaching: return VigiaL10n.tr("Acercándose")
        case .receding: return VigiaL10n.tr("Alejándose")
        case .steady: return VigiaL10n.tr("Estable")
        }
    }

    public var labelEs: String { label }
}

public enum DistanceModel {
    /// Convierte RSSI (dBm) en fracción visual `0...1` para medidores y Geiger.
    public static func proximityFraction(rssi: Double) -> Double {
        #if canImport(VigiaCore)
        return MobileProximityFraction(rssi)
        #else
        let clamped = min(-35.0, max(-98.0, rssi))
        return (clamped - (-98.0)) / (-35.0 - (-98.0))
        #endif
    }

    /// Estimación de distancia por modelo log-distance path loss.
    public static func estimatedMeters(rssi: Double, txPowerAt1m: Double = -59.0, pathLossExponent: Double = 2.6) -> Double {
        #if canImport(VigiaCore)
        if abs(pathLossExponent - 2.6) < 0.001,
           let range = MobileEstimateDistance(rssi, txPowerAt1m) {
            return range.typicalMeters
        }
        #endif
        let ratio = (txPowerAt1m - rssi) / (10.0 * pathLossExponent)
        return max(0.2, min(40.0, pow(10.0, ratio)))
    }

    public static func zoneLabel(rssi: Double, txPowerAt1m: Double = -59.0) -> String {
        let meters = estimatedMeters(rssi: rssi, txPowerAt1m: txPowerAt1m)
        switch meters {
        case ..<1.1: return VigiaL10n.tr("Encima (~<1 m)")
        case 1.1..<2.8: return VigiaL10n.tr("Muy cerca (~1–3 m)")
        case 2.8..<7.0: return VigiaL10n.tr("Misma habitación (~3–7 m)")
        default: return VigiaL10n.tr("Lejos (>7 m)")
        }
    }

    public static func zoneLabelEs(rssi: Double, txPowerAt1m: Double = -59.0) -> String {
        zoneLabel(rssi: rssi, txPowerAt1m: txPowerAt1m)
    }
}

public struct TrendDetector: Sendable {
    private var samples: [(time: TimeInterval, rssi: Double)] = []
    private let windowSeconds: TimeInterval = 5.0

    public init() {}

    public mutating func reset() {
        samples.removeAll()
    }

    public mutating func add(rssi: Double, at timestamp: TimeInterval = Date().timeIntervalSinceReferenceDate) -> ProximityTrend {
        samples.append((timestamp, rssi))
        samples.removeAll { timestamp - $0.time > windowSeconds }
        guard samples.count >= 6 else { return .steady }

        let mid = samples.count / 2
        let firstHalf = samples[..<mid].map(\.rssi).reduce(0, +) / Double(mid)
        let secondHalf = samples[mid...].map(\.rssi).reduce(0, +) / Double(samples.count - mid)
        let delta = secondHalf - firstHalf
        if delta > 2.2 { return .approaching }
        if delta < -2.2 { return .receding }
        return .steady
    }
}

public struct BearingEstimate: Equatable, Sendable {
    public var bearingDegrees: Double?
    public var confidence: Double
    public var coveredSectors: [Bool]
    public var coveredCount: Int
    public var targetSector: Int?

    public init(
        bearingDegrees: Double?,
        confidence: Double,
        coveredSectors: [Bool],
        coveredCount: Int,
        targetSector: Int?
    ) {
        self.bearingDegrees = bearingDegrees
        self.confidence = confidence
        self.coveredSectors = coveredSectors
        self.coveredCount = coveredCount
        self.targetSector = targetSector
    }
}

/// Estimador de rumbo en 8 sectores de 45° ("Vuelta guiada") por ajuste de coseno:
/// ajusta `RSSI(θ) ≈ a + b*cos(θ) + c*sin(θ)` por mínimos cuadrados sobre los promedios por sector.
public struct CosineBearingEstimator: Sendable {
    public static let sectorCount = 8
    private var sectorSamples: [[Double]] = Array(repeating: [], count: sectorCount)

    public init() {}

    public mutating func reset() {
        sectorSamples = Array(repeating: [], count: Self.sectorCount)
    }

    public mutating func addSample(headingDegrees: Double, smoothedRssi: Double) {
        let normalized = Self.normalizeDegrees(headingDegrees)
        let sector = min(Self.sectorCount - 1, Int((normalized + 22.5).truncatingRemainder(dividingBy: 360.0) / 45.0))
        sectorSamples[sector].append(smoothedRssi)
        if sectorSamples[sector].count > 12 {
            sectorSamples[sector].removeFirst()
        }
    }

    public func estimate() -> BearingEstimate {
        let covered = sectorSamples.map { !$0.isEmpty }
        let coveredCount = covered.filter { $0 }.count
        guard coveredCount >= 5 else {
            return BearingEstimate(
                bearingDegrees: nil,
                confidence: 0,
                coveredSectors: covered,
                coveredCount: coveredCount,
                targetSector: nil
            )
        }

        // Resuelve a + b*cos(θ) + c*sin(θ) por ecuaciones normales de Gauss sobre sectores cubiertos.
        var s00 = 0.0, s01 = 0.0, s02 = 0.0
        var s11 = 0.0, s12 = 0.0, s22 = 0.0
        var y0 = 0.0, y1 = 0.0, y2 = 0.0
        var means: [(theta: Double, mean: Double)] = []

        for idx in 0..<Self.sectorCount {
            let vals = sectorSamples[idx]
            guard !vals.isEmpty else { continue }
            let mean = vals.reduce(0, +) / Double(vals.count)
            let theta = Double(idx) * (Double.pi / 4.0)
            let cs = cos(theta)
            let sn = sin(theta)
            means.append((theta, mean))

            s00 += 1.0
            s01 += cs
            s02 += sn
            s11 += cs * cs
            s12 += cs * sn
            s22 += sn * sn
            y0 += mean
            y1 += mean * cs
            y2 += mean * sn
        }

        guard let coeffs = solve3x3(
            m: [[s00, s01, s02], [s01, s11, s12], [s02, s12, s22]],
            v: [y0, y1, y2]
        ) else {
            return BearingEstimate(bearingDegrees: nil, confidence: 0, coveredSectors: covered, coveredCount: coveredCount, targetSector: nil)
        }

        let b = coeffs[1]
        let c = coeffs[2]
        let amplitude = hypot(b, c)
        guard amplitude >= 1.8 else {
            return BearingEstimate(bearingDegrees: nil, confidence: 0.1, coveredSectors: covered, coveredCount: coveredCount, targetSector: nil)
        }

        var angleRad = atan2(c, b)
        if angleRad < 0 { angleRad += 2.0 * Double.pi }
        let deg = Self.normalizeDegrees(angleRad * 180.0 / Double.pi)

        // R² del ajuste como medida de confianza (0..1)
        let overallMean = means.map(\.mean).reduce(0, +) / Double(means.count)
        let ssTot = means.reduce(0.0) { $0 + pow($1.mean - overallMean, 2) }
        let ssRes = means.reduce(0.0) { acc, item in
            let pred = coeffs[0] + b * cos(item.theta) + c * sin(item.theta)
            return acc + pow(item.mean - pred, 2)
        }
        let r2 = ssTot > 0.01 ? max(0.0, min(1.0, 1.0 - ssRes / ssTot)) : 0.0
        let coverageFactor = Double(coveredCount) / Double(Self.sectorCount)
        let confidence = min(1.0, r2 * 0.75 + min(1.0, amplitude / 8.0) * 0.25) * coverageFactor

        let targetSector = min(Self.sectorCount - 1, Int((deg + 22.5).truncatingRemainder(dividingBy: 360.0) / 45.0))
        return BearingEstimate(
            bearingDegrees: deg,
            confidence: confidence,
            coveredSectors: covered,
            coveredCount: coveredCount,
            targetSector: targetSector
        )
    }

    public static func normalizeDegrees(_ deg: Double) -> Double {
        #if canImport(VigiaCore)
        return MobileNormalizeDegrees(deg)
        #else
        let mod = deg.truncatingRemainder(dividingBy: 360.0)
        return mod >= 0 ? mod : mod + 360.0
        #endif
    }

    public static func shortestAngleDiff(from: Double, to: Double) -> Double {
        #if canImport(VigiaCore)
        return MobileShortestAngleDiff(from, to)
        #else
        var diff = normalizeDegrees(to) - normalizeDegrees(from)
        if diff > 180.0 { diff -= 360.0 }
        if diff < -180.0 { diff += 360.0 }
        return diff
        #endif
    }

    private func solve3x3(m: [[Double]], v: [Double]) -> [Double]? {
        var a = m
        var b = v
        for i in 0..<3 {
            var maxRow = i
            for k in (i + 1)..<3 where abs(a[k][i]) > abs(a[maxRow][i]) {
                maxRow = k
            }
            if maxRow != i {
                a.swapAt(i, maxRow)
                b.swapAt(i, maxRow)
            }
            let pivot = a[i][i]
            guard abs(pivot) > 1e-8 else { return nil }
            for k in (i + 1)..<3 {
                let factor = a[k][i] / pivot
                for j in i..<3 {
                    a[k][j] -= factor * a[i][j]
                }
                b[k] -= factor * b[i]
            }
        }
        var x = [0.0, 0.0, 0.0]
        for i in stride(from: 2, through: 0, by: -1) {
            var sum = b[i]
            for j in (i + 1)..<3 {
                sum -= a[i][j] * x[j]
            }
            x[i] = sum / a[i][i]
        }
        return x
    }
}

// MARK: - Modelo de equipo BLE descubierto + afinidad con red LAN

public struct BlePeripheralReading: Identifiable, Equatable, Sendable {
    public var id: UUID
    public var name: String?
    public var companyId: Int?
    public var brandName: String?
    public var rssi: Int
    public var smoothedRssi: Double
    public var txPower: Int?
    public var serviceUuids: [String]
    public var isConnectable: Bool
    public var lastSeen: Date
    public var matchedLanDeviceIp: String?
    public var matchedLanDeviceName: String?

    public var displayName: String {
        if let name = name?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty {
            return name
        }
        if let brandName, !brandName.isEmpty {
            return brandName
        }
        return VigiaL10n.tr("Dispositivo BLE")
    }

    public var isNamed: Bool {
        guard let name = name?.trimmingCharacters(in: .whitespacesAndNewlines) else { return false }
        return !name.isEmpty
    }

    public var estimatedMeters: Double {
        DistanceModel.estimatedMeters(rssi: smoothedRssi, txPowerAt1m: Double(txPower ?? -59))
    }
}

public enum BleLanKinship {
    /// Busca afinidad entre un anuncio BLE y un equipo del inventario LAN (por nombre/marca/hostname).
    public static func matchLanDevice(
        bleName: String?,
        bleBrand: String?,
        lanDevices: [KnownDevice]
    ) -> KnownDevice? {
        let normName = normalize(bleName)
        let normBrand = normalize(bleBrand)

        if !normName.isEmpty {
            if let exact = lanDevices.first(where: {
                let dName = normalize($0.displayName)
                let host = normalize($0.last.hostname)
                return (!dName.isEmpty && (dName.contains(normName) || normName.contains(dName)))
                    || (!host.isEmpty && (host.contains(normName) || normName.contains(host)))
            }) {
                return exact
            }
        }
        if !normBrand.isEmpty {
            return lanDevices.first(where: {
                let b = normalize($0.last.identity.brand)
                return !b.isEmpty && b == normBrand && $0.online
            })
        }
        return nil
    }

    private static func normalize(_ raw: String?) -> String {
        (raw ?? "")
            .lowercased()
            .replacingOccurrences(of: ".local", with: "")
            .replacingOccurrences(of: "-", with: " ")
            .replacingOccurrences(of: "_", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

// MARK: - Motor BLE + Brújula + Motion + Geiger (CoreBluetooth / CoreLocation / CoreMotion)

#if os(iOS)
@MainActor
public final class RadarBleEngine: NSObject, ObservableObject {
    @Published public private(set) var authorizationState: CBManagerAuthorization = CBCentralManager.authorization
    @Published public private(set) var bluetoothState: CBManagerState = .unknown
    @Published public private(set) var isScanning = false
    @Published public private(set) var peripherals: [BlePeripheralReading] = []

    // Estado de búsqueda activa ("HuntView")
    @Published public private(set) var huntedPeripheralId: UUID?
    @Published public private(set) var huntedReading: BlePeripheralReading?
    @Published public private(set) var huntedTrend: ProximityTrend = .steady
    @Published public private(set) var currentHeadingDeg: Double = 0
    @Published public private(set) var currentPitchDeg: Double = 0
    @Published public private(set) var bearingEstimate = BearingEstimate(
        bearingDegrees: nil,
        confidence: 0,
        coveredSectors: Array(repeating: false, count: CosineBearingEstimator.sectorCount),
        coveredCount: 0,
        targetSector: nil
    )
    @Published public var geigerSoundEnabled: Bool = false
    @Published public var geigerHapticEnabled: Bool = true

    private var centralManager: CBCentralManager?
    private let locationManager = CLLocationManager()
    private let motionManager = CMMotionManager()
    private let companyCatalog = BluetoothCompanyCatalog.shared

    private var knownLanDevices: [KnownDevice] = []
    private var filtersById: [UUID: KalmanRssiFilter] = [:]
    private var readingMap: [UUID: BlePeripheralReading] = [:]
    private var huntedFilter = KalmanRssiFilter()
    private var trendDetector = TrendDetector()
    private var bearingEstimator = CosineBearingEstimator()

    private var geigerTask: Task<Void, Never>?
    private var stalePruneTask: Task<Void, Never>?
    #if canImport(UIKit)
    private let impactGenerator = UIImpactFeedbackGenerator(style: .medium)
    #endif

    public override init() {
        super.init()
        locationManager.delegate = self
        locationManager.headingFilter = 2
    }

    public func updateKnownLanDevices(_ devices: [KnownDevice]) {
        knownLanDevices = devices
    }

    public func startScanning() {
        ensureCentral()
        authorizationState = CBCentralManager.authorization
        guard let centralManager else { return }
        isScanning = true
        if centralManager.state == .poweredOn {
            centralManager.scanForPeripherals(
                withServices: nil,
                options: [CBCentralManagerScanOptionAllowDuplicatesKey: true]
            )
        }
        startStalePruningLoop()
    }

    public func stopScanning() {
        centralManager?.stopScan()
        isScanning = false
        stalePruneTask?.cancel()
        stalePruneTask = nil
    }

    public func startHunt(peripheralId: UUID) {
        huntedPeripheralId = peripheralId
        huntedFilter.reset()
        trendDetector.reset()
        bearingEstimator.reset()
        huntedTrend = .steady
        bearingEstimate = bearingEstimator.estimate()
        huntedReading = readingMap[peripheralId]
        if let current = readingMap[peripheralId] {
            huntedFilter.update(rawRssi: Double(current.rssi))
        }
        startScanning()
        startCompassAndMotion()
        startGeigerLoop()
    }

    public func resetGuidedTurn() {
        bearingEstimator.reset()
        bearingEstimate = bearingEstimator.estimate()
    }

    public func stopHunt() {
        huntedPeripheralId = nil
        geigerTask?.cancel()
        geigerTask = nil
        stopCompassAndMotion()
    }

    private func ensureCentral() {
        if centralManager == nil {
            centralManager = CBCentralManager(delegate: self, queue: nil)
        }
    }

    private func startCompassAndMotion() {
        if CLLocationManager.headingAvailable() {
            locationManager.startUpdatingHeading()
        }
        if motionManager.isDeviceMotionAvailable {
            motionManager.deviceMotionUpdateInterval = 0.1
            motionManager.startDeviceMotionUpdates(to: .main) { [weak self] motion, _ in
                guard let self, let motion else { return }
                // Cabeceo aproximado: 0° plano sobre mesa, 90° vertical frente al usuario.
                let pitchRad = motion.attitude.pitch
                self.currentPitchDeg = pitchRad * 180.0 / Double.pi
            }
        }
    }

    private func stopCompassAndMotion() {
        locationManager.stopUpdatingHeading()
        motionManager.stopDeviceMotionUpdates()
    }

    private func startStalePruningLoop() {
        stalePruneTask?.cancel()
        stalePruneTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 3_000_000_000)
                guard let self else { break }
                let cutoff = Date().addingTimeInterval(-15)
                self.readingMap = self.readingMap.filter { $0.value.lastSeen >= cutoff }
                self.publishSortedPeripherals()
            }
        }
    }

    private func startGeigerLoop() {
        geigerTask?.cancel()
        #if canImport(UIKit)
        impactGenerator.prepare()
        #endif
        geigerTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { break }
                let intervalMs: UInt64
                if let hunted = self.huntedReading, Date().timeIntervalSince(hunted.lastSeen) < 6.0 {
                    let frac = DistanceModel.proximityFraction(rssi: hunted.smoothedRssi)
                    #if canImport(VigiaCore)
                    intervalMs = UInt64(max(110, MobileGeigerIntervalMillis(hunted.smoothedRssi)))
                    #else
                    // De ~1100 ms lejos hasta ~120 ms encima del dispositivo.
                    intervalMs = UInt64(1100.0 - frac * 980.0)
                    #endif
                    #if canImport(UIKit)
                    if self.geigerHapticEnabled {
                        let style: UIImpactFeedbackGenerator.FeedbackStyle = frac > 0.72 ? .heavy : (frac > 0.4 ? .medium : .light)
                        let gen = UIImpactFeedbackGenerator(style: style)
                        gen.impactOccurred(intensity: CGFloat(0.45 + frac * 0.55))
                    }
                    #endif
                    if self.geigerSoundEnabled {
                        AudioServicesPlaySystemSound(1104)
                    }
                } else {
                    intervalMs = 800
                }
                try? await Task.sleep(nanoseconds: max(110, intervalMs) * 1_000_000)
            }
        }
    }

    private func publishSortedPeripherals() {
        peripherals = readingMap.values.sorted { lhs, rhs in
            if lhs.isNamed != rhs.isNamed {
                return lhs.isNamed
            }
            return lhs.smoothedRssi > rhs.smoothedRssi
        }
    }

    private func parseCompanyId(from advertisementData: [String: Any]) -> Int? {
        guard let mfg = advertisementData[CBAdvertisementDataManufacturerDataKey] as? Data, mfg.count >= 2 else {
            return nil
        }
        return Int(mfg[0]) | (Int(mfg[1]) << 8)
    }
}

extension RadarBleEngine: CBCentralManagerDelegate {
    nonisolated public func centralManagerDidUpdateState(_ central: CBCentralManager) {
        let state = central.state
        let auth = CBCentralManager.authorization
        Task { @MainActor [weak self] in
            guard let self else { return }
            self.bluetoothState = state
            self.authorizationState = auth
            if state == .poweredOn && self.isScanning {
                central.scanForPeripherals(
                    withServices: nil,
                    options: [CBCentralManagerScanOptionAllowDuplicatesKey: true]
                )
            }
        }
    }

    nonisolated public func centralManager(
        _ central: CBCentralManager,
        didDiscover peripheral: CBPeripheral,
        advertisementData: [String: Any],
        rssi RSSI: NSNumber
    ) {
        let rssiInt = RSSI.intValue
        guard rssiInt != 127, (-110 ... -10).contains(rssiInt) else { return }

        let uuid = peripheral.identifier
        let advName = (advertisementData[CBAdvertisementDataLocalNameKey] as? String) ?? peripheral.name
        let txPower = advertisementData[CBAdvertisementDataTxPowerLevelKey] as? Int
        let connectable = (advertisementData[CBAdvertisementDataIsConnectable] as? Bool) ?? true
        let uuids = (advertisementData[CBAdvertisementDataServiceUUIDsKey] as? [CBUUID])?.map(\.uuidString) ?? []
        let mfgData = advertisementData[CBAdvertisementDataManufacturerDataKey] as? Data

        Task { @MainActor [weak self] in
            guard let self else { return }
            var companyId: Int?
            if let mfgData, mfgData.count >= 2 {
                companyId = Int(mfgData[0]) | (Int(mfgData[1]) << 8)
            }
            let brandName = companyId.flatMap { self.companyCatalog.brand(for: $0) }

            var filter = self.filtersById[uuid] ?? KalmanRssiFilter()
            let smoothed = filter.update(rawRssi: Double(rssiInt))
            self.filtersById[uuid] = filter

            let matched = BleLanKinship.matchLanDevice(
                bleName: advName,
                bleBrand: brandName,
                lanDevices: self.knownLanDevices
            )

            let reading = BlePeripheralReading(
                id: uuid,
                name: advName,
                companyId: companyId,
                brandName: brandName,
                rssi: rssiInt,
                smoothedRssi: smoothed,
                txPower: txPower,
                serviceUuids: uuids,
                isConnectable: connectable,
                lastSeen: Date(),
                matchedLanDeviceIp: matched?.lastIp,
                matchedLanDeviceName: matched?.displayName
            )
            self.readingMap[uuid] = reading
            self.publishSortedPeripherals()

            if self.huntedPeripheralId == uuid {
                let huntSmoothed = self.huntedFilter.update(rawRssi: Double(rssiInt))
                var huntedCopy = reading
                huntedCopy.smoothedRssi = huntSmoothed
                self.huntedReading = huntedCopy
                self.huntedTrend = self.trendDetector.add(rssi: huntSmoothed)
                self.bearingEstimator.addSample(headingDegrees: self.currentHeadingDeg, smoothedRssi: huntSmoothed)
                self.bearingEstimate = self.bearingEstimator.estimate()
            }
        }
    }
}

extension RadarBleEngine: CLLocationManagerDelegate {
    nonisolated public func locationManager(_ manager: CLLocationManager, didUpdateHeading newHeading: CLHeading) {
        let heading = newHeading.trueHeading >= 0 ? newHeading.trueHeading : newHeading.magneticHeading
        guard heading >= 0 else { return }
        Task { @MainActor [weak self] in
            self?.currentHeadingDeg = CosineBearingEstimator.normalizeDegrees(heading)
        }
    }
}
#endif
