import XCTest
@testable import Vigia

final class WidgetSummaryTests: XCTestCase {
    private var suiteName: String!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suiteName = "com.baxner.vigia.tests.widget.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
    }

    override func tearDown() {
        if let suiteName {
            UserDefaults.standard.removePersistentDomain(forName: suiteName)
        }
        defaults = nil
        suiteName = nil
        super.tearDown()
    }

    func testWidgetSummaryLoadReturnsDefaultWhenEmptyOrCorrupt() {
        let empty = WidgetSummaryDto.load(from: defaults)
        XCTAssertEqual(empty, WidgetSummaryDto())
        XCTAssertFalse(empty.linked)
        XCTAssertEqual(empty.onlineDevices, 0)
        XCTAssertEqual(empty.internetState, "unknown")

        defaults.set(Data("not-json".utf8), forKey: AppGroupConstants.widgetSummaryKey)
        let corrupt = WidgetSummaryDto.load(from: defaults)
        XCTAssertEqual(corrupt, WidgetSummaryDto())
    }

    func testWidgetSummaryRoundTripPersistence() {
        let dto = WidgetSummaryDto(
            linked: true,
            onlineDevices: 14,
            unreadAlerts: 2,
            internetState: "up",
            internetLatencyMs: 18,
            updatedAtEpochMs: 1_763_145_600_000
        )
        dto.save(to: defaults)

        let loaded = WidgetSummaryDto.load(from: defaults)
        XCTAssertEqual(loaded, dto)
        XCTAssertTrue(loaded.linked)
        XCTAssertEqual(loaded.onlineDevices, 14)
        XCTAssertEqual(loaded.unreadAlerts, 2)
        XCTAssertEqual(loaded.internetState, "up")
        XCTAssertEqual(loaded.internetLatencyMs, 18)
    }

    func testVigiaDateParsingFormattingAndDurations() {
        let withFrac = VigiaDate.parse("2026-11-14T18:30:45.123Z")
        let noFrac = VigiaDate.parse("2026-11-14T18:30:45Z")
        XCTAssertNotNil(withFrac)
        XCTAssertNotNil(noFrac)

        if let date = noFrac {
            let formatted = VigiaDate.format(date)
            XCTAssertEqual(VigiaDate.parse(formatted)?.timeIntervalSince1970 ?? 0, date.timeIntervalSince1970, accuracy: 1.0)
        }

        XCTAssertEqual(VigiaDate.formatDuration(45), VigiaL10n.fmt("%ld s", 45))
        XCTAssertEqual(VigiaDate.formatDuration(125), VigiaL10n.fmt("%ld min %ld s", 2, 5))
        XCTAssertEqual(VigiaDate.formatDuration(180), VigiaL10n.fmt("%ld min", 3))
        XCTAssertEqual(VigiaDate.formatDuration(3660), VigiaL10n.fmt("%ld h %ld min", 1, 1))
        XCTAssertEqual(VigiaDate.formatDuration(7200), VigiaL10n.fmt("%ld h", 2))

        let now = Date(timeIntervalSince1970: 1_794_767_445)
        let tenMinAgo = VigiaDate.format(now.addingTimeInterval(-600))
        XCTAssertEqual(VigiaDate.relativeDescription(from: tenMinAgo, now: now), VigiaL10n.fmt("hace %ld min", 10))
        let justNow = VigiaDate.format(now.addingTimeInterval(-10))
        XCTAssertEqual(VigiaDate.relativeDescription(from: justNow, now: now), VigiaL10n.tr("hace un momento"))
    }
}
