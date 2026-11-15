# Vigía

**Monitor de red local 100 % privado, sin nube ni suscripciones.**

Vigía descubre, identifica y vigila los dispositivos de tu red local en tiempo real. Combina un motor compartido en **Go** (`core/`), un **Agente 24/7** multiplataforma (macOS, Linux / Raspberry Pi y Docker) y aplicaciones nativas para **Android** (Kotlin + Jetpack Compose + Widget Glance) e **iPhone / iOS** (Swift + SwiftUI + WidgetKit).

---

## Principios de privacidad

- **Sin nube ni cuentas:** el inventario de equipos, direcciones MAC, puertos detectados, eventos históricos, alertas y reglas viven únicamente en tus dispositivos y en tu propio agente.
- **Sondas opcionales de Internet (Fase 7):** el medidor de calidad de Internet del agente realiza únicamente pruebas técnicas de conectividad (`1.1.1.1`, `8.8.8.8` en TCP/443, `[2606:4700:4700::1111]:443` para IPv6) y resolución DNS / pruebas de velocidad opcionales contra `speed.cloudflare.com`, **sin enviar nunca datos de tu red local**, direcciones MAC ni inventarios de equipos. Las pruebas automáticas de velocidad pueden desactivarse (`speedTestsPerDay: 0`).
- **Comunicación cifrada y fijada:** la conexión entre apps móviles y el Agente 24/7 utiliza HTTPS con certificado autofirmado ECDSA P-256 y fijación (*pinning*) de huella SHA-256 en almacenamiento seguro del teléfono (Android Keystore / iOS Keychain), tanto en LAN como vía Tailscale.

---

## Arquitectura

| Módulo | Directorio | Tecnología | Función principal |
|---|---|---|---|
| **Motor de red** | `core/` | Go 1.26 (`//go:embed`) | Escaneo paralelo (ICMP, TCP, mDNS, SSDP, NetBIOS, Tuya), catálogo OUI IEEE, reconocimiento de marca/modelo/tipo, escaneo de puertos, consulta UPnP / NAT-PMP e informe de seguridad |
| **Puente móvil** | `mobile/` | Go + `gomobile` | API JSON unificada compilada a `.aar` (Android) y `.xcframework` (iOS) |
| **Agente 24/7** | `agent/`, `cmd/vigia-agent/` | Go + SQLite (`modernc.org/sqlite`) | Vigilancia continua, historial en SQLite, alertas, webhooks HMAC, anillo exterior, medidor de Internet, TLS mux y SSE en vivo; instalable en macOS, Linux (`systemd`) y Docker |
| **CLI de prueba** | `cmd/vigia-scan/` | Go | Escaneo rápido de la red desde terminal (`-noports`, `-json`, `-cidr`) |
| **App Android** | `android/` | Kotlin, Jetpack Compose, Room, WorkManager, Glance | Escáner autónomo de LAN, modo Agente 24/7 (LAN / Tailscale), radar Wi-Fi / Bluetooth LE con Geiger y vuelta guiada, alertas en vivo y widget |
| **App iOS (iPhone)** | `ios/` | Swift, SwiftUI, CoreBluetooth, CoreMotion, WidgetKit | Escáner LAN compatible con permisos de Apple, modo Agente 24/7 (LAN / Tailscale), radar BLE con brújula/Geiger, webhooks push y widget |

---

## Características por módulo

### 1. Descubrimiento y reconocimiento (`core/`)
- Barrido híbrido rápido (ARP/NDP cuando el sistema lo permite, ICMP echo sin privilegios, TCP connect a puertos representativos, mDNS/Bonjour, SSDP/UPnP, NetBIOS, fingerprinting HTTP y protocolo local Tuya).
- Reconocimiento automático de fabricante por OUI (`core/data/oui.txt`), modelos Apple (`core/data/apple-models.txt`), tipo de equipo (router, teléfono, computadora, TV, altavoz, consola, cámara, IoT) y nivel de confianza.
- Escaneo de puertos bajo demanda con catálogo descriptivo en español (`core/data/ports.tsv`) de riesgo y recomendaciones accionables.

### 2. Seguridad y auditoría del router (`core/`, `agent/`)
- Puntaje de seguridad (`0–100`) con hallazgos priorizados y pasos concretos de mitigación.
- Inspección automática del router por **UPnP IGD** y **NAT-PMP**: detección de redirecciones de puertos activas y estado de exposición (`outer_ring` vs `internet`).
- **Anillo exterior** opcional: detecta y vigila el segmento entre tu router y el módem/ONT del proveedor.
- **Hallazgos reconocidos (*"Ya lo sé, es a propósito"*):** permite aceptar servicios intencionales sin penalizar el puntaje.
- Alertas de cambio de MAC del router (`ROUTER_CHANGED`), posible suplantación ARP (`ARP_CONFLICT`), nuevas redirecciones UPnP (`PORT_MAPPING_ADDED`) y servicios de riesgo nuevos (`RISKY_SERVICE`).

