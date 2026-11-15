package core

import (
	"net"
	"regexp"
	"strings"
)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func trimSuffixFold(s, suffix string) string {
	if len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// hex12Re: 12 hex sin separadores — el formato de una MAC cruda
// ("aabbccddeeff"), no un nombre. Se usa tanto para filtrar hostnames basura
// (scan.go) como nombres de instancia mDNS (discover_extra.go).
var hex12Re = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)

func isHex12(s string) bool {
	return hex12Re.MatchString(s)
}

// garbageHostnameRe: hostnames sin valor identificador que algunos equipos
// anuncian igual (el "none-2" del Echo .62: ver AGENTS/hechos medidos).
var garbageHostnameRe = regexp.MustCompile(`(?i)^(none|localhost|unknown|android)(-\d+)?$`)

// isGarbageHostname: sin valor como nombre — ni basura conocida (none-2,
// unknown, android-7...) ni una MAC cruda de 12 hex usada como "nombre".
func isGarbageHostname(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" {
		return true
	}
	return garbageHostnameRe.MatchString(h) || isHex12(h)
}

// cleanHostname: "" si h es basura (ver isGarbageHostname), h sin cambios si no.
func cleanHostname(h string) string {
	if isGarbageHostname(h) {
		return ""
	}
	return h
}

// conflictSuffixRe: el sufijo "-N" que mDNS agrega cuando hay un choque de
// nombre en la red ("DESKTOP-ABC1234-2" cuando ya hay o hubo otro
// "DESKTOP-ABC1234" visto desde otra interfaz/caché).
var conflictSuffixRe = regexp.MustCompile(`-\d+$`)

// trimConflictSuffix quita ese sufijo: "DESKTOP-ABC1234-2" → "DESKTOP-ABC1234".
// Sin sufijo, devuelve h tal cual.
func trimConflictSuffix(h string) string {
	return conflictSuffixRe.ReplaceAllString(h, "")
}

// ipLess ordena IPs numéricamente (v4 antes que v6).
func ipLess(a, b string) bool {
	ipa, ipb := net.ParseIP(a), net.ParseIP(b)
	a4, b4 := ipa.To4() != nil, ipb.To4() != nil
	if a4 != b4 {
		return a4
	}
	if a4 {
		return ipToU32(ipa) < ipToU32(ipb)
	}
	return a < b
}
