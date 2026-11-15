import Foundation
import Security

/// Error de acceso o escritura al Keychain de iOS.
public enum KeychainStoreError: LocalizedError, Sendable {
    case unexpectedStatus(OSStatus)
    case encodingFailed

    public var errorDescription: String? {
        switch self {
        case .unexpectedStatus(let status):
            let message = SecCopyErrorMessageString(status, nil) as String? ?? VigiaL10n.fmt("Código %ld", Int(status))
            return VigiaL10n.fmt("Error de Keychain: %@", message)
        case .encodingFailed:
            return VigiaL10n.tr("No se pudo codificar el vínculo del agente para Keychain.")
        }
    }
}

/// Almacena y recupera `AgentLink` (`agentId`, `agentName`, `host`, `port`, `token`, `certSha256`)
/// de forma segura en el Keychain de iOS mediante `SecItem*`.
public final class KeychainLinkStore: @unchecked Sendable {
    public static let shared = KeychainLinkStore()

    private let service: String
    private let account: String

    public init(service: String = "com.baxner.vigia.agent-link", account: String = "primary") {
        self.service = service
        self.account = account
    }

    public func load() -> AgentLink? {
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
            kSecReturnData: kCFBooleanTrue as Any,
            kSecMatchLimit: kSecMatchLimitOne
        ]

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else {
            return nil
        }
        return try? JSONDecoder().decode(AgentLink.self, from: data)
    }

    public func save(_ link: AgentLink) throws {
        guard let data = try? JSONEncoder().encode(link) else {
            throw KeychainStoreError.encodingFailed
        }

        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account
        ]

        let attributes: [CFString: Any] = [
            kSecValueData: data,
            kSecAttrAccessible: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]

        let updateStatus = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        if updateStatus != errSecItemNotFound {
            throw KeychainStoreError.unexpectedStatus(updateStatus)
        }

        var addQuery = query
        addQuery[kSecValueData] = data
        addQuery[kSecAttrAccessible] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(addQuery as CFDictionary, nil)
        guard addStatus == errSecSuccess else {
            throw KeychainStoreError.unexpectedStatus(addStatus)
        }
    }

    public func clear() throws {
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainStoreError.unexpectedStatus(status)
        }
    }
}

/// Estado auxiliar no secreto guardado en el App Group (`addresses` de `/info`, última red y último ID de alerta).
public enum AgentPreferencesStore {
    private static let addressesKey = "vigia.agent.addresses"
    private static let selectedNetworkKey = "vigia.agent.network_key"
    private static let lastNotifiedAlertIdKey = "vigia.agent.last_notified_alert_id"
    private static let localAcknowledgedKey = "vigia.local.acknowledged_findings"

    public static var savedAddresses: [String] {
        get { AppGroupConstants.sharedDefaults.stringArray(forKey: addressesKey) ?? [] }
        set { AppGroupConstants.sharedDefaults.set(newValue, forKey: addressesKey) }
    }

    public static var selectedNetworkKeyValue: String? {
        get { AppGroupConstants.sharedDefaults.string(forKey: selectedNetworkKey) }
        set { AppGroupConstants.sharedDefaults.set(newValue, forKey: selectedNetworkKey) }
    }

    public static var lastNotifiedAlertId: Int64 {
        get { Int64(AppGroupConstants.sharedDefaults.integer(forKey: lastNotifiedAlertIdKey)) }
        set { AppGroupConstants.sharedDefaults.set(Int(newValue), forKey: lastNotifiedAlertIdKey) }
    }

    public static var localAcknowledgedFindings: [AcknowledgedFinding] {
        get {
            guard let data = AppGroupConstants.sharedDefaults.data(forKey: localAcknowledgedKey),
                  let items = try? JSONDecoder().decode([AcknowledgedFinding].self, from: data) else {
                return []
            }
            return items
        }
        set {
            if let data = try? JSONEncoder().encode(newValue) {
                AppGroupConstants.sharedDefaults.set(data, forKey: localAcknowledgedKey)
            }
        }
    }

    public static func clearAgentSessionData() {
        savedAddresses = []
        selectedNetworkKeyValue = nil
        lastNotifiedAlertId = 0
    }
}
