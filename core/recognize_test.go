package core

import (
	"strings"
	"testing"
)

func recog(d Device) Identity { return recognize(&d) }

// TestRecognize cubre los casos sintéticos del criterio de revisión: una
// mezcla de señales SSDP/mDNS/puertos/hostname/OUI por caso, y lo que
// recognize() debería deducir de cada combinación.
func TestRecognize(t *testing.T) {
	cases := []struct {
		name      string
		dev       Device
		wantType  string
		wantBrand string // "" = no se verifica
		wantOS    string // "" = no se verifica
		minConf   int
	}{
		{
			name: "router Tenda (vendor + ssdp Mesh + puerto UPnP)",
			dev: Device{
				Vendor:    "Tenda Technology Co.,Ltd.",
				Services:  []Service{{Proto: "ssdp", Name: "Mesh Wi-Fi", Info: "Tenda"}},
				OpenPorts: []int{49152},
			},
			wantType:  "router",
			wantBrand: "Tenda",
			minConf:   70,
		},
		{
			name: "PC Windows (hostname DESKTOP-ABC + netbios + ssdp + 139/445)",
			dev: Device{
				Hostname: "DESKTOP-ABC",
				Vendor:   "Giga-byte Technology Co., Ltd.",
				Services: []Service{
					{Proto: "netbios", Name: "DESKTOP-ABC"},
					{Proto: "ssdp", Name: "Windows Media Player"},
				},
				OpenPorts: []int{139, 445},
			},
			wantType: "computer",
			wantOS:   "Windows",
			minConf:  80,
		},
		{
			name: "TV Samsung (ssdp MediaRenderer + modelName + mdns airplay)",
			dev: Device{
				Vendor: "Samsung Electronics Co.,Ltd",
				Services: []Service{
					{Proto: "ssdp", Name: "QN75Q70CDFXZA", Info: "Samsung Electronics"},
					{Proto: "mdns", Name: "_airplay._tcp"},
				},
				Extra: map[string]string{
					"ssdp:deviceType":   "urn:schemas-upnp-org:device:MediaRenderer:1",
					"ssdp:modelName":    "QN75Q70CDFXZA",
					"ssdp:friendlyName": "[TV] Samsung Q70 Series (75)",
					"ssdp:manufacturer": "Samsung Electronics",
				},
			},
			wantType:  "tv",
			wantBrand: "Samsung",
			minConf:   90,
		},
		{
			name: "TV Samsung, formato viejo (solo Service ssdp: modelo + fabricante)",
			dev: Device{
				Vendor: "Samsung Electronics Co.,Ltd",
				Services: []Service{
					{Proto: "ssdp", Name: "QN75Q70CDFXZA", Info: "Samsung"},
					{Proto: "mdns", Name: "_airplay._tcp"},
				},
			},
			wantType:  "tv",
			wantBrand: "Samsung",
			minConf:   70,
		},
		{
			name: "camara AltoBeam (vendor + puerto 554 RTSP)",
			dev: Device{
				Vendor:    "AltoBeam (Beijing) Inc.",
				OpenPorts: []int{80, 554},
			},
			wantType: "camera",
			minConf:  60,
		},
		{
			name: "Echo (vendor Amazon + mdns spotify-connect + hostname echoshow)",
			dev: Device{
				Vendor:   "Amazon Technologies Inc.",
				Hostname: "echoshow-abcd",
				Services: []Service{{Proto: "mdns", Name: "_spotify-connect._tcp"}},
			},
			wantType:  "speaker",
			wantBrand: "Amazon",
			minConf:   40,
		},
		{
			name: "Android con MAC aleatoria (sin vendor, hostname + nearbypresence)",
			dev: Device{
				Vendor:   "",
				Hostname: "Android_ABC",
				Services: []Service{{Proto: "mdns", Name: "_nearbypresence._tcp"}},
			},
			wantType: "phone",
			wantOS:   "Android",
			minConf:  70,
		},
		{
			name: "consola Nintendo (solo vendor)",
			dev: Device{
				Vendor: "Nintendo Co., Ltd.",
			},
			wantType:  "console",
			wantBrand: "Nintendo",
			minConf:   30,
		},
		{
			name: "iot Tuya (solo vendor)",
			dev: Device{
				Vendor: "Tuya Smart Inc.",
			},
			wantType: "iot",
			minConf:  30,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.dev
			got := recognize(&d)

			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q (reason=%q)", got.Type, tc.wantType, got.Reason)
			}
			if tc.wantBrand != "" && !strings.Contains(got.Brand, tc.wantBrand) {
				t.Errorf("Brand = %q, want que contenga %q", got.Brand, tc.wantBrand)
			}
			if tc.wantOS != "" && got.OS != tc.wantOS {
				t.Errorf("OS = %q, want %q", got.OS, tc.wantOS)
			}
			if got.Confidence < tc.minConf {
				t.Errorf("Confidence = %d, want >= %d (reason=%q)", got.Confidence, tc.minConf, got.Reason)
			}
			if got.Confidence > 99 {
				t.Errorf("Confidence = %d, nunca debe superar 99", got.Confidence)
			}
			if got.Type != "unknown" && got.Reason == "" {
				t.Errorf("Reason vacío para un Type reconocido (%q): debe ser transparente", got.Type)
			}
		})
	}
}

