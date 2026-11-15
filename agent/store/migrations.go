package store

// migrations: el esquema completo, versionado con PRAGMA user_version (ver
// Store.migrate en store.go). Cambios futuros se agregan al final de este
// slice — nunca editando una migración ya publicada, para no romper bases
// que ya la aplicaron.
var migrations = []string{
	migration1,
	migration2,
	migration3,
	migration4,
	migration5,
}

// migration1 crea el esquema inicial: redes, equipos (con las columnas
// indexables ssdp_udn/strong_name que la reconciliación necesita poder
// consultar sin deserializar last_json — ver DeviceEntity en
// HistoryEntities.kt, de donde viene este diseño), su cronología de
// eventos, los clientes vinculados (solo el hash del token) y los códigos
// de vinculación pendientes.
const migration1 = `
CREATE TABLE networks (
	key          TEXT PRIMARY KEY,
	subnet       TEXT NOT NULL,
	gateway      TEXT NOT NULL,
	gateway_mac  TEXT,
	first_seen   TEXT NOT NULL,
	last_seen    TEXT NOT NULL,
	last_scan_at TEXT
);

CREATE TABLE devices (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	network_key      TEXT NOT NULL REFERENCES networks(key) ON DELETE CASCADE,
	-- MAC "pegajosa": ver history.Seen. Puede faltar si nunca se resolvió.
	mac              TEXT,
	last_ip          TEXT NOT NULL,
	-- ssdp_udn y strong_name son una copia indexable de los identificadores
	-- fuertes del ÚLTIMO escaneo (recalculados desde last_json en cada
	-- escritura, igual que history.Reconcile con KnownDevice.Last). A
	-- diferencia de mac, NO son pegajosos: si un escaneo no los vuelve a
	-- detectar, quedan en NULL hasta el próximo que sí.
	ssdp_udn         TEXT,
	strong_name      TEXT,
	first_seen       TEXT NOT NULL,
	last_seen        TEXT NOT NULL,
	online           INTEGER NOT NULL,
	online_since     TEXT,
	-- offline_misses: contabilidad interna de history.Missing para la
	-- gracia de WENT_OFFLINE. No forma parte del contrato HTTP.
	offline_misses   INTEGER NOT NULL DEFAULT 0,
	-- El último core.Device tal cual, serializado en JSON.
	last_json        TEXT NOT NULL,
	custom_name      TEXT,
	custom_type_id   TEXT,
	custom_notes     TEXT,
	custom_location  TEXT,
	custom_important INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX devices_network_idx ON devices(network_key);
CREATE INDEX devices_network_mac_idx ON devices(network_key, mac);
CREATE INDEX devices_network_udn_idx ON devices(network_key, ssdp_udn);
CREATE INDEX devices_network_name_idx ON devices(network_key, strong_name);

CREATE TABLE device_events (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	type      TEXT NOT NULL,
	at        TEXT NOT NULL,
	detail    TEXT
);
CREATE INDEX device_events_device_idx ON device_events(device_id);

-- Un cliente vinculado (un teléfono, normalmente). Solo el hash del token:
-- el agente nunca guarda el token en claro, solo lo entrega una vez al
-- vincular (POST /api/v1/pair).
CREATE TABLE clients (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	created_at   TEXT NOT NULL,
	last_used_at TEXT,
	-- NULL = vigente. No se borra al revocar: queda el rastro en "tokens".
	revoked_at   TEXT
);

-- A lo sumo una fila: generar un código nuevo (vigia-agent pair) borra
-- cualquier pendiente. Ver Store.CreatePairingCode / TryConsumePairingCode.
CREATE TABLE pairing_codes (
	code       TEXT PRIMARY KEY,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	attempts   INTEGER NOT NULL DEFAULT 0
);
`

