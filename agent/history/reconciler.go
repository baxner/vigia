package history

import (
	"regexp"
	"strings"

	"github.com/baxner/vigia/core"
)

// randomAndroidName: nombre que Android le pone al hostname cuando
// aleatoriza por privacidad (p. ej. "Android_1a2b3c4d").
var randomAndroidName = regexp.MustCompile(`^Android_[A-Za-z0-9]{8}$`)

// junkName: nombres sin información real que no deben juntar dos equipos
// distintos.
var junkName = regexp.MustCompile(`(?i)^(none(-\d+)?|localhost)$`)

// notBlank: equivalente a Kotlin String?.isNotBlank() — no vacío y no solo
// espacios. (Distinto de "no vacío" a secas, que es lo que se usa para MAC.)
func notBlank(s string) bool { return strings.TrimSpace(s) != "" }

// StrongNameOf es el nombre "fuerte" de un device para reconciliar, o "" si
// no tiene uno útil: lo que anuncia por mDNS/Bonjour (extra["mdns:name"]) o,
// si no hay, el hostname — pero nunca uno que Android aleatoriza por
// privacidad ni basura genérica ("none", "none-2", "localhost"). Normalizado
// a minúsculas: la comparación no distingue mayúsculas. Puerto de
// DeviceReconciler.strongNameOf en Kotlin.
func StrongNameOf(d core.Device) string {
	candidate := d.Extra[ExtraMDNSName]
	if !notBlank(candidate) {
		candidate = d.Hostname
	}
	if !notBlank(candidate) {
		return ""
	}
	if randomAndroidName.MatchString(candidate) || junkName.MatchString(candidate) {
		return ""
	}
	return strings.ToLower(candidate)
}

// Match empareja un Device recién escaneado con el KnownDevice al que
// corresponde, o Known == nil si es un equipo nuevo.
type Match struct {
	Scanned core.Device
	Known   *KnownDevice
}

// Reconcile decide, para cada escaneado, a cuál conocido corresponde (si
// alguno). Reglas en orden (la primera que aplica gana), igual que
// DeviceReconciler.kt. Cada regla es más débil que la anterior, así que cada
// una se bloquea si CONTRADICE a alguna de las más fuertes que ya se
// intentaron (mismo identificador, pero con un valor distinto):
//
//  1. Misma MAC (la "pegajosa" del conocido). Nada más fuerte por delante,
//     no se bloquea por nada.
//  2. Mismo extra["ssdp:udn"] (UUID UPnP) del último Device visto — salvo
//     que la MAC contradiga.
//  3. Mismo "nombre fuerte" (ver StrongNameOf) — salvo que la MAC o el UDN
//     contradigan.
//  4. Misma última IP — salvo que la MAC, el UDN o el nombre fuerte
//     contradigan.
//
// Cada conocido se usa como máximo una vez por escaneo: si dos escaneados
// calificarían para el mismo conocido, solo el primero (en el orden de
// scanned) se lo queda. Sin ninguna regla aplicable, el escaneado es un
// equipo nuevo (Known == nil).
func Reconcile(known []KnownDevice, scanned []core.Device) []Match {
	available := make([]*KnownDevice, len(known))
	for i := range known {
		available[i] = &known[i]
	}
	matches := make([]Match, len(scanned))
	for i, dev := range scanned {
		idx := findMatch(available, dev)
		if idx < 0 {
			matches[i] = Match{Scanned: dev}
			continue
		}
		matches[i] = Match{Scanned: dev, Known: available[idx]}
		available = append(available[:idx], available[idx+1:]...)
	}
	return matches
}

func findMatch(candidates []*KnownDevice, device core.Device) int {
	if device.MAC != "" {
		for i, k := range candidates {
			if k.MAC != nil && *k.MAC == device.MAC {
				return i
			}
		}
	}
	if udn := device.Extra[ExtraSSDPUDN]; notBlank(udn) {
		for i, k := range candidates {
			if k.Last.Extra[ExtraSSDPUDN] == udn && !macConflicts(k, device) {
				return i
			}
		}
	}
	if name := StrongNameOf(device); name != "" {
		for i, k := range candidates {
			if StrongNameOf(k.Last) == name && !macConflicts(k, device) && !udnConflicts(k, device) {
				return i
			}
		}
	}
	for i, k := range candidates {
		if k.LastIP == device.IP && !macConflicts(k, device) && !udnConflicts(k, device) && !nameConflicts(k, device) {
			return i
		}
	}
	return -1
}

// macConflicts: known y device traen cada uno una MAC no vacía, y son
// distintas.
func macConflicts(known *KnownDevice, device core.Device) bool {
	return known.MAC != nil && *known.MAC != "" && device.MAC != "" && *known.MAC != device.MAC
}

// udnConflicts: known y device traen cada uno un ssdp:udn, y son distintos.
func udnConflicts(known *KnownDevice, device core.Device) bool {
	knownUDN := known.Last.Extra[ExtraSSDPUDN]
	deviceUDN := device.Extra[ExtraSSDPUDN]
	return notBlank(knownUDN) && notBlank(deviceUDN) && knownUDN != deviceUDN
}

// nameConflicts: known y device tienen cada uno un nombre fuerte (ver
// StrongNameOf), y son distintos.
func nameConflicts(known *KnownDevice, device core.Device) bool {
	knownName := StrongNameOf(known.Last)
	deviceName := StrongNameOf(device)
	return knownName != "" && deviceName != "" && knownName != deviceName
}