// TestRecognizeUnknown: sin ninguna señal, el resultado debe ser unknown/0,
// no una confianza arbitraria.
func TestRecognizeUnknown(t *testing.T) {
	d := Device{}
	got := recognize(&d)
	if got.Type != "unknown" {
		t.Errorf("Type = %q, want %q", got.Type, "unknown")
	}
	if got.Confidence != 0 {
		t.Errorf("Confidence = %d, want 0", got.Confidence)
	}
}

// TestRecognizeVendorOnlyFloor: un fabricante sin regla específica en
// vendorRules (p. ej. un OEM chico cualquiera) no debe quedar en confianza 0:
// el piso es 10, igual que documenta el contrato.
func TestRecognizeVendorOnlyFloor(t *testing.T) {
	d := Device{Vendor: "Qingdao Intelligent Appliance Co.,Ltd"}
	got := recognize(&d)
	if got.Confidence == 0 {
		t.Errorf("Confidence = 0 con Vendor conocido %q, quiere piso de 10", d.Vendor)
	}
	if got.Brand == "" {
		t.Errorf("Brand vacío con Vendor conocido %q", d.Vendor)
	}
}

// TestRecognizeRandomMACNotZero: una MAC aleatoria (Vendor vacío) con
// hostname/servicios no debe degradar la confianza a 0 — es el caso que más
// se parece a un falso negativo de Fing.
func TestRecognizeRandomMACNotZero(t *testing.T) {
	d := Device{Hostname: "Android_ABC", Services: []Service{{Proto: "mdns", Name: "_nearbypresence._tcp"}}}
	got := recognize(&d)
	if got.Confidence == 0 {
		t.Errorf("Confidence = 0 con hostname/servicios presentes y Vendor vacío")
	}
}

// TestRecognizeBrandPriority: una TV Samsung que además anuncia _airplay (un
// protocolo de Apple) debe seguir siendo Samsung — el fabricante por OUI/SSDP
// gana sobre lo que insinúe el protocolo.
func TestRecognizeBrandPriority(t *testing.T) {
	d := Device{
		Vendor: "Samsung Electronics Co.,Ltd",
		Services: []Service{
			{Proto: "ssdp", Name: "QN75Q70CDFXZA", Info: "Samsung"},
			{Proto: "mdns", Name: "_airplay._tcp"},
		},
	}
	got := recognize(&d)
	if strings.Contains(strings.ToLower(got.Brand), "apple") {
		t.Errorf("Brand = %q, _airplay no debe imponer Apple sobre un Samsung", got.Brand)
	}
	if !strings.Contains(got.Brand, "Samsung") {
		t.Errorf("Brand = %q, want que contenga Samsung", got.Brand)
	}
}

