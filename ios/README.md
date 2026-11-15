# Vigía para iOS (iPhone + WidgetKit)

Aplicación nativa para iPhone (`iOS 17+`) construida con Swift, SwiftUI, CoreBluetooth, NetworkExtension, BackgroundTasks y WidgetKit. Comparte el motor de red en Go (`VigiaCore.xcframework` vía `gomobile`) y el contrato **API v1.5** del Agente Vigía 24/7.

> **Estado actual:** La app iOS compila y pasa la construcción en simulador de Xcode en CI; las pruebas funcionales en un iPhone físico están pendientes (requieren emparejamiento inicial con Xcode y cuenta Apple Developer con permisos de Wi-Fi/App Groups, además de compilar `VigiaCore.xcframework` para el motor Go completo).

## Arquitectura

- **Modo local (sin agente):** Escanea tu subred Wi-Fi actual mediante `VigiaEngineBridge` (usando `VigiaCore.xcframework` si está enlazado, o el respaldo en Swift puro), consulta la información de tu Wi-Fi actual (`NEHotspotNetwork`), evalúa hallazgos de seguridad y busca dispositivos Bluetooth LE cercanos con el radar de proximidad.
- **Modo vigilado 24/7 (con Agente Vigía):** Descubre automáticamente agentes en tu LAN mediante Bonjour (`_vigia._tcp`), fija la huella SHA-256 del certificado TLS en el emparejamiento (TOFU + Keychain), sincroniza inventario de equipos, seguridad, estado de internet/speedtest y eventos en vivo por SSE (`/api/v1/stream`), además de soportar acceso remoto seguro vía Tailscale o webhooks push instantáneos (`ntfy.sh`, Telegram, Home Assistant).

## Estructura

```text
ios/
├── project.yml                         # Especificación XcodeGen (app + widget)
├── README.md
├── Vigia/
│   ├── Info.plist
│   ├── Vigia.entitlements              # App Group + Access WiFi Information
│   ├── Resources/
│   │   ├── Assets.xcassets
│   │   └── bt_companies.tsv            # Tabla Bluetooth SIG de fabricantes
│   ├── Bridge/                         # Modelos Codable v1.5 + puente a VigiaCore
│   ├── Platform/                       # Keychain, TLS pinning/SSE, BLE radar, Wi-Fi, BGTask
│   └── UI/                             # Vistas SwiftUI ("Radar Nocturno")
└── VigiaWidget/
    ├── Info.plist
    ├── VigiaWidget.entitlements        # App Group group.com.baxner.vigia
    └── VigiaWidget.swift               # systemSmall, systemMedium, accessoryRectangular
```

## Compilar el motor Go (`VigiaCore.xcframework`, opcional)

El proyecto compila limpiamente **con o sin** `VigiaCore.xcframework` gracias a `#if canImport(VigiaCore)` en `Vigia/Bridge/VigiaEngineBridge.swift`. Para habilitar el motor completo en Go (`ARP`/`NDP`/mDNS/SSDP/huella de fabricante OUI y modelo):

1. Instala Go y `gomobile`:
   ```bash
   go install golang.org/x/mobile/cmd/gomobile@latest
   gomobile init
   ```
2. Desde la raíz del repositorio, genera el XCFramework:
   ```bash
   mkdir -p ios/Frameworks
   gomobile bind -target=ios -o ios/Frameworks/VigiaCore.xcframework ./mobile
   ```
3. En `ios/project.yml`, descomenta las líneas del bloque `Frameworks/VigiaCore.xcframework` dentro de `targets.Vigia.dependencies`.

## Generar el proyecto Xcode

1. Instala [XcodeGen](https://github.com/yonaskolb/XcodeGen):
   ```bash
   brew install xcodegen
   ```
2. Genera `Vigia.xcodeproj`:
   ```bash
   cd ios
   xcodegen generate
   open Vigia.xcodeproj
   ```
3. Configura tu `Signing & Capabilities` (`DEVELOPMENT_TEAM`) en ambos targets (`Vigia` y `VigiaWidget`).
   - Verifica que ambos targets compartan el App Group `group.com.baxner.vigia`.
   - En el target `Vigia`, la capacidad **Access WiFi Information** (`com.apple.developer.networking.wifi-info`) requiere una cuenta de Apple Developer con aprovisionamiento para dispositivo físico.

## Verificación de compilación sin XcodeGen (CLI)

Puedes validar todos los tipos y sintaxis Swift contra el SDK de iOS Simulator directamente con `swiftc`:

```bash
xcrun --sdk iphonesimulator swiftc -typecheck -target x86_64-apple-ios17.0-simulator -swift-version 5 \
  ios/Vigia/Bridge/*.swift \
  ios/Vigia/Platform/*.swift \
  ios/Vigia/UI/*/*.swift

xcrun --sdk iphonesimulator swiftc -typecheck -target x86_64-apple-ios17.0-simulator -swift-version 5 \
  ios/Vigia/Bridge/WidgetModels.swift \
  ios/VigiaWidget/*.swift
```

## Diferencias y límites transparentes de iOS vs Android

- **Wi-Fi vecinos:** A diferencia de Android (`WifiManager.scanResults`), iOS restringe la enumeración de puntos de acceso cercanos (`NEHotspotHelper` requiere aprobación especial de Apple orientada a operadores). `HotspotWifiProvider` lee la red actual vía `NEHotspotNetwork.fetchCurrent` y la pestaña **Wi-Fi** explica con transparencia esta restricción de Apple.
- **Mapeo WPA3 sin falsos positivos:** `NEHotspotNetworkSecurityType.personal` agrupa WPA2 y WPA3 Personal en iOS. Para evitar falsos positivos `WIFI_WPA2` en routers WPA3 modernos, `.personal` se reporta como `""` al motor de seguridad, mientras que `.open` y `.wep` sí activan sus alertas reales.
- **Tabla ARP:** iOS bloquea la lectura directa de la caché ARP desde iOS 11+; en modo local el respaldo Swift descubre hosts activos mediante sondeo TCP concurrente y, en modo 24/7, el Agente en tu servidor aporta el inventario completo por ARP/NDP/mDNS.
- **Alertas con la app cerrada:** Además de `BGAppRefreshTask` (`com.baxner.vigia.alerts.refresh`), **Webhooks** ofrece un asistente de 1 toque para registrar notificaciones push instantáneas en `ntfy.sh`, Telegram o Home Assistant directamente en el Agente.
