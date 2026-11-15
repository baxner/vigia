package core

import (
	"bufio"
	"strings"
	"testing"
)

func TestAppleModelNameBasics(t *testing.T) {
	tests := []struct {
		id       string
		contains []string // Substrings que debe contener el nombre
	}{
		// Macs: el nombre debe contener el modelo y el año
		{"iMac18,3", []string{"iMac", "2017"}},
		{"MacBookPro11,1", []string{"MacBook Pro"}},
		{"MacBookAir4,2", []string{"MacBook Air", "2011"}},
		{"Macmini7,1", []string{"Mac mini"}},
		{"MacPro6,1", []string{"Mac Pro"}},

		// iPhone: debe contener "iPhone" y el número de modelo
		{"iPhone15,2", []string{"iPhone", "14 Pro"}},
		{"iPhone14,7", []string{"iPhone", "14"}},
		{"iPhone1,1", []string{"iPhone"}},

		// iPad: debe contener "iPad"
		{"iPad13,1", []string{"iPad", "Air", "4th generation"}},
		{"iPad14,5", []string{"iPad Pro", "12.9"}},

		// Apple Watch: debe contener "Apple Watch"
		{"Watch6,1", []string{"Apple Watch", "6"}},
		{"Watch6,18", []string{"Apple Watch", "Ultra"}},

		// Apple TV: debe contener "Apple TV"
		{"AppleTV11,1", []string{"Apple TV", "4K", "2nd generation"}},

		// HomePod: debe contener "HomePod"
		{"AudioAccessory5,1", []string{"HomePod", "Mini"}},

		// iPod touch: debe contener "iPod"
		{"iPod9,1", []string{"iPod touch", "7th generation"}},
	}

	for _, tt := range tests {
		name := AppleModelName(tt.id)
		if name == "" {
			t.Errorf("AppleModelName(%q) = \"\", want non-empty", tt.id)
			continue
		}

		for _, substr := range tt.contains {
			if !strings.Contains(name, substr) {
				t.Errorf("AppleModelName(%q) = %q, should contain %q", tt.id, name, substr)
			}
		}
	}
}

func TestAppleModelNameCaseSensitive(t *testing.T) {
	tests := []string{
		"imac18,3",
		"IMAC18,3",
		"iMac18,3",
		"ImAc18,3",
	}

	var firstResult string
	for i, id := range tests {
		result := AppleModelName(id)
		if result == "" {
			t.Errorf("AppleModelName(%q) = \"\", want non-empty", id)
			continue
		}

		if i == 0 {
			firstResult = result
		} else if result != firstResult {
			t.Errorf("AppleModelName(%q) = %q, want %q (case insensitivity)", id, result, firstResult)
		}
	}
}

func TestAppleModelNameUnknown(t *testing.T) {
	tests := []string{
		"Unknown1,1",
		"FakeDevice99,99",
		"NotARealModel",
		"",
	}

	for _, id := range tests {
		result := AppleModelName(id)
		if result != "" {
			t.Errorf("AppleModelName(%q) = %q, want \"\"", id, result)
		}
	}
}

func TestAppleModelNameCount(t *testing.T) {
	// Verificar que tenemos una cantidad razonable de modelos.
	models := loadAppleModels()
	if len(models) < 350 { // Esperamos al menos 350 modelos
		t.Errorf("loadAppleModels() returned %d models, want at least 350", len(models))
	}
}

func TestAppleModelNameAllFamilies(t *testing.T) {
	// Verificar que cada familia de dispositivos está representada.
	families := map[string]bool{
		"IMAC":           false,
		"MACBOOKPRO":     false,
		"IPHONE":         false,
		"IPAD":           false,
		"WATCH":          false,
		"APPLETV":        false,
		"AUDIOACCESSORY": false, // HomePod
		"IPOD":           false,
	}

	models := loadAppleModels()
	for id := range models {
		for family := range families {
			if strings.HasPrefix(id, family) {
				families[family] = true
			}
		}
	}

	for family, found := range families {
		if !found {
			t.Errorf("No models found for family %q", family)
		}
	}
}

func TestAppleModelNameNoDuplicates(t *testing.T) {
	// Verificar que no hay identificadores duplicados en el archivo embebido.
	// Esto protege contra problemas de fusión de múltiples fuentes.
	models := loadAppleModels()
	seen := make(map[string]int, len(models))

	// Procesar el archivo embebido línea por línea para detectar duplicados
	scanner := bufio.NewScanner(strings.NewReader(appleModelsData))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.ToUpper(strings.TrimSpace(parts[0]))
		if id != "" {
			seen[id]++
		}
	}

	// Reportar duplicados
	for id, count := range seen {
		if count > 1 {
			t.Errorf("Duplicate identifier %q found %d times in apple-models.txt", id, count)
		}
	}
}