// TestRecognizeFalsosPositivos: casos de la auditoría contra los 49k
// fabricantes del OUI: claves que calzaban como subcadena y no deben calzar.
func TestRecognizeFalsosPositivos(t *testing.T) {
	cases := []struct {
		name    string
		dev     Device
		notType string
	}{
		{"Engineering no es Ring", Device{Vendor: "Totsu Engineering"}, "camera"},
		{"Intelligent no es Intel", Device{Vendor: "Qingdao Intelligent Appliance Co.,Ltd"}, "computer"},
		{"Pegasus no es Asus", Device{Vendor: "Pegasus Technologies"}, "router"},
		{"Amazonia no es Amazon", Device{Vendor: "Digitron da Amazonia S/A"}, "speaker"},
		{"CCTV no es TV", Device{Extra: map[string]string{"ssdp:modelName": "CCTV-NVR16", "ssdp:friendlyName": "CCTV DVR"}}, "tv"},
		{"core-switch no es consola", Device{Hostname: "core-switch-01"}, "console"},
		{"ups5 no es PlayStation", Device{Hostname: "ups5"}, "console"},
		{"techo no es Echo", Device{Hostname: "luz-techo"}, "speaker"},
		{"Mac con AirPlay no es TV", Device{
			Vendor:   "Apple, Inc.",
			Services: []Service{{Proto: "mdns", Name: "_airplay._tcp"}, {Proto: "mdns", Name: "_companion-link._tcp"}},
		}, "tv"},
		{"Mac con compartir archivos no es NAS", Device{
			Vendor:    "Apple, Inc.",
			Services:  []Service{{Proto: "mdns", Name: "_smb._tcp"}, {Proto: "mdns", Name: "_adisk._tcp"}},
			OpenPorts: []int{445},
		}, "nas"},
		{"PC con Windows Media Player no es TV", Device{Extra: map[string]string{
			"ssdp:deviceType":   "urn:schemas-upnp-org:device:MediaRenderer:1",
			"ssdp:modelName":    "Windows Media Player",
			"ssdp:friendlyName": "DESKTOP-ABC: usuario",
			"ssdp:manufacturer": "Microsoft Corporation",
		}}, "tv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := recog(tc.dev); got.Type == tc.notType {
				t.Errorf("Type = %q, no debía (reason=%q)", got.Type, got.Reason)
			}
		})
	}
}

// TestRecognizeCasosReales: combinaciones vistas en la red real (datos
// anonimizados) y lo que debe salir de cada una.
func TestRecognizeCasosReales(t *testing.T) {
	cases := []struct {
		name                        string
		dev                         Device
		wantType, wantBrand, wantOS string
		wantModel                   string
		minConf                     int
	}{
		{
			name: "PC Windows sin SSDP: 445 + DESKTOP- + Giga-byte ganan al 554",
			dev: Device{
				Hostname:  "DESKTOP-ABC1234",
				Vendor:    "Giga-byte Technology Co., Ltd.",
				OpenPorts: []int{139, 443, 445, 554},
			},
			wantType: "computer", wantBrand: "Gigabyte", wantOS: "Windows", minConf: 60,
		},
		{
			name: "PC Windows con Windows Media Player (deviceType MediaRenderer)",
			dev: Device{
				Hostname: "DESKTOP-ABC1234",
				Vendor:   "Giga-byte Technology Co., Ltd.",
				Extra: map[string]string{
					"ssdp:deviceType":   "urn:schemas-upnp-org:device:MediaRenderer:1",
					"ssdp:modelName":    "Windows Media Player",
					"ssdp:friendlyName": "DESKTOP-ABC1234: usuario",
					"ssdp:manufacturer": "Microsoft Corporation",
				},
				OpenPorts: []int{139, 445, 554},
			},
			wantType: "computer", wantBrand: "Gigabyte", wantOS: "Windows", minConf: 90,
		},
		{
			name:     "iPhone por modelo mDNS",
			dev:      Device{Extra: map[string]string{"mdns:model": "iPhone15,2"}},
			wantType: "phone", wantBrand: "Apple", wantOS: "iOS", wantModel: "iPhone 14 Pro", minConf: 90,
		},
		{
			name: "Apple TV por modelo mDNS, aunque _airplay no cuente en un Apple",
			dev: Device{
				Vendor:   "Apple, Inc.",
				Services: []Service{{Proto: "mdns", Name: "_airplay._tcp"}},
				Extra:    map[string]string{"mdns:model": "AppleTV6,2"},
			},
			wantType: "tv", wantBrand: "Apple", wantOS: "tvOS", wantModel: "Apple TV 4K (1st generation)", minConf: 90,
		},
		{
			name: "NAS Synology con SMB: 445 no lo vuelve Windows",
			dev: Device{
				Vendor:    "Synology Incorporated",
				Services:  []Service{{Proto: "mdns", Name: "_smb._tcp"}, {Proto: "netbios", Name: "DISKSTATION"}},
				OpenPorts: []int{139, 445, 5000},
			},
			wantType: "nas", wantBrand: "Synology", minConf: 60,
		},
		{
			name: "router Tenda con campos SSDP separados, sin modelo basura",
			dev: Device{
				Vendor: "Tenda Technology Co.,Ltd.Dongguan branch",
				Extra: map[string]string{
					"ssdp:deviceType":   "urn:schemas-upnp-org:device:InternetGatewayDevice:1",
					"ssdp:friendlyName": "Mesh Wi-Fi",
					"ssdp:modelName":    "Mesh Wi-Fi",
					"ssdp:manufacturer": "Tenda",
				},
				OpenPorts: []int{80, 49152},
			},
			wantType: "router", wantBrand: "Tenda", minConf: 90,
		},
		{
			name:     "Galaxy con MAC aleatoria: la marca sale del hostname",
			dev:      Device{Hostname: "Galaxy-S24-Ultra"},
			wantType: "phone", wantBrand: "Samsung", wantOS: "Android", minConf: 40,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := recog(tc.dev)
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q (reason=%q)", got.Type, tc.wantType, got.Reason)
			}
			if tc.wantBrand != "" && got.Brand != tc.wantBrand {
				t.Errorf("Brand = %q, want %q", got.Brand, tc.wantBrand)
			}
			if tc.wantOS != "" && got.OS != tc.wantOS {
				t.Errorf("OS = %q, want %q", got.OS, tc.wantOS)
			}
			if got.Model != tc.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, tc.wantModel)
			}
			if got.Confidence < tc.minConf || got.Confidence > 99 {
				t.Errorf("Confidence = %d, want %d..99 (reason=%q)", got.Confidence, tc.minConf, got.Reason)
			}
		})
	}
}

