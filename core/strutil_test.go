package core

import "testing"

// TestIsGarbageHostname cubre los hostnames basura vistos en la red real
// (Echo .62: "none-2") y los que la regla cubre por generalización.
func TestIsGarbageHostname(t *testing.T) {
	garbage := []string{
		"none", "none-2", "None-7",
		"localhost", "localhost-1",
		"unknown", "unknown-3",
		"android", "android-12",
		"aabbccddeeff", "AABBCCDDEEFF", // MAC cruda de 12 hex
		"",
		"   ",
	}
	for _, h := range garbage {
		if !isGarbageHostname(h) {
			t.Errorf("isGarbageHostname(%q) = false, want true", h)
		}
	}

	good := []string{
		"iMac-Estudio",
		"DESKTOP-ABC1234-2", // el sufijo de conflicto no es basura, es otro problema
		"echoshow",
		"DISKSTATION",
		"androidtv-sala", // "android" solo es basura si ES el hostname entero
		"none-of-the-above",
	}
	for _, h := range good {
		if isGarbageHostname(h) {
			t.Errorf("isGarbageHostname(%q) = true, want false", h)
		}
	}
}

func TestCleanHostname(t *testing.T) {
	if got := cleanHostname("none-2"); got != "" {
		t.Errorf("cleanHostname(%q) = %q, want vacío", "none-2", got)
	}
	if got := cleanHostname("echoshow"); got != "echoshow" {
		t.Errorf("cleanHostname(%q) = %q, want sin cambios", "echoshow", got)
	}
}

// TestTrimConflictSuffix: el sufijo "-N" que mDNS agrega en un choque de
// nombre (PC .55: "DESKTOP-ABC1234-2").
func TestTrimConflictSuffix(t *testing.T) {
	cases := map[string]string{
		"DESKTOP-ABC1234-2": "DESKTOP-ABC1234",
		"DESKTOP-ABC1234":   "DESKTOP-ABC1234", // sin sufijo, sin cambios
		"host-99":           "host",
		"iMac-Estudio":      "iMac-Estudio", // termina en letra, no en "-dígitos"
	}
	for in, want := range cases {
		if got := trimConflictSuffix(in); got != want {
			t.Errorf("trimConflictSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsHex12(t *testing.T) {
	if !isHex12("aabbccddeeff") || !isHex12("AABBCCDDEEFF") {
		t.Error("isHex12 debería aceptar 12 hex sin separadores en cualquier caso")
	}
	for _, s := range []string{"aabbccddeef", "aabbccddeeffa", "iMac de Estudio", "0", ""} {
		if isHex12(s) {
			t.Errorf("isHex12(%q) = true, want false", s)
		}
	}
}