// migration2 agrega "Alertas (v1.1)" (ver docs/agente-api.md, sección
// Alertas): las reglas puras viven en agent/alerts, esta migración solo
// guarda su resultado y lo que necesitan para evaluarse.
const migration2 = `
-- Una alerta ya evaluada y persistida (ver agent/alerts.Alert y
-- store.Alert, que la sirve tal cual por la API). dedup_key es la
-- defensa real de "una alerta por evento": agent/alerts calcula la misma
-- clave si el evento de origen es el mismo, y esta restricción UNIQUE
-- hace que INSERT ... ON CONFLICT DO NOTHING (ver InsertAlert) nunca
-- duplique la fila, incluso si recordScan se reevaluara dos veces.
CREATE TABLE alerts (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	type        TEXT NOT NULL,
	severity    TEXT NOT NULL,
	at          TEXT NOT NULL,
	title       TEXT NOT NULL,
	body        TEXT NOT NULL,
	-- nil en AGENT_BLIND/AGENT_RECOVERED. ON DELETE SET NULL (no CASCADE):
	-- olvidar un equipo (DELETE /api/v1/devices/{id}) no debe borrar su
	-- historial de alertas, solo desvincularlo.
	device_id   INTEGER REFERENCES devices(id) ON DELETE SET NULL,
	network_key TEXT,
	read        INTEGER NOT NULL DEFAULT 0,
	dedup_key   TEXT NOT NULL UNIQUE
);
CREATE INDEX alerts_device_idx ON alerts(device_id);

-- Encendido/apagado por tipo (GET/PUT /api/v1/alert-settings). Fila única
-- (id fijo en 1): es la configuración del agente, no "por cliente".
CREATE TABLE alert_settings (
	id                INTEGER PRIMARY KEY CHECK (id = 1),
	new_device        INTEGER NOT NULL DEFAULT 1,
	important_offline INTEGER NOT NULL DEFAULT 1,
	important_online  INTEGER NOT NULL DEFAULT 1,
	agent_blind       INTEGER NOT NULL DEFAULT 1,
	agent_recovered   INTEGER NOT NULL DEFAULT 1
);
INSERT INTO alert_settings (id) VALUES (1);

-- Webhooks registrados (CLI "vigia-agent webhook" o la API). secret se
-- guarda para firmar X-Vigia-Signature pero la API nunca lo devuelve (ver
-- store.Webhook: json:"-"). types es un CSV de tipos de alerta; NULL o ""
-- = todos (ver Webhook.Accepts).
CREATE TABLE webhooks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	url         TEXT NOT NULL,
	types       TEXT,
	secret      TEXT,
	created_at  TEXT NOT NULL,
	last_status INTEGER,
	last_error  TEXT
);

-- Transición ciego/normal del agente (AGENT_BLIND/AGENT_RECOVERED): fila
-- única (id fijo en 1) para que la alerta se dispare una sola vez hasta
-- la recuperación, incluso entre reinicios del proceso — a diferencia de
-- scanHealth en cmd/vigia-agent/run.go, que es solo en memoria (sirve
-- para /api/v1/info, no para decidir si ya se avisó).
CREATE TABLE agent_health (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	blind       INTEGER NOT NULL DEFAULT 0,
	network_key TEXT,
	since       TEXT
);
INSERT INTO agent_health (id) VALUES (1);
`

// migration3 agrega "Seguridad (v1.2)" (ver docs/agente-api.md, sección
// Seguridad, y docs/plan.md, Fase 5):
//   - networks.role distingue el anillo interior ("inner", el de siempre)
//     del exterior ("outer", la red de delante del router) — ver GET
//     /api/v1/networks.
//   - router_state guarda el último RouterInfo que contestó UPnP o NAT-PMP
//     (una fila por red: normalmente solo la interior), para que
//     GET /api/v1/security tenga algo que servir aunque la consulta más
//     reciente al router no haya contestado.
//   - port_mappings es el historial de redirecciones del router (ver
//     RouterInfo.PortMappings): una fila por redirección distinta
//     (network_key + protocol + external_port + internal_ip, UNIQUE),
//     con first_seen/last_seen y active para saber si sigue ahí o ya no.
//   - risky_service_seen es la memoria PERMANENTE de qué (equipo, puerto)
//     ya disparó RISKY_SERVICE alguna vez — a propósito nunca se borra,
//     así no repite aunque el puerto se cierre y se reabra (ver
//     agent/alerts.EvaluateNewRiskyServices).
//   - security_baseline evita la tormenta de alertas del primer arranque
//     tras actualizar (ver agent/alerts.EvaluateNewPortMappings/
//     EvaluateNewRiskyServices: "hasBaseline"): una fila (id=1) con un
//     interruptor por regla, que se prende la primera vez que esa regla
//     corre con las tablas de arriba vacías y ya no se vuelve a tocar.
//   - alert_settings suma las 4 claves nuevas (ROUTER_CHANGED/
//     ARP_CONFLICT/PORT_MAPPING_ADDED/RISKY_SERVICE), encendidas por
//     defecto como las demás.
const migration3 = `
ALTER TABLE networks ADD COLUMN role TEXT NOT NULL DEFAULT 'inner';

CREATE TABLE router_state (
	network_key TEXT PRIMARY KEY,
	ip          TEXT NOT NULL,
	mac         TEXT,
	upnp        INTEGER NOT NULL DEFAULT 0,
	natpmp      INTEGER NOT NULL DEFAULT 0,
	external_ip TEXT,
	reach       TEXT,
	updated_at  TEXT NOT NULL
);

CREATE TABLE port_mappings (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	network_key   TEXT NOT NULL,
	protocol      TEXT NOT NULL,
	external_port INTEGER NOT NULL,
	internal_ip   TEXT NOT NULL,
	internal_port INTEGER NOT NULL,
	description   TEXT,
	source        TEXT NOT NULL DEFAULT 'upnp',
	-- El equipo conocido de internal_ip al momento de verla (ver "Llena
	-- PortMapping.DeviceID" en docs/plan.md). ON DELETE SET
	-- NULL: olvidar ese equipo no debe borrar el historial de la
	-- redirección, solo desvincularla (mismo criterio que alerts.device_id).
	device_id     INTEGER REFERENCES devices(id) ON DELETE SET NULL,
	first_seen    TEXT NOT NULL,
	last_seen     TEXT NOT NULL,
	active        INTEGER NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX port_mappings_key_idx ON port_mappings(network_key, protocol, external_port, internal_ip);
CREATE INDEX port_mappings_network_active_idx ON port_mappings(network_key, active);

CREATE TABLE risky_service_seen (
	device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
	port       INTEGER NOT NULL,
	first_seen TEXT NOT NULL,
	PRIMARY KEY (device_id, port)
);

CREATE TABLE security_baseline (
	id                  INTEGER PRIMARY KEY CHECK (id = 1),
	port_mappings_done  INTEGER NOT NULL DEFAULT 0,
	risky_services_done INTEGER NOT NULL DEFAULT 0
);
INSERT INTO security_baseline (id) VALUES (1);

ALTER TABLE alert_settings ADD COLUMN router_changed      INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_settings ADD COLUMN arp_conflict        INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_settings ADD COLUMN port_mapping_added  INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_settings ADD COLUMN risky_service       INTEGER NOT NULL DEFAULT 1;
`