// TestRecognizeSinOSAjeno: el OS de una señal de otro tipo no se cuela (una TV
// con el 445 abierto no es "Windows").
func TestRecognizeSinOSAjeno(t *testing.T) {
	got := recog(Device{
		Extra:     map[string]string{"ssdp:deviceType": "urn:schemas-upnp-org:device:MediaRenderer:1", "ssdp:manufacturer": "Samsung Electronics"},
		OpenPorts: []int{445},
	})
	if got.Type != "tv" || got.OS != "" {
		t.Errorf("Type=%q OS=%q, want tv sin OS (reason=%q)", got.Type, got.OS, got.Reason)
	}
}

// TestRecognizeSelf: del equipo que escanea se sabe el SO con certeza.
func TestRecognizeSelf(t *testing.T) {
	got := recog(Device{Extra: map[string]string{"self": "true"}})
	want := selfRule()
	if got.Type != want.Type || got.OS != want.OS || got.Confidence < want.Conf {
		t.Errorf("self = %+v, want Type %q OS %q conf >= %d", got, want.Type, want.OS, want.Conf)
	}
	if !strings.Contains(got.Reason, "equipo que escanea") {
		t.Errorf("Reason = %q, debe decir que es el equipo que escanea", got.Reason)
	}
}

// TestRecognizeDeterminista: mismo dispositivo → misma identidad (las tablas
// en map no deben filtrar su orden aleatorio al resultado).
func TestRecognizeDeterminista(t *testing.T) {
	d := Device{
		Vendor:    "Samsung Electronics Co.,Ltd",
		Hostname:  "galaxy-tab",
		Services:  []Service{{Proto: "mdns", Name: "_airplay._tcp"}, {Proto: "mdns", Name: "_spotify-connect._tcp"}},
		OpenPorts: []int{7000, 8008, 8009, 445},
		Extra:     map[string]string{"ssdp:manufacturer": "Samsung Electronics", "ssdp:friendlyName": "Samsung Sonos Roku"},
	}
	first := recog(d)
	for i := 0; i < 200; i++ {
		if got := recog(d); got != first {
			t.Fatalf("iteración %d: %+v != %+v", i, got, first)
		}
	}
}

// ---------------------------------------------------------------------------
// identificacion-v2: API HTTP de TV, osxvers, título HTTP, agrupación de
// puertos, redacción de correos y las reglas mDNS nuevas (ver AGENTS.md).
// ---------------------------------------------------------------------------

