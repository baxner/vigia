package core

import (
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// recognize deduce qué es un dispositivo a partir de TODA la evidencia reunida
// en d (Vendor por OUI, Hostname, Services de mDNS/SSDP/NetBIOS, OpenPorts y
// Extra: campos SSDP por separado, mDNS model/os/manufacturer, API HTTP del
// aparato, título/Server HTTP, marca de equipo propio) y devuelve
// Identity{Type, Brand, Model, OS, Confidence 0-100, Reason}.
//
// Capas de evidencia, de mayor a menor confianza (tablas en recognize_rules.go):
//
//  0. API HTTP propia del aparato (Samsung/Roku/Chromecast) — directa, 90-95.
//  1. SSDP (deviceType/modelName/friendlyName/manufacturer) — directa, 90.
//  2. Modelo mDNS (TXT "model"/"md"/"am"/"rpMd") — directa, 90 si calza una
//     regla; osxvers (SO de una Mac) y manufacturer (marca) van aparte, sin
//     fijar Type, solo afinan OS/Brand.
//  3. Servicios mDNS anunciados — 35-85.
//     3.5. Título HTTP (<title> de un GET "/") — 45-75.
//  4. Puertos TCP abiertos — 50-70.
//  5. Equipo propio (el que escanea) — SO cierto, 60-95.
//  6. Hostname por regex — heurística débil, 35-42.
//  7. Fabricante OUI — la más débil.
//
// Gana el tipo con más puntaje (su señal más fuerte + 5 por cada señal extra
// que coincide, tope 99; ver identity). Marca: API del aparato > SSDP
// manufacturer > marca de un modelo mDNS reconocido > TXT manufacturer mDNS >
// fabricante OUI > marca implícita de un protocolo (una TV Samsung con
// _airplay es Samsung, no Apple). Reason lista la evidencia usada —con
// correos redactados y puertos de una misma regla agrupados— y es la
// transparencia que diferencia a Vigía de Fing.
func recognize(d *Device) Identity {
	var ev evidence
	apple := appleEvidence(d)
	windows := windowsEvidence(d)

	// 0) API de aparato por HTTP (core/httpfp.go): confirma modelo/SO exactos
	// contra la API propia del fabricante, más confiable que SSDP/mDNS — por
	// eso su confianza va por encima de la de ambos (90-95 contra 90).
	apiBrand, apiModel := "", ""
	if t := strings.TrimSpace(d.Extra["api:type"]); t != "" {
		conf := 90
		switch {
		case hasOpenPort(d.OpenPorts, 8001):
			apiBrand, conf = "Samsung", 95
		case hasOpenPort(d.OpenPorts, 8060):
			apiBrand, conf = "Roku", 95
		case hasOpenPort(d.OpenPorts, 8008):
			apiBrand = "Google"
		}
		apiModel = strings.TrimSpace(d.Extra["api:model"])
		apiOS := strings.TrimSpace(d.Extra["api:os"])
		reason := "api " + t
		if apiBrand != "" {
			reason += " " + apiBrand
		}
		if apiModel != "" {
			reason += " modelo=" + apiModel
		}
		if apiOS != "" {
			reason += " os=" + apiOS
		}
		ev.add(rule{Type: t, Brand: apiBrand, OS: apiOS, Conf: conf}, reason)
	}

	// 1) SSDP
	ssdp := ssdpFieldsOf(d)
	model := apiModel
	if ssdp.present {
		// El modelo y el nombre describen al aparato concreto; el deviceType es
		// genérico (un PC con Windows Media Player también es "MediaRenderer").
		r := rule{Conf: 30} // sin tipo: sabemos marca/modelo, no qué aparato es
		for _, s := range []string{ssdp.modelName, ssdp.friendlyName, ssdp.deviceType} {
			if t, os := ssdpTypeHint(s); t != "" {
				r = rule{Type: t, OS: os, Conf: 90}
				break
			}
		}
		if r.Type == "" && ssdp.manufacturer != "" {
			if mr, ok := matchSSDPManufacturer(ssdp.manufacturer); ok {
				r = mr // un fabricante solo sugiere el tipo: su propia confianza, no 90
			}
		}
		if model == "" {
			model = ssdpModel(ssdp) // la API (si la hay) manda sobre el modelName SSDP
		}
		ev.add(r, ssdp.reason(model, d.Hostname))
	}

	// 2) Modelo mDNS: evidencia directa si calza una regla (Apple/Google).
	modelBrand := ""
	if m := strings.TrimSpace(d.Extra["mdns:model"]); m != "" {
		if r, ok := matchMDNSModel(m); ok {
			ev.add(r, "mdns model="+m)
			modelBrand = r.Brand
		}
		if model == "" {
			// Identificador Apple ("iMac18,3") → nombre comercial si lo conocemos.
			model = firstNonEmpty(AppleModelName(m), m)
		}
	}
	// 2b) SO por osxvers: mismo TXT _device-info que el modelo (ver
	// discover_extra.go), pero Darwin no dice el nombre comercial ("24"), así
	// que se traduce con osxVersionNames. Conf por encima del modelo mDNS (90)
	// para que "macOS 15 Sequoia" le gane al "macOS" genérico de
	// mdnsModelRules; Type vacío porque esto solo afina el SO, el modelo ya
	// dijo que es una Mac.
	if v := strings.TrimSpace(d.Extra["mdns:os"]); v != "" {
		if label, ok := osxVersionNames[v]; ok {
			ev.add(rule{OS: label, Conf: 91}, "osxvers="+v)
		}
	}
	// 2c) Marca por TXT manufacturer/integrator (p. ej. una TV Samsung
	// anunciando _airplay trae "manufacturer=Samsung" sin SSDP de por medio).
	// No fija Type: solo deja la marca disponible para el "brand :=" de abajo,
	// con su propio renglón en Reason para que quede claro de dónde salió.
	mdnsManufacturer := strings.TrimSpace(d.Extra["mdns:manufacturer"])
	if mdnsManufacturer != "" {
		ev.add(rule{Brand: cleanBrand(mdnsManufacturer)}, "mdns manufacturer="+mdnsManufacturer)
	}

	// 3) Servicios mDNS.
	seen := map[string]bool{}
	for _, s := range d.Services {
		if s.Proto != "mdns" {
			continue
		}
		name := strings.ToLower(s.Name)
		r, ok := mdnsServiceRules[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		switch name {
		case "_airplay._tcp", "_raop._tcp":
			if apple {
				r.Type = "" // en un Apple es el Receptor AirPlay, no una TV/altavoz
			}
		case "_smb._tcp", "_adisk._tcp":
			if apple || windows {
				r.Type = "" // un ordenador compartiendo archivos no es un NAS
			}
		}
		ev.add(r, "mdns "+s.Name)
	}

	// 3b) Anuncio Tuya (core/tuya.go): el broadcast UDP lleva la firma del
	// protocolo, así que es evidencia directa de un aparato Tuya/Smart Life.
	for _, s := range d.Services {
		if s.Proto == "tuya" {
			ev.add(rule{Type: "iot", Brand: "Tuya", Conf: 85}, "anuncio Tuya "+s.Info)
			break
		}
	}

	// 3.5) Título HTTP (core/httpfp.go, Extra "http:title"): palabra completa,
	// igual criterio que SSDP/vendor. "Web Viewer" es débil a propósito — sube
	// solo si además hay evidencia de cámara real (puerto 554), por el
	// mecanismo normal de combinación de la capa 4, no con un caso especial.
	if title := strings.TrimSpace(d.Extra["http:title"]); title != "" {
		if r, ok := httpTitleHint(title); ok {
			ev.add(r, "http title="+title)
		}
	}

	// 4) Puertos abiertos: se agrupan los que comparten regla ("puertos 139,
	// 445") en vez de solo mencionar el primero.
	samba := seen["_smb._tcp"] || seen["_adisk._tcp"] // Windows no anuncia SMB por mDNS
	var portOrder []rule
	portsByRule := map[rule][]int{}
	for _, p := range d.OpenPorts {
		r, ok := portRules[p]
		if !ok {
			continue
		}
		if (p == 445 || p == 139) && (apple || samba) {
			continue // SMB de un Mac o de Samba (NAS/Linux), no de Windows
		}
		if _, dup := portsByRule[r]; !dup {
			portOrder = append(portOrder, r)
		}
		portsByRule[r] = append(portsByRule[r], p)
	}
	for _, r := range portOrder {
		ps := portsByRule[r]
		label := "puerto"
		if len(ps) > 1 {
			label = "puertos"
		}
		nums := make([]string, len(ps))
		for i, p := range ps {
			nums[i] = itoa(p)
		}
		ev.add(r, label+" "+strings.Join(nums, ", "))
	}

	// 5) Equipo propio.
	if d.Extra["self"] == "true" {
		r := selfRule()
		if _, ok := osxVersionNames[d.Extra["mdns:os"]]; ok {
			r.OS = "" // la versión exacta ("macOS 15 Sequoia") manda sobre "macOS"
		}
		if osName := d.Extra["self:os"]; osName != "" {
			r.OS = osName
			if r.Conf < 85 {
				r.Conf = 85
			}
		}
		if selfModel := strings.TrimSpace(d.Extra["self:model"]); selfModel != "" {
			if model == "" {
				model = selfModel
			}
			if r.Conf < 90 {
				r.Conf = 90
			}
		}
		ev.add(r, "equipo que escanea")
	}

	// 6) Hostname: la primera regla que calce.
	if d.Hostname != "" {
		for _, hr := range hostnameRules {
			if hr.Pattern.MatchString(d.Hostname) {
				ev.add(hr.R, "hostname="+d.Hostname)
				break
			}
		}
	}

	// 7) Fabricante OUI.
	if d.Vendor != "" {
		if vr, ok := matchVendor(d.Vendor); ok {
			ev.add(vr, "oui "+d.Vendor)
		}
	}

	brand := ""
	switch {
	case apiBrand != "":
		brand = apiBrand
	case ssdp.manufacturer != "" && !ssdpSoftwareRe.MatchString(ssdp.modelName):
		brand = cleanBrand(ssdp.manufacturer)
	case modelBrand != "":
		brand = modelBrand
	case mdnsManufacturer != "":
		brand = cleanBrand(mdnsManufacturer)
	case d.Vendor != "":
		brand = cleanBrand(d.Vendor)
	}
	return ev.identity(brand, model, d.Vendor != "")
}

// ---------------------------------------------------------------------------
// Acumulación y combinación de señales
// ---------------------------------------------------------------------------

type evidence struct {
	signals []rule
	reasons []string
}

func (e *evidence) add(r rule, reason string) {
	if r.Type == "" && r.Brand == "" && r.OS == "" && r.Conf == 0 {
		return
	}
	e.signals = append(e.signals, r)
	e.reasons = append(e.reasons, reason)
}

// emailRe: una dirección de correo en cualquier parte del texto (p. ej. el
// friendlyName SSDP de un PC: "DESKTOP-ABC1234: usuario@dominio.com:").
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// redactEmails tapa cualquier correo que se haya colado en el Reason: viene
// de aparatos que exponen su cuenta en el nombre del equipo (OneDrive,
// Windows Media Player Sharing...), y Reason es texto pensado para mostrarse.
func redactEmails(s string) string {
	return emailRe.ReplaceAllString(s, "[correo]")
}

// identity combina las señales. Cada tipo puntúa su señal más fuerte + 5 por
// cada señal adicional que coincide (tope 99), y gana el tipo con más puntaje
// (a igualdad, el de la capa anterior): varias pistas débiles y coherentes
// (445 + hostname DESKTOP- + OUI Giga-byte → computer) le ganan a una pista
// suelta (554 → camera). OS y marca de respaldo salen solo de señales sin tipo
// o del tipo ganador, para no ponerle "Windows" a una TV por un puerto 445.
func (e *evidence) identity(brand, model string, hasVendor bool) Identity {
	id := Identity{Type: "unknown", Model: model, Brand: brand}

	type score struct{ best, n int }
	scores := map[string]*score{}
	var order []string
	maxConf := 0
	for _, s := range e.signals {
		maxConf = max(maxConf, s.Conf)
		if s.Type == "" {
			continue
		}
		sc := scores[s.Type]
		if sc == nil {
			sc = &score{}
			scores[s.Type] = sc
			order = append(order, s.Type)
		}
		sc.best = max(sc.best, s.Conf)
		sc.n++
	}
	best := 0
	for _, t := range order {
		if v := scores[t].best + 5*(scores[t].n-1); v > best {
			best, id.Type = v, t
		}
	}

	osConf := -1
	for _, s := range e.signals {
		if s.Type != "" && s.Type != id.Type {
			continue
		}
		if s.OS != "" && s.Conf > osConf {
			osConf, id.OS = s.Conf, s.OS
		}
		if id.Brand == "" && s.Brand != "" {
			id.Brand = s.Brand
		}
	}

	switch {
	case id.Type != "unknown":
		id.Confidence = min(best, 99)
	case len(e.signals) > 0:
		// Sabemos algo (marca, SO) pero no qué aparato es.
		id.Confidence = min(maxConf, 30)
	case hasVendor:
		id.Confidence = 10
	}
	id.Reason = redactEmails(strings.Join(e.reasons, "; "))
	return id
}

// ---------------------------------------------------------------------------
// SSDP
// ---------------------------------------------------------------------------

type ssdpFields struct {
	present                              bool
	modelName, modelNumber, manufacturer string
	deviceType, friendlyName             string
}

// ssdpFieldsOf lee los campos SSDP de Extra (los guarda scan.go por separado).
// Si no están —datos de una versión anterior— cae al Service "ssdp".
func ssdpFieldsOf(d *Device) ssdpFields {
	f := ssdpFields{
		modelName:    d.Extra["ssdp:modelName"],
		modelNumber:  d.Extra["ssdp:modelNumber"],
		manufacturer: d.Extra["ssdp:manufacturer"],
		deviceType:   d.Extra["ssdp:deviceType"],
		friendlyName: d.Extra["ssdp:friendlyName"],
	}
	f.present = f.modelName != "" || f.manufacturer != "" || f.deviceType != "" || f.friendlyName != ""
	if f.present || d.Extra["ssdp:server"] != "" {
		// Formato actual. Si solo hay cabecera SERVER (no bajó la descripción
		// XML), no hay nada que reconocer: "Linux/5.4 UPnP/1.0" no es un modelo.
		return f
	}
	for _, s := range d.Services {
		if s.Proto == "ssdp" {
			f.present = true
			f.modelName, f.manufacturer = s.Name, s.Info
			break
		}
	}
	return f
}

// reason arma el renglón de Reason para la capa SSDP. hostname es el
// d.Hostname del dispositivo: si el friendlyName es solo eso (p. ej. un
// router cuyo friendlyName repite tal cual su nombre mDNS/NetBIOS) no se
// repite — ya se ve aparte si la capa 6 (hostname) también disparó.
func (f ssdpFields) reason(model, hostname string) string {
	parts := []string{}
	if model != "" {
		parts = append(parts, "modelo="+model)
	}
	if f.deviceType != "" {
		parts = append(parts, "deviceType="+shortDeviceType(f.deviceType))
	}
	if f.friendlyName != "" && !strings.EqualFold(f.friendlyName, model) && !strings.EqualFold(f.friendlyName, hostname) {
		parts = append(parts, "nombre="+f.friendlyName)
	}
	if f.manufacturer != "" {
		parts = append(parts, "fabricante="+f.manufacturer)
	}
	if len(parts) == 0 {
		return "ssdp"
	}
	return "ssdp " + strings.Join(parts, ", ")
}

// shortDeviceType: "urn:schemas-upnp-org:device:MediaRenderer:1" → "MediaRenderer".
func shortDeviceType(dt string) string {
	parts := strings.Split(dt, ":")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return dt
}

var (
	// Un modelName "descriptivo" (palabras genéricas) no es un modelo.
	genericModelRe = regexp.MustCompile(`(?i)\b(wi-?fi|router|access point|media player|mesh|gateway|wps)\b`)
	hasLetterRe    = regexp.MustCompile(`[A-Za-z]`)
	hasDigitRe     = regexp.MustCompile(`[0-9]`)
)

// ssdpModel elige el modelo: modelName, salvo que sea genérico (igual al
// friendlyName o palabras descriptivas como "Mesh Wi-Fi"); entonces usa
// modelNumber si parece un modelo de verdad ("Mesh12X", no "12.0"). Si no hay
// nada fiable devuelve "": mejor sin modelo que con basura.
func ssdpModel(f ssdpFields) string {
	m := strings.TrimSpace(f.modelName)
	generic := m == "" || strings.EqualFold(m, strings.TrimSpace(f.friendlyName)) || genericModelRe.MatchString(m)
	if !generic {
		return m
	}
	n := strings.TrimSpace(f.modelNumber)
	if n != "" && !strings.Contains(n, " ") && hasLetterRe.MatchString(n) && hasDigitRe.MatchString(n) {
		return n
	}
	return ""
}

// ssdpTypeHint busca un tipo en un deviceType/modelName/friendlyName, por
// palabra completa ("CCTV" no es "tv").
func ssdpTypeHint(s string) (string, string) {
	if s == "" {
		return "", ""
	}
	for i, h := range ssdpTypeHints {
		if ssdpTypeHintRe[i].MatchString(s) {
			return h.Type, h.OS
		}
	}
	return "", ""
}

// httpTitleHint busca un tipo en el <title> HTTP (core/httpfp.go), por
// palabra completa (ver httpTitleHints en recognize_rules.go).
func httpTitleHint(title string) (rule, bool) {
	if title == "" {
		return rule{}, false
	}
	for i, h := range httpTitleHints {
		if httpTitleHintRe[i].MatchString(title) {
			return h.R, true
		}
	}
	return rule{}, false
}

// ssdpManufacturerKeys: claves en orden fijo (un map se recorre en orden
// aleatorio y el resultado debe ser determinista).
var ssdpManufacturerKeys = func() []string {
	k := make([]string, 0, len(ssdpManufacturerRules))
	for key := range ssdpManufacturerRules {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}()

func matchSSDPManufacturer(s string) (rule, bool) {
	for _, k := range ssdpManufacturerKeys {
		if ssdpManufacturerRe[k].MatchString(s) {
			return ssdpManufacturerRules[k], true
		}
	}
	return rule{}, false
}

// ---------------------------------------------------------------------------
// mDNS, fabricante, Apple/Windows, equipo propio
// ---------------------------------------------------------------------------

func matchMDNSModel(m string) (rule, bool) {
	for _, mr := range mdnsModelRules {
		if mr.Pattern.MatchString(m) {
			return mr.R, true
		}
	}
	return rule{}, false
}

// matchVendor: primera regla de fabricante que calce como palabra completa.
func matchVendor(v string) (rule, bool) {
	for i, vr := range vendorRules {
		if vendorRuleRe[i].MatchString(v) {
			return vr.R, true
		}
	}
	return rule{}, false
}

var appleWordRe = regexp.MustCompile(`(?i)\bapple\b`)

// appleEvidence: hay indicios fuertes de que es un aparato de Apple.
func appleEvidence(d *Device) bool {
	if appleWordRe.MatchString(d.Vendor) {
		return true
	}
	if r, ok := matchMDNSModel(d.Extra["mdns:model"]); ok && r.Brand == "Apple" {
		return true
	}
	for _, s := range d.Services {
		if s.Proto == "mdns" && strings.EqualFold(s.Name, "_companion-link._tcp") {
			return true
		}
	}
	return false
}

var windowsHostRe = regexp.MustCompile(`(?i)^(desktop|laptop)-`)

// windowsEvidence: indicios de un PC con Windows.
func windowsEvidence(d *Device) bool {
	if windowsHostRe.MatchString(d.Hostname) {
		return true
	}
	for _, s := range d.Services {
		// NetBIOS solo no basta: Samba (NAS, Linux, Mac) también responde.
		if s.Proto == "mdns" && strings.EqualFold(s.Name, "_dosvc._tcp") {
			return true
		}
	}
	return false
}

// selfRule: del equipo que escanea sabemos el SO con certeza (es donde corre
// el motor): macOS solo corre en un Mac. En Android puede ser teléfono o
// tablet, de ahí la confianza menor.
func selfRule() rule {
	switch runtime.GOOS {
	case "android":
		return rule{Type: "phone", OS: "Android", Conf: 80}
	case "ios":
		return rule{Type: "phone", Brand: "Apple", OS: "iOS", Conf: 85}
	case "darwin":
		return rule{Type: "computer", Brand: "Apple", OS: "macOS", Conf: 95}
	case "windows":
		return rule{Type: "computer", OS: "Windows", Conf: 95}
	case "linux":
		return rule{Type: "computer", OS: "Linux", Conf: 60}
	}
	return rule{Type: "computer", Conf: 50}
}

// ---------------------------------------------------------------------------
// Marca legible
// ---------------------------------------------------------------------------

var parenRe = regexp.MustCompile(`\s*\([^)]*\)`)

var brandSuffixRe = regexp.MustCompile(`(?i)[\s,]+(co\.?|ltd\.?|inc\.?|corp\.?|incorporated|corporation|corporate|technologies|technology|electronics|systems|networks|limited|llc|gmbh|ag)$`)

// brandAliases: razones sociales que no se parecen a la marca que conoce la
// gente ("Shenzhen Intellirocks Tech." es Govee) o que la esconden tras la
// ciudad y el tipo de sociedad ("Hangzhou Hikvision Digital Technology").
// Palabra completa, como el resto de las reglas.
var brandAliases = []struct {
	re    *regexp.Regexp
	brand string
}{
	{compileWordBoundary("hikvision"), "Hikvision"},
	{compileWordBoundary("dahua"), "Dahua"},
	{compileWordBoundary("xiaomi"), "Xiaomi"},
	{compileWordBoundary("huawei"), "Huawei"},
	{compileWordBoundary("intellirocks"), "Govee"},
	{compileWordBoundary("gaoshengda"), "Gaoshengda"},
	{compileWordBoundary("espressif"), "Espressif"},
	{compileWordBoundary("tuya"), "Tuya"},
	{compileWordBoundary("tp-link"), "TP-Link"},
	{compileWordBoundary("tenda"), "Tenda"},
	{compileWordBoundary("amazon"), "Amazon"},
	{compileWordBoundary("google"), "Google"},
	{compileWordBoundary("samsung"), "Samsung"},
	{compileWordBoundary("apple"), "Apple"},
	{compileWordBoundary("lg electronics"), "LG"},
	{compileWordBoundary("hon hai"), "Foxconn"},
	{compileWordBoundary("giga-byte"), "Gigabyte"},
	{compileWordBoundary("micro-star"), "MSI"},
	{compileWordBoundary("asustek"), "ASUS"},
	{compileWordBoundary("raspberry pi"), "Raspberry Pi"},
	{compileWordBoundary("qnap"), "QNAP"},
}

// cleanBrand: "Samsung Electronics Co.,Ltd" → "Samsung", "Tenda
// Technology,Ltd.Dongguan branch" → "Tenda", "Intel Corporate" → "Intel",
// "AltoBeam (Beijing) Inc." → "AltoBeam".
func cleanBrand(s string) string {
	for _, a := range brandAliases {
		if a.re.MatchString(s) {
			return a.brand
		}
	}
	orig := strings.TrimSpace(s)
	out := orig
	if i := strings.Index(out, ","); i > 0 {
		out = out[:i]
	}
	out = strings.TrimSpace(parenRe.ReplaceAllString(out, " "))
	for {
		t := strings.TrimSpace(brandSuffixRe.ReplaceAllString(out, ""))
		if t == out || t == "" {
			break
		}
		out = t
	}
	if out == "" {
		return orig
	}
	return out
}