### 3. Agente 24/7, alertas y webhooks (`agent/`, `cmd/vigia-agent/`)
- Escaneos programados (cada 5 min por defecto), detección de suspensión del host (`agent/wake`) para evitar alertas falsas tras hibernación y reconciliación estable por MAC / UDN UPnP / nombre / IP.
- Flujo **Server-Sent Events** (`/api/v1/stream`) y **Webhooks** con firma `HMAC-SHA256`, reintentos exponenciales y protección anti-SSRF (bloquea loopback, direcciones no especificadas, link-local y redirecciones).
- Medidor de **Internet (Fase 7)**: latencia por tramos (*router → módem → internet* y *DNS*) cada 30 s, detección de caídas con atribución de culpable (`lan`, `modem`, `isp`), verificación de salida IPv6 y pruebas de velocidad opcionales con Cloudflare.
- Emparejamiento de un solo uso con código de 6 dígitos (`vigia-agent pair`), tokens almacenados como hash SHA-256 y soporte multiplataforma (`LaunchAgent` en macOS, `systemd` en Linux/Raspberry Pi y `Dockerfile` / `compose.yml` con `network_mode: host`).

### 4. Apps móviles nativas (`android/` e `ios/`)
- **Funcionamiento autónomo o vinculado:** escanea cualquier red Wi-Fi al instante sin requerir servidor, o vincúlate al Agente 24/7 en casa para historial continuo y acceso remoto vía Tailscale.
- **Radar / Buscar señal:** detector frío/caliente, pulso acústico/háptico estilo **Geiger** y **vuelta guiada** con estimación de rumbo usando la brújula y el giroscopio del teléfono.
- **Widgets de pantalla de inicio:** Jetpack Glance en Android y WidgetKit en iPhone.
- **Transparencia sobre iOS:** por restricciones de privacidad de Apple, en iPhone las apps estándar no pueden leer la tabla ARP del sistema ni listar redes Wi-Fi vecinas (solo el SSID/BSSID de la red actual mediante `NEHotspotNetwork` con el entitlement correspondiente). El radar BLE funciona en tiempo real con `CoreBluetooth` y las alertas con la app cerrada se resuelven mediante `BGAppRefreshTask` + webhooks push (`ntfy`, Telegram, Home Assistant). *(Nota: el target iOS compila y pasa Xcode/Simulator en CI; las pruebas en hardware iPhone físico están pendientes.)*

---

## Inicio rápido

### 1. Probar el motor desde terminal (sin instalar nada)

```sh
git clone https://github.com/baxner/vigia.git
cd vigia
go run ./cmd/vigia-scan -noports
```

### 2. Ejecutar las pruebas del motor y del agente

```sh
go vet ./...
go test -race ./...
```

### 3. Desplegar el Agente 24/7

Elige tu plataforma en **[docs/instalacion.md](docs/instalacion.md)**:

- **Docker (Linux / Raspberry Pi / NAS):**
  ```sh
  docker compose up -d
  docker compose exec vigia-agent vigia-agent pair -data /data
  ```
- **macOS (LaunchAgent) o Linux (`systemd`):**
  ```sh
  go build -o vigia-agent ./cmd/vigia-agent
  ./vigia-agent install -interval 5m
  vigia-agent pair
  ```

### 4. Compilar las apps móviles

- **Android:** ejecuta `./scripts/build-aar.sh` y luego `cd android && ./gradlew assembleDebug` (ver [docs/instalacion.md](docs/instalacion.md#5-compilar-la-app-android)).
- **iPhone (iOS):** opcionalmente genera `ios/Frameworks/VigiaCore.xcframework` con `./scripts/build-xcframework.sh`, luego genera el proyecto con `cd ios && xcodegen generate` (ver [docs/instalacion.md](docs/instalacion.md#6-compilar-la-app-ios-iphone) e [ios/README.md](ios/README.md)).

---

## Documentación

- **[Guía de instalación y compilación](docs/instalacion.md)** — macOS (`LaunchAgent` + firma estable), Linux / Raspberry Pi (`systemd`), Docker (`network_mode: host`), Android e iOS.
- **[Contrato de la API del Agente (v1.5)](docs/agente-api.md)** — Endpoints REST, eventos SSE, alertas, webhooks, seguridad por anillos, medidor de Internet, TLS pinning y CLI.
- **[Política de Seguridad](SECURITY.md)** — Reporte coordinado de vulnerabilidades y modelo de amenazas.
- **[Guía de Contribución](CONTRIBUTING.md)** — Reglas de desarrollo, pruebas y prevención de filtraciones de PII/secretos.

---

## Licencias y marca registrada

Vigía utiliza un esquema de **licencia dual** para garantizar la total auditabilidad del código y el uso gratuito en casa, protegiendo al mismo tiempo la distribución comercial y oficial en tiendas de aplicaciones:

- **Motor Go, Agente 24/7, scripts y documentación** (`core/`, `mobile/`, `agent/`, `cmd/`, `scripts/`, `deploy/`, `docs/`): bajo licencia **[Apache License 2.0](LICENSE)** (`Copyright 2026 baxner`).
- **Aplicaciones móviles nativas** (`android/` e `ios/`): bajo licencia **[PolyForm Noncommercial License 1.0.0](android/LICENSE)** ([`android/LICENSE`](android/LICENSE) e [`ios/LICENSE`](ios/LICENSE)). Cualquier persona puede inspeccionar, modificar, compilar y usar las apps Android e iOS gratis en casa para uso personal o no comercial; queda prohibida su distribución o venta comercial por terceros.
- **Marca y distribución oficial:** el nombre **Vigía**, sus logotipos y la publicación oficial de binarios empaquetados en Google Play Store y Apple App Store están reservados exclusivamente al autor (**baxner**). Consulta **[TRADEMARK.md](TRADEMARK.md)** para más detalles.