// TestRecognizeIMacRealWorldCombination prueba que con mdns:model + mdns:os
// ya en Extra (del TXT huérfano de _device-info, ver discover_extra_test.go),
// un iMac se reconoce con alta confianza y el nombre comercial exacto del SO,
// no solo el "macOS" genérico del hostname.
func TestRecognizeIMacRealWorldCombination(t *testing.T) {
	d := Device{
		Hostname: "iMac-Estudio",
		Services: []Service{{Proto: "mdns", Name: "_device-info._tcp"}, {Proto: "mdns", Name: "_rfb._tcp"}},
		Extra:    map[string]string{"mdns:model": "iMac18,3", "mdns:os": "24"},
	}
	got := recog(d)
	if got.Type != "computer" || got.Brand != "Apple" {
		t.Errorf("Type/Brand = %q/%q, want computer/Apple (reason=%q)", got.Type, got.Brand, got.Reason)
	}
	if got.OS != "macOS 15 Sequoia" {
		t.Errorf("OS = %q, want %q (osxvers=24 debe ganarle al \"macOS\" genérico del modelo)", got.OS, "macOS 15 Sequoia")
	}
	if got.Confidence < 90 {
		t.Errorf("Confidence = %d, want >= 90", got.Confidence)
	}
}

func TestRecognizeOSXVersionUnknownIgnored(t *testing.T) {
	// Un osxvers fuera de la tabla no debe inventar un nombre: se queda con
	// el "macOS" genérico del modelo en vez de "".
	got := recog(Device{Extra: map[string]string{"mdns:model": "iMac18,3", "mdns:os": "99"}})
	if got.OS != "macOS" {
		t.Errorf("OS = %q, want %q (99 no está en osxVersionNames)", got.OS, "macOS")
	}
}

// TestRecognizeAPISamsung/Roku/Chromecast: la API HTTP propia del aparato
// (core/httpfp.go) manda sobre SSDP/mDNS — confirma modelo/SO exactos contra
// la API del fabricante.
func TestRecognizeAPISamsung(t *testing.T) {
	got := recog(Device{
		OpenPorts: []int{8001},
		Extra: map[string]string{
			"api:type":  "tv",
			"api:os":    "Tizen",
			"api:model": "QN75Q70CDFXZA",
		},
	})
	if got.Type != "tv" || got.Brand != "Samsung" || got.OS != "Tizen" || got.Model != "QN75Q70CDFXZA" {
		t.Errorf("got %+v (reason=%q)", got, got.Reason)
	}
	if got.Confidence < 95 {
		t.Errorf("Confidence = %d, want >= 95", got.Confidence)
	}
}

func TestRecognizeAPIRoku(t *testing.T) {
	got := recog(Device{
		OpenPorts: []int{8060},
		Extra:     map[string]string{"api:type": "tv", "api:os": "Roku OS", "api:model": "Roku Ultra"},
	})
	if got.Type != "tv" || got.Brand != "Roku" {
		t.Errorf("got %+v (reason=%q)", got, got.Reason)
	}
	if got.Confidence < 95 {
		t.Errorf("Confidence = %d, want >= 95", got.Confidence)
	}
}

func TestRecognizeAPIChromecast(t *testing.T) {
	got := recog(Device{
		OpenPorts: []int{8008},
		Extra:     map[string]string{"api:type": "tv", "api:os": "Cast", "api:model": "Chromecast Ultra"},
	})
	if got.Type != "tv" || got.Brand != "Google" {
		t.Errorf("got %+v (reason=%q)", got, got.Reason)
	}
	if got.Confidence < 90 {
		t.Errorf("Confidence = %d, want >= 90", got.Confidence)
	}
}

// TestRecognizeAPIModelOverridesSSDP: la API es más confiable que el
// modelName SSDP (puede venir de un XML UPnP genérico/desactualizado).
func TestRecognizeAPIModelOverridesSSDP(t *testing.T) {
	got := recog(Device{
		OpenPorts: []int{8001},
		Extra: map[string]string{
			"api:type":          "tv",
			"api:model":         "QN75Q70CDFXZA",
			"ssdp:modelName":    "QCQ70D",
			"ssdp:manufacturer": "Samsung Electronics",
		},
	})
	if got.Model != "QN75Q70CDFXZA" {
		t.Errorf("Model = %q, want el de la API (%q), no el de SSDP", got.Model, "QN75Q70CDFXZA")
	}
}

