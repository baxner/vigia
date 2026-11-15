package core

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

// data/oui.txt: prefijos OUI (MAC → fabricante). Formato por línea "XXXXXX Nombre".
// Origen: datos públicos del IEEE (vía nmap-mac-prefixes). Va embebido para que el
// motor sea autosuficiente en macOS, en Linux/Pi y dentro de la app.
//
//go:embed data/oui.txt
var ouiData string

var (
	ouiOnce sync.Once
	ouiMap  map[string]string
)

func loadOUI() map[string]string {
	ouiOnce.Do(func() {
		ouiMap = make(map[string]string, 40000)
		sc := bufio.NewScanner(strings.NewReader(ouiData))
		for sc.Scan() {
			line := sc.Text()
			if len(line) < 8 || line[0] == '#' {
				continue
			}
			// "XXXXXX Vendor..." — 6 hex, un espacio, el resto es el nombre.
			prefix := line[:6]
			if !isHex6(prefix) || line[6] != ' ' {
				continue
			}
			ouiMap[strings.ToUpper(prefix)] = strings.TrimSpace(line[7:])
		}
	})
	return ouiMap
}

// vendorForMAC devuelve el fabricante de una MAC, o "" si no se reconoce o la MAC
// es localmente administrada (bit U/L = 1: direcciones aleatorias de privacidad).
func vendorForMAC(mac string) string {
	norm := normalizeMAC(mac)
	if norm == "" {
		return ""
	}
	if isLocallyAdministered(norm) {
		return "" // MAC aleatoria: el OUI no identifica al fabricante
	}
	hex := strings.ToUpper(strings.ReplaceAll(norm, ":", ""))
	if len(hex) < 6 {
		return ""
	}
	return loadOUI()[hex[:6]]
}

// normalizeMAC pasa formatos separados por ':' o '-' (incluidos octetos de
// 1 solo dígito como "8:0:27:f0:fb:f9" de `arp -an` en macOS) o 12 hex seguidos
// ("aabb.ccdd.eeff", "aabbccddeeff") a "aa:bb:cc:dd:ee:ff". Devuelve "" si no es válida.
func normalizeMAC(mac string) string {
	s := strings.TrimSpace(mac)
	if s == "" {
		return ""
	}
	hasColon := strings.ContainsRune(s, ':')
	hasDash := strings.ContainsRune(s, '-')
	if hasColon && hasDash {
		return ""
	}
	if hasColon || hasDash {
		sep := ":"
		if hasDash {
			sep = "-"
		}
		parts := strings.Split(s, sep)
		if len(parts) == 6 {
			var out strings.Builder
			for i, p := range parts {
				if len(p) != 1 && len(p) != 2 {
					return ""
				}
				lo, ok := lowerHex(p)
				if !ok {
					return ""
				}
				if i > 0 {
					out.WriteByte(':')
				}
				if len(lo) == 1 {
					out.WriteByte('0')
				}
				out.WriteString(lo)
			}
			return out.String()
		}
		if hasDash && len(parts) == 3 {
			return normalizeHexGroups4(parts)
		}
		return ""
	}
	if strings.ContainsRune(s, '.') {
		parts := strings.Split(s, ".")
		if len(parts) != 3 {
			return ""
		}
		return normalizeHexGroups4(parts)
	}
	if len(s) != 12 {
		return ""
	}
	lo, ok := lowerHex(s)
	if !ok {
		return ""
	}
	return format12Hex(lo)
}

func normalizeHexGroups4(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		if len(p) != 4 {
			return ""
		}
		lo, ok := lowerHex(p)
		if !ok {
			return ""
		}
		b.WriteString(lo)
	}
	return format12Hex(b.String())
}

func lowerHex(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
			b.WriteByte(c)
		case c >= 'A' && c <= 'F':
			b.WriteByte(c + ('a' - 'A'))
		default:
			return "", false
		}
	}
	return b.String(), true
}

func format12Hex(h string) string {
	var out strings.Builder
	out.Grow(17)
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			out.WriteByte(':')
		}
		out.WriteString(h[i : i+2])
	}
	return out.String()
}

// isLocallyAdministered: segundo bit menos significativo del primer octeto.
func isLocallyAdministered(normMAC string) bool {
	if len(normMAC) < 2 {
		return false
	}
	v := hexVal(normMAC[0])<<4 | hexVal(normMAC[1])
	return v&0x02 != 0
}

func isHex6(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}
