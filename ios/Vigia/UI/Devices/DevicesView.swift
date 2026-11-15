import SwiftUI

public enum DeviceFilter: String, CaseIterable, Identifiable {
    case all = "Todos"
    case online = "En línea"
    case risky = "Con alertas"
    case unknown = "Sin identificar"
    case offline = "Desconectados"

    public var id: String { rawValue }
    public var localizedTitle: String { VigiaL10n.tr(rawValue) }
}

public struct DevicesView: View {
    @EnvironmentObject private var store: AppStore
    @State private var searchText = ""
    @State private var filter: DeviceFilter = .all

    public init() {}

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                if store.agentNetworks.count > 1 {
                    networkPickerBar
                }
                filterChipsBar
                deviceListContent
            }
            .background(VigiaColor.background.ignoresSafeArea())
            .navigationTitle("Equipos")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $searchText, prompt: "Buscar por nombre, IP, MAC o marca")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await store.triggerScan() }
                    } label: {
                        if store.isScanning {
                            ProgressView()
                                .tint(VigiaColor.radarCyan)
                                .vigiaMinTouchTarget()
                        } else {
                            Image(systemName: "arrow.clockwise")
                                .vigiaMinTouchTarget()
                        }
                    }
                    .accessibilityLabel(Text(VigiaL10n.tr("Escanear red")))
                }
            }
        }
    }

    private var networkPickerBar: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(store.agentNetworks) { net in
                    let selected = net.key == store.selectedNetworkKey
                    Button {
                        Task { await store.selectNetwork(key: net.key) }
                    } label: {
                        HStack(spacing: 5) {
                            Image(systemName: net.role == "outer" ? "arrow.triangle.branch" : "house")
                                .font(.caption2)
                            Text(net.subnet.isEmpty ? net.key : net.subnet)
                                .font(.system(.caption, design: .monospaced).weight(.semibold))
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .background(
                            Capsule().fill(selected ? VigiaColor.electricBlue.opacity(0.22) : VigiaColor.surface)
                                .overlay(Capsule().strokeBorder(selected ? VigiaColor.electricBlue : VigiaColor.outline, lineWidth: 1))
                        )
                        .foregroundStyle(selected ? VigiaColor.electricBlue : VigiaColor.textSecondary)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                    }
                    .accessibilityAddTraits(selected ? [.isSelected] : [])
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
        }
    }

    private var filterChipsBar: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(DeviceFilter.allCases) { item in
                    let selected = filter == item
                    Button {
                        filter = item
                    } label: {
                        Text("\(item.localizedTitle) (\(count(for: item)))")
                            .font(.caption.weight(.semibold))
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .background(
                                Capsule()
                                    .fill(selected ? VigiaColor.radarCyan.opacity(0.2) : VigiaColor.surface)
                                    .overlay(
                                        Capsule().strokeBorder(
                                            selected ? VigiaColor.radarCyan : VigiaColor.outline,
                                            lineWidth: 1
                                        )
                                    )
                            )
                            .foregroundStyle(selected ? VigiaColor.radarCyan : VigiaColor.textSecondary)
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .accessibilityAddTraits(selected ? [.isSelected] : [])
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
        }
    }

    @ViewBuilder
    private var deviceListContent: some View {
        let items = filteredDevices
        if items.isEmpty {
            ScrollView {
                VigiaEmptyState(
                    symbol: "desktopcomputer.trianglebadge.exclamationmark",
                    title: store.devices.isEmpty ? VigiaL10n.tr("Sin equipos todavía") : VigiaL10n.tr("Ningún equipo coincide"),
                    message: store.devices.isEmpty
                        ? VigiaL10n.tr("Toca Escanear en el Panel para descubrir los equipos conectados a tu red.")
                        : VigiaL10n.tr("Prueba cambiar el filtro o limpiar el texto de búsqueda."),
                    actionTitle: store.devices.isEmpty ? VigiaL10n.tr("Escanear red") : VigiaL10n.tr("Limpiar filtros")
                ) {
                    if store.devices.isEmpty {
                        Task { await store.triggerScan() }
                    } else {
                        filter = .all
                        searchText = ""
                    }
                }
                .padding(16)
            }
        } else {
            List {
                ForEach(items) { device in
                    NavigationLink {
                        DeviceDetailView(initialDevice: device)
                    } label: {
                        DeviceRowView(device: device)
                    }
                    .listRowBackground(VigiaColor.surface)
                    .listRowSeparatorTint(VigiaColor.outline)
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .refreshable {
                await store.triggerScan()
            }
        }
    }

    private func count(for filter: DeviceFilter) -> Int {
        store.devices.filter { matchesFilter($0, filter: filter) }.count
    }

    private func matchesFilter(_ device: KnownDevice, filter: DeviceFilter) -> Bool {
        switch filter {
        case .all:
            return true
        case .online:
            return device.online
        case .risky:
            return !VigiaEngineBridge.shared.riskyPorts(among: device.last.openPorts).isEmpty
        case .unknown:
            return device.effectiveType == .unknown
        case .offline:
            return !device.online
        }
    }

    private var filteredDevices: [KnownDevice] {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return store.devices
            .filter { matchesFilter($0, filter: filter) }
            .filter { device in
                guard !query.isEmpty else { return true }
                return device.displayName.lowercased().contains(query)
                    || device.lastIp.lowercased().contains(query)
                    || (device.mac?.lowercased().contains(query) ?? false)
                    || (device.last.vendor?.lowercased().contains(query) ?? false)
                    || device.last.identity.brand.lowercased().contains(query)
                    || (device.custom.location?.lowercased().contains(query) ?? false)
            }
            .sorted { lhs, rhs in
                if lhs.online != rhs.online { return lhs.online }
                if lhs.custom.important != rhs.custom.important { return lhs.custom.important }
                return VigiaRadarLayout.ipSortKey(lhs.lastIp) < VigiaRadarLayout.ipSortKey(rhs.lastIp)
            }
    }
}

private struct DeviceRowView: View {
    let device: KnownDevice

    var body: some View {
        let color = device.statusColor
        let risky = VigiaEngineBridge.shared.riskyPorts(among: device.last.openPorts)

        HStack(spacing: 12) {
            ZStack {
                Circle()
                    .fill(color.opacity(0.15))
                    .frame(width: 42, height: 42)
                Image(systemName: device.effectiveType.sfSymbol)
                    .foregroundStyle(color)
                    .font(.system(size: 18, weight: .semibold))
            }

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 5) {
                    Text(device.displayName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(device.online ? VigiaColor.textPrimary : VigiaColor.textSecondary)
                        .lineLimit(1)
                    if device.custom.important {
                        Image(systemName: "star.fill")
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.amber)
                    }
                    if let loc = device.custom.location, !loc.isEmpty {
                        Text(loc)
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 1)
                            .background(Capsule().fill(VigiaColor.surfaceElevated))
                    }
                }

                HStack(spacing: 8) {
                    Text(device.lastIp)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(VigiaColor.textSecondary)
                    if let subtitle = subtitleText {
                        Text("· \(subtitle)")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textTertiary)
                            .lineLimit(1)
                    }
                }
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 4) {
                if !risky.isEmpty {
                    VigiaStatusChip(VigiaL10n.fmt("%ld riesgo", risky.count), color: VigiaColor.risk)
                } else if !device.online {
                    Text("Offline")
                        .font(.caption2.weight(.medium))
                        .foregroundStyle(VigiaColor.textTertiary)
                } else if device.effectiveType == .unknown {
                    VigiaStatusChip(VigiaL10n.tr("Sin identificar"), color: VigiaColor.amber)
                } else if !device.last.openPorts.isEmpty {
                    Text(VigiaL10n.fmt("%ld p.", device.last.openPorts.count))
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(VigiaColor.textSecondary)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private var subtitleText: String? {
        let brand = device.last.identity.brand.trimmingCharacters(in: .whitespacesAndNewlines)
        if !brand.isEmpty { return brand }
        if let vendor = device.last.vendor, !vendor.isEmpty { return vendor }
        return device.effectiveType == .unknown ? nil : device.effectiveType.labelEs
    }
}