// TestRecognizeHTTPTitle: reglas de <title> HTTP (core/httpfp.go). "Web
// Viewer" es débil a propósito: sube con el puerto 554 (RTSP) por el
// mecanismo normal de combinación de evidencia, no por un caso especial.
func TestRecognizeHTTPTitleCameraWeakAloneStrongWith554(t *testing.T) {
	weak := recog(Device{Extra: map[string]string{"http:title": "Web Viewer"}})
	if weak.Type != "camera" {
		t.Errorf("Type = %q, want camera (reason=%q)", weak.Type, weak.Reason)
	}
	strong := recog(Device{Extra: map[string]string{"http:title": "Web Viewer"}, OpenPorts: []int{554}})
	if strong.Type != "camera" {
		t.Errorf("Type = %q, want camera (reason=%q)", strong.Type, strong.Reason)
	}
	if strong.Confidence <= weak.Confidence {
		t.Errorf("Confidence con 554 = %d, want > %d (sin 554)", strong.Confidence, weak.Confidence)
	}
}

func TestRecognizeHTTPTitleOctoprint(t *testing.T) {
	got := recog(Device{Extra: map[string]string{"http:title": "OctoPrint instance on octopi"}})
	if got.Type != "computer" || got.OS != "Linux" {
		t.Errorf("got %+v (reason=%q)", got, got.Reason)
	}
}

func TestRecognizeHTTPTitleSynologyNAS(t *testing.T) {
	got := recog(Device{Extra: map[string]string{"http:title": "Synology DiskStation"}})
	if got.Type != "nas" {
		t.Errorf("Type = %q, want nas (reason=%q)", got.Type, got.Reason)
	}
}

// TestRecognizePortsGroupedInReason: 139 y 445 comparten regla (ver item 5 de
// la tarea) y deben agruparse en un solo "puertos 139, 445", no listarse por
// separado ni perderse el segundo.
func TestRecognizePortsGroupedInReason(t *testing.T) {
	got := recog(Device{Hostname: "DESKTOP-ABC", OpenPorts: []int{139, 445}})
	if !strings.Contains(got.Reason, "puertos 139, 445") {
		t.Errorf("Reason = %q, want que incluya %q", got.Reason, "puertos 139, 445")
	}
}

// TestRecognizeRedactsEmailInReason: el friendlyName SSDP de un equipo
// Windows puede traer la cuenta de usuario (ver PC .55 en AGENTS.md) —
// Reason nunca debe mostrar un correo en texto plano.
func TestRecognizeRedactsEmailInReason(t *testing.T) {
	got := recog(Device{Extra: map[string]string{
		"ssdp:friendlyName": "DESKTOP-ABC1234: usuario@example.com:",
		"ssdp:manufacturer": "Microsoft Corporation",
		"ssdp:deviceType":   "urn:schemas-upnp-org:device:MediaServer:1",
	}})
	if strings.Contains(got.Reason, "@") {
		t.Errorf("Reason = %q: se coló un correo sin redactar", got.Reason)
	}
	if !strings.Contains(got.Reason, "[correo]") {
		t.Errorf("Reason = %q, want que incluya el marcador %q", got.Reason, "[correo]")
	}
}

// TestRecognizeSSDPFriendlyNameNotRepeatedWhenJustHostname: si el
// friendlyName SSDP es solo el hostname, no se repite en Reason (item 5).
func TestRecognizeSSDPFriendlyNameNotRepeatedWhenJustHostname(t *testing.T) {
	got := recog(Device{
		Hostname: "router-sala",
		Extra: map[string]string{
			"ssdp:friendlyName": "router-sala",
			"ssdp:deviceType":   "urn:schemas-upnp-org:device:InternetGatewayDevice:1",
			"ssdp:manufacturer": "Tenda",
		},
	})
	if strings.Contains(got.Reason, "nombre=") {
		t.Errorf("Reason = %q: no debía repetir el friendlyName, es solo el hostname", got.Reason)
	}
}

