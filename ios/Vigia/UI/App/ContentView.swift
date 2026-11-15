import SwiftUI

public enum VigiaTab: Hashable {
    case panel
    case devices
    case wifi
    case bluetooth
}

public struct ContentView: View {
    @EnvironmentObject private var store: AppStore
    @State private var selectedTab: VigiaTab = .panel

    public init() {}

    public var body: some View {
        TabView(selection: $selectedTab) {
            PanelView(selectedTab: $selectedTab)
                .tabItem {
                    Label("Panel", systemImage: "dot.radiowaves.left.and.right")
                }
                .tag(VigiaTab.panel)

            DevicesView()
                .tabItem {
                    Label("Equipos", systemImage: "desktopcomputer.and.iphone")
                }
                .badge(store.onlineDeviceCount > 0 ? "\(store.onlineDeviceCount)" : nil)
                .tag(VigiaTab.devices)

            WifiView(selectedTab: $selectedTab)
                .tabItem {
                    Label("Wi-Fi", systemImage: "wifi")
                }
                .tag(VigiaTab.wifi)

            BluetoothView()
                .tabItem {
                    Label("Bluetooth", systemImage: "wave.3.right")
                }
                .tag(VigiaTab.bluetooth)
        }
        .background(VigiaColor.background.ignoresSafeArea())
    }
}
