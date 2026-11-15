import SwiftUI
import UIKit

@main
public struct VigiaApp: App {
    @StateObject private var store = AppStore()
    @Environment(\.scenePhase) private var scenePhase

    public init() {
        BackgroundAlertManager.shared.registerBackgroundTasks()
        Self.configureAppearance()
    }

    public var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(store)
                .preferredColorScheme(.dark)
                .tint(VigiaColor.radarCyan)
        }
        .onChange(of: scenePhase) { _, newPhase in
            switch newPhase {
            case .active:
                store.handleSceneActive()
            case .background:
                if store.isLinked {
                    BackgroundAlertManager.shared.scheduleNextRefresh()
                }
            default:
                break
            }
        }
    }

    private static func configureAppearance() {
        let navAppearance = UINavigationBarAppearance()
        navAppearance.configureWithOpaqueBackground()
        navAppearance.backgroundColor = UIColor(VigiaColor.background)
        navAppearance.titleTextAttributes = [.foregroundColor: UIColor(VigiaColor.textPrimary)]
        navAppearance.largeTitleTextAttributes = [.foregroundColor: UIColor(VigiaColor.textPrimary)]
        navAppearance.shadowColor = UIColor(VigiaColor.outline)
        UINavigationBar.appearance().standardAppearance = navAppearance
        UINavigationBar.appearance().scrollEdgeAppearance = navAppearance

        let tabAppearance = UITabBarAppearance()
        tabAppearance.configureWithOpaqueBackground()
        tabAppearance.backgroundColor = UIColor(VigiaColor.surface)
        tabAppearance.shadowColor = UIColor(VigiaColor.outline)
        UITabBar.appearance().standardAppearance = tabAppearance
        UITabBar.appearance().scrollEdgeAppearance = tabAppearance
    }
}