// TestRecognizeMDNSManufacturerBrandFallback: TXT "manufacturer=" de mDNS
// (p. ej. _airplay de una TV Samsung) da la marca aunque no haya SSDP.
func TestRecognizeMDNSManufacturerBrandFallback(t *testing.T) {
	got := recog(Device{
		Services: []Service{{Proto: "mdns", Name: "_airplay._tcp"}},
		Extra:    map[string]string{"mdns:manufacturer": "Samsung", "mdns:model": "QCQ70D"},
	})
	if got.Brand != "Samsung" {
		t.Errorf("Brand = %q, want %q (reason=%q)", got.Brand, "Samsung", got.Reason)
	}
	if got.Model != "QCQ70D" {
		t.Errorf("Model = %q, want %q", got.Model, "QCQ70D")
	}
}

// TestRecognizeNewMDNSServiceRules cubre los tipos de servicio añadidos
// (ver AGENTS.md, item 4): algunos vistos en la red real (_rfb, _teamviewer),
// otros de uso común que faltaban (_sonos, _hue, _octoprint).
func TestRecognizeNewMDNSServiceRules(t *testing.T) {
	cases := []struct {
		name      string
		svc       string
		wantType  string
		wantBrand string
		minConf   int
	}{
		{"rfb (Screen Sharing)", "_rfb._tcp", "computer", "", 45},
		{"teamviewer", "_teamviewer._tcp", "computer", "", 55},
		{"octoprint", "_octoprint._tcp", "computer", "", 75},
		{"sonos", "_sonos._tcp", "speaker", "Sonos", 85},
		{"hue", "_hue._tcp", "iot", "Philips Hue", 85},
		{"amzn-wplay (Fire TV)", "_amzn-wplay._tcp", "tv", "Amazon", 70},
		{"androidtvremote2", "_androidtvremote2._tcp", "tv", "", 80},
		{"home-assistant", "_home-assistant._tcp", "computer", "", 80},
		{"esphomelib", "_esphomelib._tcp", "iot", "", 80},
		{"workstation", "_workstation._tcp", "computer", "", 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := recog(Device{Services: []Service{{Proto: "mdns", Name: tc.svc}}})
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q (reason=%q)", got.Type, tc.wantType, got.Reason)
			}
			if tc.wantBrand != "" && got.Brand != tc.wantBrand {
				t.Errorf("Brand = %q, want %q", got.Brand, tc.wantBrand)
			}
			if got.Confidence < tc.minConf {
				t.Errorf("Confidence = %d, want >= %d", got.Confidence, tc.minConf)
			}
		})
	}
}

// TestRecognizeOculusAirLinkIsWindowsOnly: _oculusal_sp/_oculusal_sp_v2 (Meta
// Quest Air Link) solo corre en Windows — ver AGENTS.md.
func TestRecognizeOculusAirLinkIsWindowsOnly(t *testing.T) {
	for _, svc := range []string{"_oculusal_sp._tcp", "_oculusal_sp_v2._tcp"} {
		got := recog(Device{Services: []Service{{Proto: "mdns", Name: svc}}})
		if got.Type != "computer" || got.OS != "Windows" {
			t.Errorf("%s: Type/OS = %q/%q, want computer/Windows", svc, got.Type, got.OS)
		}
	}
}

func TestCleanBrand(t *testing.T) {
	cases := map[string]string{
		"Samsung Electronics Co.,Ltd":                    "Samsung",
		"Tenda Technology Co.,Ltd.Dongguan branch":       "Tenda",
		"Intel Corporate":                                "Intel",
		"Giga-byte Technology Co., Ltd.":                 "Gigabyte",
		"Amazon Technologies Inc.":                       "Amazon",
		"AltoBeam (Beijing) Inc.":                        "AltoBeam",
		"Apple, Inc.":                                    "Apple",
		"Microsoft Corporation":                          "Microsoft",
		"Ring LLC":                                       "Ring",
		"Synology Incorporated":                          "Synology",
		"Qnap Systems, Inc.":                             "QNAP",
		"Ubiquiti Networks Inc.":                         "Ubiquiti",
		"Hangzhou Hikvision Digital Technology Co.,Ltd.": "Hikvision",
		"Shenzhen Intellirocks Tech. Co., Ltd.":          "Govee",
		"Beijing Xiaomi Mobile Software Co., Ltd":        "Xiaomi",
		"TP-LINK TECHNOLOGIES CO.,LTD.":                  "TP-Link",
		"Tenda":                                          "Tenda",
		"Inc.":                                           "Inc.",
	}
	for in, want := range cases {
		if got := cleanBrand(in); got != want {
			t.Errorf("cleanBrand(%q) = %q, want %q", in, got, want)
		}
	}
}
