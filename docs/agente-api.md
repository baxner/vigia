# Agente Vigía — contrato de la API (v1.5)

El agente (`cmd/vigia-agent`) vigila tu red local 24/7 desde un equipo siempre encendido (macOS, Linux / Raspberry Pi o contenedor Docker). Las aplicaciones móviles (Android e iOS) se conectan a él cuando están en esa red o vía Tailscale; fuera de casa siguen escaneando de forma autónoma la red a la que estén conectadas. Este documento define el contrato compartido entre el agente, el motor Go (`core/`) y las apps clientes.

> Para desplegar el agente paso a paso en macOS, Linux/Raspberry Pi o Docker, consulta [docs/instalacion.md](instalacion.md).

## Principios

- **100 % local por diseño (LAN y Tailscale):** el inventario de equipos, direcciones MAC, historial, eventos, alertas e informe de seguridad nunca salen de tu red ni pasan por servidores propios o de terceros.
- **Sondas opcionales de Internet (Fase 7):** la única comunicación hacia el exterior corresponde al medidor opcional de calidad de Internet, que realiza sondas técnicas de conectividad (`1.1.1.1`, `8.8.8.8` en TCP/443, `[2606:4700:4700::1111]:443` para IPv6) y resolución DNS / prueba de ancho de banda contra `speed.cloudflare.com`, **sin enviar ningún dato de tu red local**, MACs ni nombres de dispositivos. Las pruebas automáticas de velocidad pueden reducirse o desactivarse (`speedTestsPerDay: 0`).
- **Autenticación por cabecera:** `Authorization: Bearer <token>`, nunca en la URL.
- **Contrato JSON unificado:** los tipos de dominio (`KnownDevice`, `DeviceEvent`, `DeviceCustomization`, `Alert`, `SecurityReport`, `InternetStatus`) son compartidos con el motor Go y los clientes Android/iOS; el campo `last` contiene el `core.Device` del motor. Todas las fechas van en RFC 3339 UTC.
- **Ligero:** escaneo periódico cada 5 min (configurable) con prioridad de fondo.

## Descubrimiento (mDNS / Bonjour)

