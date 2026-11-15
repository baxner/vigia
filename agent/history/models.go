// Package history reconcilia la identidad de los dispositivos entre
// escaneos y mantiene el historial de eventos (alta, caída, reconexión,
// cambio de IP). Es un puerto directo a Go de DeviceReconciler.kt y
// DeviceHistoryEngine.kt (android/app/src/main/kotlin/com/baxner/vigia/data/history/):
// mismas reglas, mismos casos límite, mismos nombres donde tiene sentido.
// Paquete puro: sin IO, sin reloj propio (quien llama pasa `at`), para poder
// probarlo con listas en memoria igual que el original en los tests de
// Kotlin (DeviceReconcilerTest.kt, DeviceHistoryEngineTest.kt).
package history

import (
	"time"

	"github.com/baxner/vigia/core"
)

// Claves de core.Device.Extra que trae el motor y que la reconciliación usa
// como identificadores fuertes. Mismos nombres que ExtraSsdpUdn/ExtraMdnsName
// en DeviceReconciler.kt (ver docs/agente-api.md, "Reglas del historial").
const (
	ExtraSSDPUDN  = "ssdp:udn"
	ExtraMDNSName = "mdns:name"
)

// NetworkKey identifica una red vigilada: "<gateway>|<subred>" (ver
// docs/agente-api.md, "Red del agente vs. red de la app"). A diferencia de
// la app —que además mete el SSID en la clave—, el agente va por cable y no
// lo conoce.
type NetworkKey string

// NewNetworkKey construye la clave a partir del gateway y la subred tal como
// los devuelve core.Scan (ScanResult.Gateway, ScanResult.Subnet, ya en forma
// de red: "192.168.0.0/24", no la IP de un host).
func NewNetworkKey(gateway, subnet string) NetworkKey {
	return NetworkKey(gateway + "|" + subnet)
}

// DeviceCustomization es lo que el usuario cambió a mano (pantalla Editar de
// la app, o un PATCH al agente). Punteros nil = automático / sin
// personalizar — ver la semántica de PATCH en docs/agente-api.md ("campos
// ausentes = sin cambio; null = borrar").
type DeviceCustomization struct {
	Name      *string `json:"name"`
	TypeID    *string `json:"typeId"`
	Notes     *string `json:"notes"`
	Location  *string `json:"location"`
	Important bool    `json:"important"`
}

// KnownDevice es un equipo conocido en una red, conectado o no. Corresponde
// 1:1 al KnownDevice del contrato (docs/agente-api.md) y de HistoryModels.kt.
type KnownDevice struct {
	ID      int64      `json:"id"`
	Network NetworkKey `json:"network"`
	// MAC "pegajosa": una vez conocida no se olvida aunque un escaneo
	// puntual no la vuelva a resolver (ver Seen).
	MAC         *string    `json:"mac"`
	LastIP      string     `json:"lastIp"`
	FirstSeen   time.Time  `json:"firstSeen"`
	LastSeen    time.Time  `json:"lastSeen"`
	Online      bool       `json:"online"`
	OnlineSince *time.Time `json:"onlineSince"`
	// Last es el core.Device tal cual del último escaneo que lo vio (o, si
	// está sin conexión, el que lo confirmó por última vez).
	Last   core.Device         `json:"last"`
	Custom DeviceCustomization `json:"custom"`
}

// DeviceEventType enumera los eventos de la cronología de un equipo (ver
// DeviceEventType en HistoryModels.kt).
type DeviceEventType string

const (
	FirstSeen   DeviceEventType = "FIRST_SEEN"
	CameOnline  DeviceEventType = "CAME_ONLINE"
	WentOffline DeviceEventType = "WENT_OFFLINE"
	IPChanged   DeviceEventType = "IP_CHANGED"
)

// DeviceEvent es un evento de la cronología de un equipo (ver DeviceEvent en
// el contrato).
type DeviceEvent struct {
	DeviceID int64           `json:"deviceId"`
	Type     DeviceEventType `json:"type"`
	At       time.Time       `json:"at"`
	Detail   *string         `json:"detail,omitempty"`
}
