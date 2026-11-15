package core

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

// data/apple-models.txt: identificadores de dispositivos Apple → nombres comerciales.
// Compilado de:
//   - Macs: SIMachineAttributes.plist de Apple (framework privado)
//   - Otros: github.com/clo4/apple_device_identifiers (Unlicense/public domain)
//
// Embebido para que funcione en cualquier entorno (iMac, Pi, apps).
//
//go:embed data/apple-models.txt
var appleModelsData string

var (
	appleModelsOnce sync.Once
	appleModelsMap  map[string]string
)

func loadAppleModels() map[string]string {
	appleModelsOnce.Do(func() {
		appleModelsMap = make(map[string]string, 400)
		sc := bufio.NewScanner(strings.NewReader(appleModelsData))
		for sc.Scan() {
			line := sc.Text()
			// Ignorar líneas vacías y comentarios.
			if len(line) == 0 || line[0] == '#' {
				continue
			}
			// Formato: "identificador<TAB>nombre comercial".
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			id := strings.TrimSpace(parts[0])
			name := strings.TrimSpace(parts[1])
			if id != "" && name != "" {
				// Clave en mayúsculas para búsqueda insensible a caso.
				appleModelsMap[strings.ToUpper(id)] = name
			}
		}
	})
	return appleModelsMap
}

// AppleModelName devuelve el nombre comercial de un dispositivo Apple dado su
// identificador de modelo (p. ej. "iMac18,3" → "iMac (Retina 5K, 27-inch, 2017)").
// La búsqueda no distingue mayúsculas. Devuelve "" si no se reconoce.
func AppleModelName(id string) string {
	name, ok := loadAppleModels()[strings.ToUpper(id)]
	if !ok {
		return ""
	}
	return name
}