- Servicio `_vigia._tcp`, puerto por defecto **7341** (HTTPS; ver sección [TLS (v1.5)](#tls-v15)).
- Registros TXT:
  - `v=1`
  - `id=<uuid estable del agente>`
  - `name=<nombre del agente>` (p. ej. `mi-servidor`)
  - `tls=1`
  - `cert=<primeros 16 caracteres hex de la huella SHA-256 del certificado>`

## Vinculación (una vez por dispositivo móvil)

1. En el equipo donde corre el agente, genera un código temporal de 6 dígitos (válido 5 minutos):
   ```sh
   vigia-agent pair
   ```
2. La app (descubriendo el agente por mDNS o ingresando `IP:puerto` manualmente) envía el código:

```text
POST /api/v1/pair            (sin token)
{"code": "482913", "client": "Mi Teléfono"}
→ 200 {"token": "<64 hex>", "agentId": "…", "agentName": "mi-servidor",
       "certSha256": "<64 hex>"}
→ 401 {"error": "código inválido o vencido"}
```

- Máximo 5 intentos por código y límite de tasa por IP para mitigar fuerza bruta; agotados los intentos, debe generarse un código nuevo.
- El agente almacena únicamente el **hash SHA-256** de cada token emitido en SQLite (`agent.db`); el token en texto plano nunca vuelve a mostrarse.
- Administración desde CLI: `vigia-agent tokens` lista los clientes vinculados y `vigia-agent revoke <cliente>` revoca sus tokens.

## Endpoints principales

Todos requieren `Authorization: Bearer <token>` salvo `GET /api/v1/info` y `POST /api/v1/pair`. `{key}` de red va URL-encoded. Los errores responden `{"error": "…"}` con códigos `400`, `401`, `404`, `409`, `429`, `500` o `503`.

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/info` | `{"id","name","version","scanIntervalSec","lastScanAt","warning"?,"addresses","certSha256"}` (sin token: permite mostrar el agente antes de vincular y verificar su huella). `warning` aparece si el último escaneo no obtuvo respuestas activas (p. ej., falta de permiso de Red local en macOS o falta de `--network=host` en Docker); los escaneos ciegos no sobrescriben el historial |
| `GET /api/v1/networks` | Redes vigiladas: `[{"key","subnet","gateway","gatewayMac","firstSeen","lastSeen","lastScanAt","role"}]`. `role` (v1.2): `"inner"` para la LAN principal, `"outer"` para el anillo exterior (ver [Seguridad (v1.2)](#seguridad-v12)) |
| `GET /api/v1/networks/{key}/devices` | `[KnownDevice]` de esa red, tanto en línea como sin conexión |
| `GET /api/v1/devices/{id}` | `KnownDevice` |
| `GET /api/v1/devices/{id}/events?limit=200` | `[DeviceEvent]` del más reciente al más antiguo (`limit` máximo `1000`) |
| `PATCH /api/v1/devices/{id}` | Cuerpo `DeviceCustomization` (campos ausentes = sin cambio; `null` = borrar) → `KnownDevice` |
| `DELETE /api/v1/devices/{id}` | Elimina el equipo y su historial de eventos → `204` |
| `POST /api/v1/scan` | Dispara un escaneo inmediato → `202 {"started": true}` (o `409` si ya hay uno en curso) |
| `GET /api/v1/stream` | Flujo Server-Sent Events en vivo (ver [Stream (SSE)](#stream-sse)) |

### Formas JSON base

```json
KnownDevice = {
  "id": 12,
  "network": "192.168.1.1|192.168.1.0/24",
  "mac": "02:00:00:00:00:10",
  "lastIp": "192.168.1.10",
  "firstSeen": "2026-10-06T22:10:00Z",
  "lastSeen": "2026-10-07T01:05:00Z",
  "online": true,
  "onlineSince": "2026-10-06T22:10:00Z",
  "last": { "ip": "192.168.1.10", "mac": "02:00:00:00:00:10", "...": "..." },
  "custom": {
    "name": null,
    "typeId": null,
    "notes": null,
    "location": "Sala",
    "important": false
  }
}

DeviceEvent = {
  "deviceId": 12,
  "type": "FIRST_SEEN|CAME_ONLINE|WENT_OFFLINE|IP_CHANGED",
  "at": "2026-10-06T22:10:00Z",
  "detail": "192.168.1.10 → 192.168.1.25"
}
```

### Stream (SSE)

`GET /api/v1/stream` (con token). Eventos emitidos:

- `event: scan` — `{"network": key, "startedAt", "finishedAt", "devices": N}` al finalizar cada escaneo.
- `event: device` — `{"event": DeviceEvent, "device": KnownDevice}` por cada evento nuevo.
- `event: alert` — `Alert` completo por cada alerta nueva disparada.
- `: ping` — comentario keep-alive cada 25 s para mantener abierta la conexión.

## Alertas (v1.1)

El agente evalúa las reglas de alerta y las persiste en SQLite; las apps móviles las notifican y sistemas externos (Home Assistant, ntfy, Pushover, scripts propios) pueden recibirlas vía webhooks.

### Reglas en el agente

| Tipo | Cuándo se dispara | Severidad |
|---|---|---|
| `NEW_DEVICE` | `FIRST_SEEN` de un equipo **después de la línea base** (>15 min después del primer escaneo de la red; mismo criterio que la insignia "Nuevo") | `high` |
| `IMPORTANT_OFFLINE` | `WENT_OFFLINE` de un equipo marcado con `custom.important = true` | `medium` |
| `IMPORTANT_ONLINE` | `CAME_ONLINE` de un equipo marcado con `custom.important = true` | `low` |
| `AGENT_BLIND` | El agente deja de recibir respuestas activas en la LAN (ver `warning` en `/info`); una sola alerta hasta recuperarse | `medium` |
| `AGENT_RECOVERED` | El agente vuelve a ver la red después de un `AGENT_BLIND` | `low` |

Cada tipo puede habilitarse o deshabilitarse individualmente mediante `/api/v1/alert-settings`. La clave de deduplicación garantiza que un mismo evento no genere alertas repetidas.

### Formas JSON de Alertas y Webhooks

```json
Alert = {
  "id": 57,
  "type": "NEW_DEVICE",
  "severity": "high",
  "at": "2026-10-07T07:10:00Z",
  "title": "Equipo nuevo en tu red",
  "body": "Mi Teléfono (192.168.1.25) se conectó por primera vez.",
  "deviceId": 31,
  "network": "192.168.1.1|192.168.1.0/24",
  "read": false
}

AlertSettings = {
  "NEW_DEVICE": true,
  "IMPORTANT_OFFLINE": true,
  "IMPORTANT_ONLINE": true,
  "AGENT_BLIND": true,
  "AGENT_RECOVERED": true,
  "ROUTER_CHANGED": true,
  "ARP_CONFLICT": true,
  "PORT_MAPPING_ADDED": true,
  "RISKY_SERVICE": true,
  "INTERNET_DOWN": true,
  "INTERNET_RESTORED": true
}

Webhook = {
  "id": 3,
  "url": "http://homeassistant.local:8123/api/webhook/vigia",
  "types": ["NEW_DEVICE"],
  "createdAt": "2026-10-07T07:15:00Z",
  "lastStatus": 200,
  "lastError": null
}
```

### Endpoints de Alertas y Webhooks (con token)

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/alerts?after=<id>&limit=100` | Alertas con `id > after`, ordenadas de la más antigua a la más reciente (`limit` máximo `1000`) |
| `POST /api/v1/alerts/read` | `{"upTo": <id>}` marca como leídas todas las alertas hasta ese id → `204` |
| `GET /api/v1/alert-settings` / `PUT /api/v1/alert-settings` | Lee o reemplaza `AlertSettings` |
| `GET /api/v1/webhooks` | Lista `[Webhook]` registrados |
| `POST /api/v1/webhooks` | `{"url", "types"?: [...], "secret"?: "…"}` → `Webhook` (nunca devuelve `secret`) |
| `DELETE /api/v1/webhooks/{id}` | Elimina el webhook → `204` |
| `POST /api/v1/webhooks/{id}/test` | Envía una alerta sintética de prueba → `{"status": 200}` o `{"error": "…"}` |

Para conectividad remota (p. ej. vía Tailscale), `GET /api/v1/info` incluye `"addresses": ["192.168.1.10", "100.64.0.10"]` con las direcciones IPv4 activas del host.

### Webhooks

- Petición `POST` a la URL configurada con `Content-Type: application/json` y cuerpo:
  `{"event": "alert", "alert": Alert, "device": KnownDevice|null, "agent": {"id","name"}}`.
- Cabeceras:
  - `User-Agent: Vigia-Agent/<versión>`
  - `X-Vigia-Event: <tipo>`
  - `X-Vigia-Delivery: <uuid>`
  - `X-Vigia-Signature: sha256=<hex HMAC-SHA256(secreto, cuerpo)>` (si tiene secreto).
- Timeout de 5 s por intento, hasta 3 reintentos con espera exponencial (`1 s`, `5 s`, `25 s`); el último resultado se registra en `lastStatus`/`lastError`.
- **Protección anti-SSRF:** solo admite URLs `http`/`https` válidas sin credenciales en la URL (`user:pass@`), bloquea direcciones loopback (`127.0.0.0/8`, `::1`), no especificadas (`0.0.0.0`, `::`) y link-local / metadata cloud (`169.254.0.0/16`, `fe80::/10`), y **nunca sigue redirecciones HTTP**. Permite destinos en LAN privada y Tailscale (`100.64.0.0/10`) para integraciones locales como Home Assistant o servicios push externos.
- CLI:
  - `vigia-agent webhook add [-types T1,T2] [-secret S] <url>`
  - `vigia-agent webhook list`
  - `vigia-agent webhook rm <id>`
  - `vigia-agent webhook test <id>`

### Notificaciones en las apps móviles

- **Android:** con la app o el servicio en vivo (`LiveWatchService`) conectado al stream SSE, las alertas llegan al instante; en segundo plano, `AlertsSyncWorker` (WorkManager cada ~15 min) consulta `GET /api/v1/alerts?after=<última vista>` por LAN o Tailscale.
- **iPhone (iOS):** mientras la app está abierta recibe eventos SSE en tiempo real; en segundo plano, iOS limita la ejecución continua de sockets, por lo que `BGAppRefreshTask` sincroniza periódicamente `/api/v1/alerts` de forma oportunista y, para avisos instantáneos con la app cerrada, se pueden configurar webhooks hacia servicios push (como `ntfy`, Pushover, Telegram o Home Assistant) directamente desde la app.

## Seguridad (v1.2)

El agente genera un informe de seguridad sobre la red vigilada completamente en local.

### Anillos (Interior y Exterior)

- **Interior (`"role": "inner"`):** la subred principal del router local (p. ej. `192.168.1.0/24`).
- **Exterior (`"role": "outer"`):** la subred situada delante del router (p. ej. entre el router y el módem/ONT del proveedor). El agente la deduce consultando la IP WAN del router vía UPnP (`GetExternalIPAddress`) o NAT-PMP: si es una dirección privada o CGNAT (`10/8`, `172.16/12`, `192.168/16`, `100.64/10`), asume un `/24` con gateway en `.1`. También puede fijarse manualmente con `vigia-agent run -outer <cidr> -outer-gateway <ip>` o desactivarse con `-outer off`. Se escanea cada 10 min sin tabla ARP ni multicast (barrido ICMP/TCP, NetBIOS, puertos y huella HTTP).
- **Alcance de redirecciones (`reach`):** si la IP externa del router es privada o CGNAT, los puertos reenviados solo son alcanzables desde el anillo exterior (`"outer_ring"`); si es IP pública, quedan expuestos a `"internet"`.

### Redirecciones de puertos

- **UPnP (`source: "upnp"`):** mapeos dinámicos consultados al router (`GetGenericPortMappingEntry`) cada 5 min.
- **Manuales (`source: "probe"`):** redirecciones configuradas manualmente en el router; se detectan cuando la app móvil escanea la IP exterior del router desde el anillo exterior.

### Alertas de seguridad

| Tipo | Cuándo se dispara | Severidad |
|---|---|---|
| `ROUTER_CHANGED` | Cambia la MAC de la puerta de enlace del anillo interior respecto a la registrada | `high` |
| `ARP_CONFLICT` | La IP del router responde con la MAC de otro equipo conocido (posible suplantación ARP) | `high` |
| `PORT_MAPPING_ADDED` | Aparece una redirección UPnP nueva | `medium` (`high` si el puerto interior es de alto riesgo) |
| `RISKY_SERVICE` | Un equipo del anillo interior expone por primera vez un puerto de riesgo medio o alto | `medium` |

`NEW_DEVICE` también evalúa el anillo exterior con su propia línea base independiente. `AGENT_BLIND` y `AGENT_RECOVERED` solo se evalúan sobre el anillo interior.

### Informe (`GET /api/v1/security`)

Calculado al vuelo con el último estado conocido de cada escaneo:

```json
SecurityReport = {
  "generatedAt": "2026-10-08T15:00:00Z",
  "score": 82,
  "findings": [Finding],
  "router": RouterInfo,
  "outerRing": OuterRing
}

Finding = {
  "id": "RISKY_SERVICE|dev:12|445",
  "kind": "RISKY_SERVICE",
  "severity": "high|medium|low|info",
  "points": 8,
  "title": "Una computadora comparte archivos con toda la red",
  "detail": "PC-Escritorio (192.168.1.20) tiene abierto SMB (445).",
  "action": "Si no compartes carpetas desde esa computadora, desactiva el uso compartido.",
  "deviceId": 12,
  "ip": "192.168.1.20",
  "port": 445,
  "acknowledged": false,
  "acknowledgedAt": null
}

RouterInfo = {
  "ip": "192.168.1.1",
  "mac": "02:00:00:00:00:01",
  "upnp": true,
  "natpmp": true,
  "externalIp": "10.0.0.2",
  "reach": "outer_ring",
  "portMappings": [PortMapping]
}

PortMapping = {
  "protocol": "TCP",
  "externalPort": 8080,
  "internalIp": "192.168.1.20",
  "internalPort": 80,
  "description": "Servicio web local",
  "source": "upnp",
  "deviceId": 12,
  "firstSeen": "2026-10-08T14:00:00Z",
  "lastSeen": "2026-10-08T15:00:00Z"
}

OuterRing = {
  "network": "10.0.0.1|10.0.0.0/24",
  "subnet": "10.0.0.0/24",
  "gateway": "10.0.0.1",
  "lastScanAt": "2026-10-08T14:55:00Z",
  "devices": [SecurityDevice]
}

SecurityDevice = {
  "id": 31,
  "name": "Módem-ISP",
  "online": true,
  "device": { "...": "core.Device" }
}
```

Tipos de hallazgo (`kind`): `NO_INBOUND`, `UPNP_ENABLED`, `PORT_MAPPING`, `RISKY_SERVICE`, `UNKNOWN_DEVICES`, `OUTER_RING_DEVICES`, `ROUTER_CHANGED`, `ARP_CONFLICT`, y los específicos de Wi-Fi reportados por la app móvil (`WIFI_OPEN`, `WIFI_WEP`, `WIFI_WPA2`, `WIFI_WPS`).

### Motor compartido (`mobile/`)

El cálculo de seguridad y el catálogo de puertos (`core/data/ports.tsv`) viven en `core/` y se exponen a Android (`.aar`) e iOS (`.xcframework`) vía `gomobile`:

- `Mobile.portCatalog(): String` → `[{"port","name","risk","why","action"}]`
- `Mobile.routerInfo(gateway: String): String` → `RouterInfo` o `"null"`
- `Mobile.securityReport(input: String): String` → `SecurityReport`, recibiendo `{"devices":[SecurityDevice], "router":RouterInfo|null, "outerRing":OuterRing|null, "wifi":WifiInfo|null, "recentAlerts":[Alert], "acknowledged":[Acknowledgement]}`

## Hallazgos reconocidos (v1.3)

Permite marcar un hallazgo como intencional ("Ya lo sé, es a propósito", p. ej., un servicio SSH o flujo RTSP local esperado). El hallazgo permanece visible al final de la lista en `findings` pero deja de restar puntos del `score`.

- `PUT /api/v1/security/acknowledged/{findingId}` (URL-encoded, cuerpo opcional `{"note": "…"}`) → `204`
- `DELETE /api/v1/security/acknowledged/{findingId}` → `204`

## Internet (v1.4)

El agente mide continuamente la latencia por tramos y detecta caídas identificando dónde se interrumpió la conectividad.

### Medición (cada 30 s)

- **Tres tramos locales/externos:**
  - **router:** gateway del anillo interior (TCP/80 con respaldo ICMP sin privilegios).
  - **módem:** gateway del anillo exterior, si está disponible.
  - **internet:** sondas opcionales TCP/443 a `1.1.1.1` y `8.8.8.8`.
- **DNS:** tiempo de resolución del nombre `speed.cloudflare.com` usando el resolvedor del sistema.
- **IPv6 (cada 10 min):** si el host tiene una dirección IPv6 global, prueba conectividad contra `[2606:4700:4700::1111]:443`. Si hay IPv6 asignado pero no enruta salida, reporta `ipv6: "broken"`.
- **Retención en SQLite:** agregados por minuto durante 7 días y por hora durante 90 días.
- **Privacidad:** estas sondas no transmiten direcciones MAC, nombres de equipos, inventarios ni topología de tu red local.

### Caídas y culpables

- **Inicio / fin:** una caída inicia tras 2 mediciones consecutivas fallidas en el tramo `internet` y finaliza tras 2 mediciones exitosas consecutivas.
- **Atribución (`blame`):**
  - `lan`: el router local no responde.
  - `modem`: el router responde, el módem/ONT no.
  - `isp`: router y módem responden, Internet no.
  - `unknown`: sin suficiente evidencia por tramos.
- **Alertas asociadas:** `INTERNET_DOWN` (`high`) e `INTERNET_RESTORED` (`low`).

### Prueba de velocidad

- Utiliza los endpoints de prueba de Cloudflare (`speed.cloudflare.com/__down` y `speed.cloudflare.com/__up`), descargando ~25 MB y subiendo ~10 MB para medir ancho de banda y latencia bajo carga.
- Por defecto ejecuta 2 pruebas automáticas al día (configurable de `0` a `6` en `PUT /api/v1/internet/settings`; `0` deshabilita las pruebas automáticas), además de ejecuciones manuales bajo demanda (máximo una a la vez).

### Endpoints de Internet (con token)

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/internet` | `InternetStatus` |
| `GET /api/v1/internet/series?range=24h\|7d` | `[{"at","routerMs","modemMs","internetMs","dnsMs","lossPct"}]` por minuto (`24h`) o por hora (`7d`), con `null` en intervalos sin muestra |
| `POST /api/v1/internet/speedtest` | Dispara prueba manual → `202 {"started": true}`, o `409` si ya hay una en curso |
| `GET /api/v1/internet/settings` / `PUT` | `{"speedTestsPerDay": 2}` (`0` a `6`) |

```json
InternetStatus = {
  "state": "up|down|degraded",
  "since": "2026-10-08T12:00:00Z",
  "now": {"routerMs": 1.4, "modemMs": 2.9, "internetMs": 12.6, "dnsMs": 9.8, "lossPct": 0},
  "ipv6": "ok|broken|none",
  "outages": [{"start": "…", "end": "…", "durationSec": 45.2, "blame": "isp"}],
  "speedTests": [{"at": "…", "downMbps": 220.5, "upMbps": 85.2, "latencyMs": 11.4, "loadedLatencyMs": 24.1, "error": null}]
}
```

`"degraded"` se activa cuando la pérdida supera el 5 % o la latencia excede `3×` la mediana de las últimas 24 h.

## Suspensión del sistema (Fase 8)

Cuando el equipo donde corre el agente entra en suspensión, el proceso se congela. Al despertar, `agent/wake` compara la marca de reloj de pared (`Round(0)`, sin depender del reloj monotónico) contra la vuelta anterior para evitar falsos positivos:

- **Escaneo inmediato:** el planificador dispara un escaneo al detectar un salto superior a `2×` el intervalo configurado.
- **Sin caídas fantasma:** el medidor de Internet reinicia `OutageDetector` ante saltos >90 s (`3×` su ciclo de 30 s).
- **Sin falsas alertas `IMPORTANT_OFFLINE`:** si el intervalo entre escaneos supera `2×` el intervalo configurado, se omite la evaluación de desconexión en ese primer ciclo de recuperación y se reevalúa en el siguiente.

## Red del agente vs. red de la app y reglas de historial

- Un agente conectado por cable no conoce el SSID Wi-Fi, por lo que identifica cada red por `<gateway>|<subred>` (y registra aparte la MAC del router). La app reconoce que el agente vigila su red actual si coincide la subred y la puerta de enlace.
- **Reconciliación (`agent/history/`):** misma MAC → mismo UDN UPnP (`ssdp:udn`) → mismo nombre fuerte (`mdns:name` o hostname estable) → misma IP (bloqueado en cada paso si un identificador más fuerte contradice).
- **Periodo de gracia:** un equipo ausente en un solo escaneo pasa a `online=false` sin emitir evento; `WENT_OFFLINE` se registra tras 2 ausencias consecutivas o >10 min desde `lastSeen`.

## TLS (v1.5)

### Certificado propio

- Al primer arranque, el agente genera un certificado autofirmado **ECDSA P-256** válido **10 años** y su llave privada (`agent-cert.pem` y `agent-key.pem` con permisos `0600` dentro del directorio de datos). Se reutilizan entre reinicios.
- `vigia-agent cert` muestra la huella SHA-256 hex (64 caracteres) para verificación manual.
- `vigia-agent cert -rotate` genera un nuevo par de llaves; **todos los clientes vinculados deberán volver a vincularse** tras reiniciar el agente, ya que el pinning rechazará el nuevo certificado por seguridad.

### Servidor TLS y flags de red

- Escucha por defecto en `:7341` (`-port 7341`, configurable con `-bind <ip:puerto>` o `-bind 192.168.1.10:7341` para limitar interfaces en hosts con IPv6 público o múltiples tarjetas de red). TLS requiere mínimo TLS 1.2.
- **Modo híbrido de transición (`-http-legacy`):** por defecto acepta conexiones TLS y HTTP heredado en el mismo puerto inspeccionando el primer byte (`0x16` para `ClientHello` TLS). Para exigir exclusivamente HTTPS en entornos nuevos, arranca con `-http-legacy=false`.
- **Pinning en las apps:** durante la vinculación, el cliente móvil verifica que el certificado presentado en el handshake TLS coincida con `certSha256` (y con el TXT `cert=` en descubrimiento mDNS) y fija esa huella SHA-256 en almacenamiento seguro (Android Keystore / iOS Keychain) para todas las peticiones futuras. En redes locales diferentes que compartan la misma numeración IP (p. ej. `192.168.1.10` en otra red), un certificado distinto se reporta como otro equipo; por Tailscale (`100.64.0.0/10`), cualquier cambio de huella bloquea la conexión inmediatamente.

## Rotación de logs

`agent/rotatelog` rota `agent.log` automáticamente al alcanzar 5 MB, conservando hasta 3 respaldos (`.1`, `.2`, `.3`) con permisos `0600` y protección concurrente mediante mutex. En macOS bajo `launchd`, `agent.log.launchd` captura únicamente salida cruda previa a la inicialización o panics del runtime.
