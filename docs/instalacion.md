# Guía de instalación y compilación de Vigía

Esta guía cubre cómo desplegar el **Agente 24/7** (`vigia-agent`) en macOS, Linux / Raspberry Pi y Docker, así como cómo compilar las aplicaciones nativas para **Android** e **iOS (iPhone)** desde el código fuente.

---

## 1. Requisitos previos generales

- **Para el motor Go y el Agente (`vigia-agent`):** [Go 1.26+](https://go.dev/dl/) (o superior según `go.mod`).
- **Para Android:** JDK 17, Android SDK + NDK, `gomobile` y `gobind`.
- **Para iOS:** macOS con Xcode 16+ / iOS 17+ SDK, `gomobile`, `gobind` y [XcodeGen](https://github.com/yonaskolb/XcodeGen) (`brew install xcodegen`).

Prueba rápida del motor desde la terminal sin instalar nada:

```sh
go run ./cmd/vigia-scan -noports
```

---

## 2. Agente 24/7 en macOS (LaunchAgent y firma local)

### Compilación rápida

```sh
go build -o /tmp/vigia-agent ./cmd/vigia-agent
```

### Instalación automática como servicio de usuario

```sh
/tmp/vigia-agent install -interval 5m
```

Esto realiza automáticamente:
1. Copia el binario CLI a `~/Library/Application Support/Vigia/vigia-agent`.
2. Empaqueta el demonio dentro de `~/Library/Application Support/Vigia/Vigia Agent.app` (necesario porque macOS bloquea por privacidad `NECP` las conexiones a la red local de procesos `launchd` que no pertenecen a un bundle `.app` con `Info.plist`).
3. Crea y carga el LaunchAgent `~/Library/LaunchAgents/com.baxner.vigia.agent.plist` (`RunAtLoad`, `KeepAlive`, `ProcessType=Background`).

Verifica el estado y los registros:

```sh
~/Library/Application\ Support/Vigia/vigia-agent status
tail -f ~/Library/Logs/Vigia/agent.log
```

Si macOS muestra el diálogo **"¿Permitir que Vigia Agent busque dispositivos en redes locales?"**, acéptalo. Si en el estado o los logs aparece una advertencia de red local, habilítalo en **Ajustes del Sistema → Privacidad y seguridad → Red local → Vigia Agent**.

### Firma de código local estable (recomendada en macOS)

Por defecto, `install` usa firma *ad hoc* (`codesign --sign -`). El identificador de una firma *ad hoc* depende del hash del binario exacto, por lo que al recompilar y actualizar `vigia-agent`, macOS puede revocar el permiso de **Red local**.

Para conservar el permiso entre actualizaciones, crea una identidad de firma de código propia en tu llavero una sola vez:

1. Abre **Acceso a Llaveros** (*Keychain Access*) → menú **Asistente de Certificados** → **Crear un certificado…**
2. Asigna un nombre identificable, por ejemplo `Vigia Agent Signing`.
3. Selecciona **Tipo de identidad: Raíz autofirmada** (*Self-Signed Root*) y **Tipo de certificado: Firma de código** (*Code Signing*).
4. Opcionalmente marca *"Dejarme invalidar los valores predeterminados"* para extender la vigencia (p. ej., 10 o 20 años).
5. Guarda el certificado en el llavero **Inicio de sesión** (*login*) y verifica que aparece disponible:
   ```sh
   security find-identity -v -p codesigning
   ```
6. Instala (o reinstala) el agente pasando `-sign`:
   ```sh
   /tmp/vigia-agent install -interval 5m -sign "Vigia Agent Signing"
   ```
7. Confirma que el requisito designado referencia tu certificado y no un `cdhash` suelto:
   ```sh
   codesign -d -r- "$HOME/Library/Application Support/Vigia/Vigia Agent.app"
   ```

### Vinculación y desinstalación en macOS

```sh
# Generar código de 6 dígitos para vincular la app móvil
~/Library/Application\ Support/Vigia/vigia-agent pair

# Ver huella TLS SHA-256
~/Library/Application\ Support/Vigia/vigia-agent cert

# Desinstalar el LaunchAgent (conserva la base de datos y certificados)
~/Library/Application\ Support/Vigia/vigia-agent uninstall
```

---

## 3. Agente 24/7 en Linux / Raspberry Pi (`systemd`)

Puedes compilar directamente en tu servidor/Raspberry Pi o compilar de forma cruzada desde otra máquina:

```sh
# En la propia máquina Linux (x86_64 o ARM64)
go build -o vigia-agent ./cmd/vigia-agent

# O compilación cruzada para Raspberry Pi 64 bits (ARM64)
GOOS=linux GOARCH=arm64 go build -o vigia-agent ./cmd/vigia-agent
```

### Dependencias del sistema recomendadas en Linux

- **mDNS / descubrimiento automático:** instala `avahi-daemon` y `avahi-utils` (`avahi-publish`) para que las apps encuentren el agente vía `_vigia._tcp`. En Debian/Ubuntu/Raspberry Pi OS:
  ```sh
  sudo apt-get update && sudo apt-get install -y avahi-daemon avahi-utils
  ```
- **ICMP sin privilegios (opcional pero recomendado):** verifica que los usuarios sin `root` puedan abrir sockets ICMP `udp4`:
  ```sh
  sysctl net.ipv4.ping_group_range
  ```
  Si el rango no incluye el GID del usuario que ejecuta el agente, permite el rango completo:
  ```sh
  echo "net.ipv4.ping_group_range = 0 2147483647" | sudo tee /etc/sysctl.d/99-vigia-ping.conf
  sudo sysctl --system
  ```

### Opción A: Instalación integrada con `vigia-agent install`

El subcomando `install` crea y habilita la unidad `systemd` automáticamente:

- **Como usuario actual** (instala unidad en `~/.config/systemd/user/vigia-agent.service` y datos en `~/.local/share/vigia`):
  ```sh
  ./vigia-agent install -interval 5m
  ```
  Si deseas que el servicio de usuario arranque al encender el equipo sin sesión iniciada:
  ```sh
  sudo loginctl enable-linger "$USER"
  ```
- **Como servicio de sistema (`root`)** (instala unidad en `/etc/systemd/system/vigia-agent.service`):
  ```sh
  sudo install -m 0755 vigia-agent /usr/local/bin/vigia-agent
  sudo /usr/local/bin/vigia-agent install -interval 5m
  ```

Para desinstalar:

```sh
vigia-agent uninstall
```

### Opción B: Instalación manual con `systemd`

Si prefieres gestionar la unidad manualmente usando la plantilla incluida en `deploy/systemd/vigia-agent.service`:

1. Instala el binario y crea el directorio de datos dedicado:
   ```sh
   sudo install -m 0755 vigia-agent /usr/local/bin/vigia-agent
   sudo useradd --system --no-create-home --shell /usr/sbin/nologin vigia || true
   sudo mkdir -p /var/lib/vigia
   sudo chown vigia:vigia /var/lib/vigia
   sudo chmod 0700 /var/lib/vigia
   ```
2. Copia y revisa la unidad en `/etc/systemd/system/vigia-agent.service` (ajustando `-data /var/lib/vigia`, `-interval`, `-port` o `-bind` si necesitas restringir la IP de escucha):
   ```sh
   sudo cp deploy/systemd/vigia-agent.service /etc/systemd/system/vigia-agent.service
   sudo systemctl daemon-reload
   sudo systemctl enable --now vigia-agent
   ```
3. Verifica los logs y genera un código de vinculación:
   ```sh
   sudo journalctl -u vigia-agent -f
   sudo -u vigia /usr/local/bin/vigia-agent pair -data /var/lib/vigia
   ```

> **Firewall:** abre el puerto `7341/tcp` (API HTTPS) y `5353/udp` (mDNS) únicamente hacia tu LAN local o interfaz Tailscale. Si tu equipo tiene IPv6 público global, usa `-bind <IP_LAN>:7341` o filtra el puerto `7341` en el firewall hacia el exterior.

---

## 4. Agente 24/7 en Docker (`network_mode: host`)

Vigía necesita visibilidad directa de la interfaz física de tu LAN para leer la tabla ARP/NDP, enviar multicast mDNS/SSDP y descubrir dispositivos UPnP/NAT-PMP. Por eso **es obligatorio usar `network_mode: host`** (el modo `bridge` por defecto aísla al contenedor en una subred virtual `172.17.0.0/16`).

> **Nota sobre macOS y Windows:** Docker Desktop ejecuta los contenedores dentro de una máquina virtual aislada, por lo que `network_mode: host` no expone la LAN física del Mac/PC. En macOS utiliza el LaunchAgent nativo descrito en la sección 2. En Linux, Raspberry Pi o NAS (Unraid, Synology, Debian, etc.), Docker funciona de forma nativa. Si usas **PC con Windows**, `vigia-agent install` solo automatiza servicios en macOS/Linux; en Windows puedes compilar el binario y ejecutarlo directamente con `vigia-agent.exe run -interval 5m` (o desplegarlo en Linux/Raspberry Pi en tu misma red).

### Despliegue con Docker Compose

El repositorio incluye `Dockerfile` y `compose.yml` configurados con `network_mode: host`, capacidades mínimas de red (`cap_drop: ALL`, `cap_add: [NET_RAW]`, `no-new-privileges:true`) y volumen persistente para `/data`.

1. Arranca el servicio en segundo plano:
   ```sh
   docker compose up -d
   ```
2. Revisa los registros para confirmar el primer escaneo y la huella TLS:
   ```sh
   docker compose logs -f vigia-agent
   ```
3. Genera un código de vinculación de 6 dígitos para tu teléfono:
   ```sh
   docker compose exec vigia-agent vigia-agent pair -data /data
   ```
4. Otras operaciones útiles dentro del contenedor:
   ```sh
   # Ver huella TLS SHA-256
   docker compose exec vigia-agent vigia-agent cert -data /data

   # Ver clientes vinculados
   docker compose exec vigia-agent vigia-agent tokens -data /data

   # Ver estado general
   docker compose exec vigia-agent vigia-agent status -data /data
   ```

Para actualizar la imagen tras descargar cambios:

```sh
docker compose up -d --build
```

### Cómo vincular la app móvil por primera vez (iMac / PC / Docker)

1. Con el agente corriendo en la misma red local que tu teléfono, genera un código temporal de vinculación de 6 dígitos en el equipo servidor:
   - **macOS (iMac/Mac):** `~/Library/Application\ Support/Vigia/vigia-agent pair`
   - **Linux (`systemd`):** `vigia-agent pair` (o `sudo -u vigia /usr/local/bin/vigia-agent pair -data /var/lib/vigia`)
   - **Docker:** `docker compose exec vigia-agent vigia-agent pair -data /data`
   - **Ejecución manual / PC:** `vigia-agent pair` (misma ruta `-data` si usaste una personalizada)
2. Abre **Vigía** en el teléfono y, en el **Panel**, toca la tarjeta **Vigilancia 24/7**.
3. La app buscará el agente automáticamente en tu LAN vía mDNS (`_vigia._tcp`). Si no aparece (por ejemplo, si el firewall bloquea multicast UDP `5353`), selecciona **Agregar con IP y puerto** e introduce la IP local del servidor y el puerto `7341`.
4. Escribe el código de **6 dígitos** mostrado en consola y toca **Vincular**. La app guardará el token y fijará la huella TLS SHA-256 del agente (`certSha256`) en Android Keystore / iOS Keychain.
5. *(Opcional, acceso fuera de casa con Tailscale)*: instala Tailscale tanto en el servidor del agente como en tu teléfono; el agente anuncia automáticamente su IP `100.64.0.0/10` en `/api/v1/info`. En los ajustes de **Agente vinculado** dentro de la app, activa el modo remoto para consultar tu red vía Tailscale.

---

## 5. Compilar la app Android

### Requisitos

1. **JDK 17**, **Android SDK** (compileSdk 36, minSdk 26) y **Android NDK**.
2. **`gomobile` y `gobind`:**
   ```sh
   go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest
   export PATH="$(go env GOPATH)/bin:$PATH"
   gomobile init
   ```

### Paso 1: Compilar el motor Go a `.aar`

El archivo `android/app/libs/vigia-core.aar` no se versiona en Git; genéralo con:

```sh
./scripts/build-aar.sh
```

Este script compila `core` y `mobile` para `arm64` y `amd64` usando `-trimpath -ldflags="-s -w"` y deja el binario en `android/app/libs/vigia-core.aar`.

### Paso 2: Ejecutar pruebas y generar el APK

```sh
cd android
./gradlew testDebugUnitTest assembleDebug
```

El APK de depuración se genera en `android/app/build/outputs/apk/debug/app-debug.apk`.

Para instalarlo en un dispositivo conectado por USB/ADB:

```sh
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

### Compilación Release firmada

El tipo `release` habilita minificación R8 (`isMinifyEnabled = true`) y reducción de recursos (`isShrinkResources = true`). Puedes proporcionar tu propio keystore mediante variables de entorno (nunca subas `.keystore` ni `.jks` al repositorio):

```sh
export VIGIA_KEYSTORE_PATH=/ruta/segura/release.keystore
export VIGIA_KEYSTORE_PASSWORD=...
export VIGIA_KEY_ALIAS=...
export VIGIA_KEY_PASSWORD=...
cd android && ./gradlew assembleRelease
```

Cuando termines de compilar con Gradle en local, puedes detener el demonio para liberar memoria:

```sh
./gradlew --stop
```

---

## 6. Compilar la app iOS (iPhone)

### Requisitos

1. **macOS** con **Xcode** (iOS 17+ SDK). En instalaciones nuevas de Xcode, ejecuta una vez:
   ```sh
   sudo xcodebuild -runFirstLaunch
   ```
2. **Go 1.26+** con `gomobile` y `gobind` inicializados (`gomobile init`).
3. **XcodeGen:**
   ```sh
   brew install xcodegen
   ```

### Paso 1: Compilar `VigiaCore.xcframework`

Genera el XCFramework del motor Go (dispositivo `ios/arm64` y simulador) con:

```sh
./scripts/build-xcframework.sh
```

El artefacto queda en `ios/Frameworks/VigiaCore.xcframework` (ignorado por Git).

### Paso 2: Generar el proyecto Xcode y compilar

Si compilaste `VigiaCore.xcframework`, descomenta la dependencia `Frameworks/VigiaCore.xcframework` en `ios/project.yml` (ver `ios/README.md`) antes de generar el proyecto:

```sh
cd ios
xcodegen generate
open Vigia.xcodeproj
```

Dentro de Xcode:
1. Selecciona tu equipo de desarrollador (*Signing & Capabilities*) tanto en el target **Vigia** como en **VigiaWidgetExtension** (actualiza los identificadores de App Group / Bundle ID si usas un perfil de firma personal).
2. Ten en cuenta los **entitlements** de Apple declarados en `ios/Vigia/Vigia.entitlements`:
   - `com.apple.developer.networking.wifi-info` (permite leer el SSID/BSSID de la red Wi-Fi actual tras conceder ubicación).
   - `com.apple.developer.networking.multicast` (requiere aprobación de Apple para sockets multicast en distribución comercial; en desarrollo o sin él, `NWBrowser` cubre el descubrimiento Bonjour del agente).
   - `com.apple.security.application-groups` (`group.com.baxner.vigia`, para compartir el resumen con WidgetKit).
3. Conecta un iPhone físico (recomendado para probar red local real, Bluetooth LE y brújula) y ejecuta **Run** (`Cmd + R`).

> **Nota sobre capacidades de iOS:** por restricciones del sistema operativo de Apple, ninguna app estándar en iPhone puede acceder a la tabla ARP ni listar redes Wi-Fi vecinas; el motor Go descubre equipos en LAN vía ping ICMP/TCP, mDNS, SSDP y UPnP, y el radar BLE + brújula funciona en tiempo real con `CoreBluetooth` y `CoreMotion`. Para monitoreo continuo 24/7 y alertas con la app cerrada en iPhone, vincula la app a un Agente 24/7 y configura un webhook push (`ntfy`, Telegram o Home Assistant) desde la pestaña de alertas/webhooks.
