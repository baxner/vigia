import Foundation

/// Utilidad centralizada de localización (`es.lproj` / `en.lproj`) compartida entre `Vigia`, `VigiaWidget` y pruebas.
public enum VigiaL10n {
    /// Bundle que contiene `Localizable.strings`, priorizando `Bundle.main` y con respaldo para bundles de test.
    public static let resourceBundle: Bundle = {
        if Bundle.main.url(forResource: "Localizable", withExtension: "strings") != nil {
            return .main
        }
        for bundle in Bundle.allBundles where bundle.url(forResource: "Localizable", withExtension: "strings") != nil {
            return bundle
        }
        return .main
    }()

    public static func tr(_ key: String, table: String? = nil, bundle: Bundle = resourceBundle) -> String {
        bundle.localizedString(forKey: key, value: key, table: table)
    }

    public static func fmt(_ key: String, _ args: CVarArg...) -> String {
        let format = tr(key)
        return String(format: format, locale: Locale.current, arguments: args)
    }
}
