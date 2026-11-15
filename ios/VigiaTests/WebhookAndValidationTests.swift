import XCTest
@testable import Vigia

final class WebhookAndValidationTests: XCTestCase {
    func testWebhookUrlValidatorMatchesAndroidSsrfRules() {
        // Allowed destinations
        XCTAssertEqual(WebhookUrlValidator.validate("https://ntfy.sh/vigia-test-topic"), .valid)
        XCTAssertEqual(WebhookUrlValidator.validate("https://api.telegram.org/bot123456:ABC/sendMessage?chat_id=1"), .valid)
        XCTAssertEqual(WebhookUrlValidator.validate("http://192.168.1.20:8123/api/webhook/vigia"), .valid)
        XCTAssertEqual(WebhookUrlValidator.validate("https://hooks.example.com:8443/alert"), .valid)

        // Empty / invalid scheme / malformed
        XCTAssertFalse(WebhookUrlValidator.validate("   ").isValid)
        XCTAssertNotNil(WebhookUrlValidator.validate("   ").errorMessage)
        XCTAssertFalse(WebhookUrlValidator.validate("ftp://example.com/hook").isValid)
        XCTAssertFalse(WebhookUrlValidator.validate("javascript:alert(1)").isValid)
        XCTAssertFalse(WebhookUrlValidator.validate("https://example.com:70000/hook").isValid)

        // SSRF disallowed local & link-local hosts
        let disallowed = [
            "http://localhost/hook",
            "https://sub.localhost:8080/hook",
            "http://ip6-localhost/hook",
            "http://127.0.0.1:8080/hook",
            "http://127.10.20.30/hook",
            "http://0.0.0.0:8080/hook",
            "http://169.254.169.254/latest/meta-data/",
            "http://[::1]:8080/hook",
            "http://[fe80::1]/hook",
            "http://[::ffff:127.0.0.1]/hook",
            "http://[::ffff:169.254.169.254]/hook",
            "http://0x7f000001/hook",
            "http://2130706433/hook"
        ]
        for url in disallowed {
            let res = WebhookUrlValidator.validate(url)
            XCTAssertFalse(res.isValid, "Expected invalid for \(url)")
            XCTAssertNotNil(res.errorMessage, "Expected non-empty errorMessage for \(url)")
            XCTAssertFalse(WebhookUrlValidator.isValid(url))
        }
    }

    func testPushWebhookPresets() {
        let topicA = PushWebhookPreset.generateNtfyTopic()
        let topicB = PushWebhookPreset.generateNtfyTopic()
        XCTAssertTrue(topicA.hasPrefix("vigia-alertas-"))
        XCTAssertNotEqual(topicA, topicB)

        XCTAssertEqual(
            PushWebhookPreset.ntfyWebhookUrl(topic: "/my-topic/", serverBaseUrl: "https://ntfy.sh/"),
            "https://ntfy.sh/my-topic"
        )
        XCTAssertEqual(
            PushWebhookPreset.telegramWebhookUrl(botToken: "123:ABC", chatId: "987"),
            "https://api.telegram.org/bot123:ABC/sendMessage?chat_id=987"
        )
        XCTAssertEqual(
            PushWebhookPreset.homeAssistantWebhookUrl(baseUrl: "http://192.168.1.20:8123/", webhookId: "/vigia_hook/"),
            "http://192.168.1.20:8123/api/webhook/vigia_hook"
        )
        XCTAssertTrue(PushWebhookPreset.defaultUrgentAlertTypes.contains(AlertType.newDevice.rawValue))
        XCTAssertTrue(PushWebhookPreset.defaultUrgentAlertTypes.contains(AlertType.internetDown.rawValue))
    }

    func testAgentReachabilityCandidateOrderingAndPathEncoding() {
        let link = AgentLink(
            agentId: "a1",
            agentName: "Home Agent",
            host: "192.168.1.50",
            port: 8787,
            token: "secret",
            certSha256: "abc"
        )
        let urls = AgentReachability.candidateBaseUrls(
            link: link,
            savedAddresses: ["192.168.1.50", "8.8.8.8", "100.64.0.12", "10.0.0.5"]
        )
        // Primary host first, then Tailscale before additional LAN, public IP filtered out
        XCTAssertEqual(urls, [
            "https://192.168.1.50:8787",
            "https://100.64.0.12:8787",
            "https://10.0.0.5:8787"
        ])

        XCTAssertEqual(AgentReachability.httpsBaseUrl(host: "fd7a:115c:a1e0::1", port: 8787), "https://[fd7a:115c:a1e0::1]:8787")
        XCTAssertTrue(AgentReachability.isTailscale("100.64.0.1"))
        XCTAssertTrue(AgentReachability.isTailscale("100.127.255.254"))
        XCTAssertFalse(AgentReachability.isTailscale("100.63.255.254"))
        XCTAssertFalse(AgentReachability.isLanOrTailscale("1.1.1.1"))

        XCTAssertEqual(AgentApiClient.encodePathSegment("finding/id with space"), "finding%2Fid%20with%20space")
    }

    func testKeychainStoreErrorDescriptionsAreNonEmpty() {
        XCTAssertFalse(KeychainStoreError.encodingFailed.localizedDescription.isEmpty)
        XCTAssertFalse(KeychainStoreError.unexpectedStatus(-25300).localizedDescription.isEmpty)
    }
}