// migration4 agrega "Fase 5.1" (ver docs/plan.md y docs/agente-api.md,
// "Hallazgos reconocidos v1.3"):
//   - important_offline_pending: qué equipos importantes tienen un
//     IMPORTANT_OFFLINE sin su IMPORTANT_ONLINE todavía (ver
//     agent/alerts.EvaluateImportantOffline/EvaluateImportantOnline). A
//     diferencia de risky_service_seen, NO es permanente: se borra en
//     cuanto el equipo vuelve, para que el PRÓXIMO episodio pueda avisar
//     de nuevo.
//   - acknowledged_findings: "ya lo sé, es a propósito" sobre un hallazgo
//     de Seguridad, por su id ESTABLE (finding_id) — compartido entre
//     teléfonos, como pide el contrato ("en el agente, compartido entre
//     teléfonos"). ON DELETE no aplica aquí: finding_id no es una FK (un
//     hallazgo puede dejar de existir en el informe sin que el
//     reconocimiento sea un error, ver core.BuildSecurityReport).
const migration4 = `
CREATE TABLE important_offline_pending (
	device_id INTEGER PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE
);

CREATE TABLE acknowledged_findings (
	finding_id TEXT PRIMARY KEY,
	at         TEXT NOT NULL,
	note       TEXT
);
`

