package alerts

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/baxner/vigia/agent/history"
)

// junkHostname: hostnames que no dicen nada (puerto de junkHostname en
// android/.../data/model/DeviceType.kt) y randomAndroidName: el hostname
// que Android aleatoriza por privacidad (puerto de randomAndroidName en
// agent/history/reconciler.go, no exportado ahí) — ninguno de los dos
// sirve para mostrarle algo útil al dueño en una alerta.
var (
	junkHostname      = regexp.MustCompile(`(?i)^(none|localhost|unknown|android)(-\d+)?$`)
	randomAndroidName = regexp.MustCompile(`^Android_[A-Za-z0-9]{8}$`)
)

// label es el texto "nombre (IP)" para el cuerpo de una alerta: nombre
// visible = custom.name > mdns:name > hostname > "marca modelo" > marca >
// IP (ver displayName). Nunca lleva correos ni tokens: ninguna de esas
// fuentes los trae. Si no hay nombre mejor que la IP, no la repite entre
// paréntesis ("192.168.0.24 se conectó...", no "192.168.0.24
// (192.168.0.24) se conectó...").
func label(d history.KnownDevice) string {
	name := displayName(d)
	if name == "" || name == d.LastIP {
		return d.LastIP
	}
	return fmt.Sprintf("%s (%s)", name, d.LastIP)
}

// DisplayName expone displayName (ver su comentario) para quien necesite el
// nombre visible SIN la IP entre paréntesis — a diferencia de label(), que
// es para el cuerpo de una alerta. Lo usa agent/api para construir
// SecurityDevice.Name (ver docs/agente-api.md, "Seguridad v1.2": "name ya
// resuelto (personalizado, mDNS, hostname, marca…)"). "" si no hay nada
// mejor que la IP; el motor (core/security.go) ya sabe caer a la IP cuando
// el nombre viene vacío.
func DisplayName(d history.KnownDevice) string { return displayName(d) }

// displayName: puerto simplificado de Device.nameOrNull en
// android/.../data/model/DeviceType.kt para el cuerpo de una alerta —
// mismo orden de fuentes (custom.name > mdns:name > hostname > "marca
// modelo" > marca), sin el último fallback a vendor crudo del OUI: el
// contrato de alertas (docs/agente-api.md) no lo pide y una marca ya
// reconocida (identity.Brand) es más legible que la razón social del
// fabricante.
func displayName(d history.KnownDevice) string {
	if d.Custom.Name != nil {
		if n := strings.TrimSpace(*d.Custom.Name); n != "" {
			return n
		}
	}
	last := d.Last
	if n := strings.TrimSpace(last.Extra[history.ExtraMDNSName]); n != "" {
		return n
	}
	if h := strings.TrimSpace(last.Hostname); h != "" && !junkHostname.MatchString(h) && !randomAndroidName.MatchString(h) {
		return h
	}
	brand := strings.TrimSpace(last.Identity.Brand)
	model := strings.TrimSpace(last.Identity.Model)
	switch {
	case brand != "" && model != "":
		return brand + " " + model
	case brand != "":
		return brand
	default:
		return ""
	}
}