// migration5 agrega "Internet (v1.4)" (Fase 7, ver docs/agente-api.md y
// docs/plan.md): calidad del internet de la casa.
//   - internet_minutes/internet_hours: la agregación por minuto (7 días) y
//     por hora (90 días) de agent/internet.Agg (ver RecordMinute/RecordHour
//     en internet.go, que las podan en cada escritura — sin tabla ni job
//     de limpieza aparte). Mismas cinco columnas (sent/lost/min/avg/max)
//     por cada uno de los cuatro tramos (router/módem/internet/DNS): un
//     SegmentStats de agent/internet "aplanado" para SQL, en vez de JSON
//     en una columna — así el rango de la consulta (WHERE at >= ?) usa el
//     índice normal de una PK TEXT ordenable (RFC 3339), igual que
//     cualquier otra fecha de este archivo.
//   - internet_outages: una fila por caída (ver agent/internet.Outage);
//     end NULL mientras siga abierta — a lo sumo UNA fila así a la vez
//     (lo garantiza agent/internet.OutageDetector en memoria, no una
//     restricción de esquema: ver el comentario de OpenOutage).
//   - internet_speed_tests: una fila por prueba de velocidad (ver
//     agent/internet.SpeedTestResult), con error NULL si la prueba salió
//     bien.
//   - internet_settings: fila única (id=1), igual que alert_settings:
//     speedTestsPerDay (ver docs/agente-api.md, "Por defecto 2 al día").
//   - alert_settings suma INTERNET_DOWN/INTERNET_RESTORED, encendidas por
//     defecto como las demás.
const migration5 = `
CREATE TABLE internet_minutes (
	at                TEXT PRIMARY KEY,
	router_sent       INTEGER NOT NULL DEFAULT 0,
	router_lost       INTEGER NOT NULL DEFAULT 0,
	router_min_ms     REAL NOT NULL DEFAULT 0,
	router_avg_ms     REAL NOT NULL DEFAULT 0,
	router_max_ms     REAL NOT NULL DEFAULT 0,
	modem_sent        INTEGER NOT NULL DEFAULT 0,
	modem_lost        INTEGER NOT NULL DEFAULT 0,
	modem_min_ms      REAL NOT NULL DEFAULT 0,
	modem_avg_ms      REAL NOT NULL DEFAULT 0,
	modem_max_ms      REAL NOT NULL DEFAULT 0,
	internet_sent     INTEGER NOT NULL DEFAULT 0,
	internet_lost     INTEGER NOT NULL DEFAULT 0,
	internet_min_ms   REAL NOT NULL DEFAULT 0,
	internet_avg_ms   REAL NOT NULL DEFAULT 0,
	internet_max_ms   REAL NOT NULL DEFAULT 0,
	dns_sent          INTEGER NOT NULL DEFAULT 0,
	dns_lost          INTEGER NOT NULL DEFAULT 0,
	dns_min_ms        REAL NOT NULL DEFAULT 0,
	dns_avg_ms        REAL NOT NULL DEFAULT 0,
	dns_max_ms        REAL NOT NULL DEFAULT 0
);

-- Mismas columnas que internet_minutes (ver el comentario de arriba):
-- misma función de Go (scanInternetAgg) lee las dos.
CREATE TABLE internet_hours (
	at                TEXT PRIMARY KEY,
	router_sent       INTEGER NOT NULL DEFAULT 0,
	router_lost       INTEGER NOT NULL DEFAULT 0,
	router_min_ms     REAL NOT NULL DEFAULT 0,
	router_avg_ms     REAL NOT NULL DEFAULT 0,
	router_max_ms     REAL NOT NULL DEFAULT 0,
	modem_sent        INTEGER NOT NULL DEFAULT 0,
	modem_lost        INTEGER NOT NULL DEFAULT 0,
	modem_min_ms      REAL NOT NULL DEFAULT 0,
	modem_avg_ms      REAL NOT NULL DEFAULT 0,
	modem_max_ms      REAL NOT NULL DEFAULT 0,
	internet_sent     INTEGER NOT NULL DEFAULT 0,
	internet_lost     INTEGER NOT NULL DEFAULT 0,
	internet_min_ms   REAL NOT NULL DEFAULT 0,
	internet_avg_ms   REAL NOT NULL DEFAULT 0,
	internet_max_ms   REAL NOT NULL DEFAULT 0,
	dns_sent          INTEGER NOT NULL DEFAULT 0,
	dns_lost          INTEGER NOT NULL DEFAULT 0,
	dns_min_ms        REAL NOT NULL DEFAULT 0,
	dns_avg_ms        REAL NOT NULL DEFAULT 0,
	dns_max_ms        REAL NOT NULL DEFAULT 0
);

CREATE TABLE internet_outages (
	id    INTEGER PRIMARY KEY AUTOINCREMENT,
	start TEXT NOT NULL,
	end   TEXT,
	blame TEXT NOT NULL
);
CREATE INDEX internet_outages_start_idx ON internet_outages(start);

CREATE TABLE internet_speed_tests (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	at                TEXT NOT NULL,
	down_mbps         REAL NOT NULL DEFAULT 0,
	up_mbps           REAL NOT NULL DEFAULT 0,
	latency_ms        REAL NOT NULL DEFAULT 0,
	loaded_latency_ms REAL NOT NULL DEFAULT 0,
	error             TEXT
);
CREATE INDEX internet_speed_tests_at_idx ON internet_speed_tests(at);

CREATE TABLE internet_settings (
	id                   INTEGER PRIMARY KEY CHECK (id = 1),
	speed_tests_per_day  INTEGER NOT NULL DEFAULT 2
);
INSERT INTO internet_settings (id) VALUES (1);

ALTER TABLE alert_settings ADD COLUMN internet_down      INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_settings ADD COLUMN internet_restored  INTEGER NOT NULL DEFAULT 1;
`
