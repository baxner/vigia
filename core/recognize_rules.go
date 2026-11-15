package core

import "regexp"

// ===========================================================================
// Tablas de reconocimiento (solo datos). El motor que las recorre vive en
// recognize.go.
//
// REGLA DE ORO (post-auditoría contra core/data/oui.txt, 49k+ fabricantes):
// toda clave que se busca "contenida" en un string de fabricante/servidor/
// deviceType se compara como PALABRA COMPLETA, nunca como subcadena cruda.
// "ring" como subcadena calza con 501 fabricantes distintos (cualquier cosa
// que termine en "-ring": Engineering, Manufacturing, Monitoring...);
// "intel" como subcadena calza con "Qingdao Intelligent" y "Shenzhen
// Intellirocks"; "asus" calza con "Pegasus Technologies". Por eso cada clave
// se compara con una regex `(?i)\b<clave>\b` precompilada — ver
// compileWordBoundary más abajo y matchVendor/matchSSDPManufacturer/
// ssdpTypeHint en recognize.go.
// ===========================================================================

// rule es la conclusión parcial que aporta UNA capa de evidencia. Los campos
// vacíos significan "esta capa no opina sobre eso"; Conf es la confianza que
// tendría esa capa si fuera la única evidencia disponible.
type rule struct {
	Type  string
	Brand string
	OS    string
	Conf  int
}

// compileWordBoundary precompila una regex case-insensitive que solo calza
// `key` como palabra completa (regexp.QuoteMeta por si la clave trae "-" u
// otro carácter con significado especial).
func compileWordBoundary(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `\b`)
}

// ssdpManufacturerRules: clave buscada como palabra completa dentro del
// <manufacturer> SSDP. El Brand final no sale de aquí (sale del manufacturer
// crudo o del OUI); esta tabla solo opina del Type/OS cuando el
// deviceType/friendlyName no alcanzan.
var ssdpManufacturerRules = map[string]rule{
	"samsung":        {Type: "tv", Conf: 55},
	"lg electronics": {Type: "tv", Conf: 55},
	"roku":           {Type: "tv", Conf: 55},
	"sonos":          {Type: "speaker", Conf: 60},
	"tenda":          {Type: "router", Conf: 60},
	"tp-link":        {Type: "router", Conf: 60},
	"netgear":        {Type: "router", Conf: 60},
	"ubiquiti":       {Type: "router", Conf: 55},
	"mikrotik":       {Type: "router", Conf: 55},
	"d-link":         {Type: "router", Conf: 55},
	"microsoft":      {Type: "computer", OS: "Windows", Conf: 60},
	"plex":           {Type: "nas", Conf: 55},
}

// ssdpManufacturerRe: regex por palabra completa, precompiladas una vez,
// alineadas por clave con ssdpManufacturerRules.
var ssdpManufacturerRe = func() map[string]*regexp.Regexp {
	re := make(map[string]*regexp.Regexp, len(ssdpManufacturerRules))
	for k := range ssdpManufacturerRules {
		re[k] = compileWordBoundary(k)
	}
	return re
}()

// ssdpTypeHints: subcadenas de modelName/deviceType/friendlyName SSDP que
// delatan el tipo de aparato, independientes del fabricante — comparadas como
// palabra completa (un DVR "CCTV" o un "ATV" NO deben calzar con "tv"). Se
// revisan en orden; las más específicas van primero.
var ssdpTypeHints = []struct {
	Contains string
	Type     string
	OS       string
}{
	{"mediarenderer", "tv", ""},
	{"internetgatewaydevice", "router", ""},
	{"access point", "router", ""},
	{"mesh", "router", ""},
	{"windows media player", "computer", "Windows"},
	{"router", "router", ""},
	{"tv", "tv", ""},
}

// ssdpSoftwareRe: modelName de un servidor UPnP por software. Su
// <manufacturer> es el autor del programa (Microsoft, Plex), no la marca del
// aparato: un PC que comparte con Windows Media Player no es "Microsoft".
var ssdpSoftwareRe = regexp.MustCompile(`(?i)\b(windows media player|plex|kodi|serviio|minidlna|twonky|jellyfin|emby|universal media server)\b`)

// ssdpTypeHintRe: regex por palabra completa, precompiladas, alineadas por
// índice con ssdpTypeHints.
var ssdpTypeHintRe = func() []*regexp.Regexp {
	re := make([]*regexp.Regexp, len(ssdpTypeHints))
	for i, h := range ssdpTypeHints {
		re[i] = compileWordBoundary(h.Contains)
	}
	return re
}()

// httpTitleHints: subcadenas del <title> HTTP (core/httpfp.go, Extra
// "http:title") que delatan el tipo de aparato — mismo criterio de palabra
// completa que ssdpTypeHints/vendorRules, más específicas primero. "Web
// Viewer" es deliberadamente débil (lo usan cámaras IP genéricas sin marca):
// sube si además hay 554 (RTSP) abierto, por el mecanismo normal de
// combinación de evidencia en identity() — no hace falta un caso especial.
var httpTitleHints = []struct {
	Contains string
	R        rule
}{
	{"hikvision", rule{Type: "camera", Conf: 75}},
	{"dahua", rule{Type: "camera", Conf: 75}},
	{"netsurveillance", rule{Type: "camera", Conf: 70}},
	{"ipcam", rule{Type: "camera", Conf: 65}},
	{"nvr", rule{Type: "camera", Conf: 55}},
	{"dvr", rule{Type: "camera", Conf: 55}},
	{"web viewer", rule{Type: "camera", Conf: 45}},
	{"octoprint", rule{Type: "computer", OS: "Linux", Conf: 75}},
	{"diskstation", rule{Type: "nas", Conf: 75}},
	{"synology", rule{Type: "nas", Conf: 75}},
	{"qnap", rule{Type: "nas", Conf: 75}},
	{"archer", rule{Type: "router", Conf: 55}},
	{"deco", rule{Type: "router", Conf: 55}},
	{"tp-link", rule{Type: "router", Conf: 55}},
	{"tenda", rule{Type: "router", Conf: 55}},
	{"home assistant", rule{Type: "computer", OS: "Linux", Conf: 70}},
	{"pi-hole", rule{Type: "computer", OS: "Linux", Conf: 70}},
	{"frigate", rule{Type: "computer", OS: "Linux", Conf: 70}},
	{"laserjet", rule{Type: "printer", Conf: 70}},
	{"epson", rule{Type: "printer", Conf: 70}},
	{"brother", rule{Type: "printer", Conf: 70}},
}

// httpTitleHintRe: regex por palabra completa, precompiladas, alineadas por
// índice con httpTitleHints.
var httpTitleHintRe = func() []*regexp.Regexp {
	re := make([]*regexp.Regexp, len(httpTitleHints))
	for i, h := range httpTitleHints {
		re[i] = compileWordBoundary(h.Contains)
	}
	return re
}()

// osxVersionNames: versión mayor de Darwin (TXT "osxvers" de _device-info,
// crudo, p. ej. "24") → nombre comercial de macOS. Tabla mínima: solo las
// versiones confirmadas, no se adivinan números sin verificar.
var osxVersionNames = map[string]string{
	"25": "macOS 26 Tahoe",
	"24": "macOS 15 Sequoia",
	"23": "macOS 14 Sonoma",
	"22": "macOS 13 Ventura",
	"21": "macOS 12 Monterey",
	"20": "macOS 11 Big Sur",
	"19": "macOS 10.15 Catalina",
}

// mdnsServiceRules: tipo de servicio mDNS (ya sin ".local.", p. ej.
// "_googlecast._tcp") → pista. _airplay/_googlecast no fijan marca a propósito
// (los anuncia tanto un TV cualquiera como un Apple TV); además, en un Apple
// confirmado (ver appleEvidence en recognize.go) _airplay/_raop son el
// RECEPTOR y no deben fijar Type en absoluto, no solo marca. _smb/_adisk bajan
// a 40 y tampoco fijan Type si ya hay evidencia de computer (Apple/Windows):
// un Mac con compartir-archivos prendido no es un NAS.
var mdnsServiceRules = map[string]rule{
	"_googlecast._tcp":     {Type: "tv", Conf: 75},
	"_airplay._tcp":        {Type: "tv", Conf: 70},
	"_raop._tcp":           {Type: "speaker", Conf: 70},
	"_ipp._tcp":            {Type: "printer", Conf: 80},
	"_printer._tcp":        {Type: "printer", Conf: 80},
	"_hap._tcp":            {Type: "iot", Conf: 70},
	"_homekit._tcp":        {Type: "iot", Conf: 70},
	"_companion-link._tcp": {Brand: "Apple", Conf: 70},
	// _device-info sin marca: Samba/avahi (NAS, Linux) también lo publican,
	// con model=Xserve/RackMac por convención. La marca Apple sale de un
	// modelo Apple real (mdnsModelRules) o del OUI.
	"_spotify-connect._tcp": {Type: "speaker", Conf: 55},
	"_nearbypresence._tcp":  {Type: "phone", OS: "Android", Conf: 75},
	"_smb._tcp":             {Type: "nas", Conf: 40},
	"_adisk._tcp":           {Type: "nas", Conf: 40},
	"_matter._tcp":          {Type: "iot", Conf: 70},
	"_dosvc._tcp":           {Type: "computer", OS: "Windows", Conf: 75},
	// Añadidos para identificar sin MAC (Android 11+ sin ARP): tipos vistos en
	// la red real (.48 Screen Sharing, .55 TeamViewer/Quest Air Link) más los
	// de uso común que faltaban (Sonos, Hue, Home Assistant...).
	"_rfb._tcp":              {Type: "computer", Conf: 45}, // Screen Sharing/VNC
	"_teamviewer._tcp":       {Type: "computer", Conf: 55},
	"_oculusal_sp._tcp":      {Type: "computer", OS: "Windows", Conf: 70}, // Meta Quest Air Link, solo Windows
	"_oculusal_sp_v2._tcp":   {Type: "computer", OS: "Windows", Conf: 70},
	"_octoprint._tcp":        {Type: "computer", OS: "Linux", Conf: 75},
	"_sonos._tcp":            {Type: "speaker", Brand: "Sonos", Conf: 85},
	"_hue._tcp":              {Type: "iot", Brand: "Philips Hue", Conf: 85},
	"_amzn-wplay._tcp":       {Type: "tv", Brand: "Amazon", Conf: 70}, // Fire TV
	"_androidtvremote2._tcp": {Type: "tv", Conf: 80},
	"_home-assistant._tcp":   {Type: "computer", Conf: 80},
	"_esphomelib._tcp":       {Type: "iot", Conf: 80},
	"_ssh._tcp":              {Type: "computer", Conf: 35},
	"_workstation._tcp":      {Type: "computer", OS: "Linux", Conf: 50},
}

// mdnsModelRules: el TXT "model" de mDNS/Bonjour (Extra["mdns:model"]) es
// evidencia tan directa como un modelName SSDP: Apple y Google codifican el
// tipo de aparato en el propio string de modelo. Conf 90, igual que SSDP.
// Los patrones Apple van anclados al inicio (^) porque el TXT es del estilo
// "iPhone15,2"/"MacBookPro18,1"/"AppleTV6,2"; los de Google no, porque vienen
// como nombre comercial libre ("Google Nest Mini", "Chromecast Ultra").
var mdnsModelRules = []struct {
	Pattern *regexp.Regexp
	R       rule
}{
	{regexp.MustCompile(`(?i)^iphone`), rule{Type: "phone", Brand: "Apple", OS: "iOS", Conf: 90}},
	{regexp.MustCompile(`(?i)^ipad`), rule{Type: "tablet", Brand: "Apple", OS: "iPadOS", Conf: 90}},
	{regexp.MustCompile(`(?i)^(imac|macbook|mac ?mini|mac)`), rule{Type: "computer", Brand: "Apple", OS: "macOS", Conf: 90}},
	{regexp.MustCompile(`(?i)^appletv`), rule{Type: "tv", Brand: "Apple", OS: "tvOS", Conf: 90}},
	{regexp.MustCompile(`(?i)^audioaccessory`), rule{Type: "speaker", Brand: "Apple", Conf: 90}}, // HomePod
	{regexp.MustCompile(`(?i)^watch`), rule{Type: "wearable", Brand: "Apple", OS: "watchOS", Conf: 90}},
	{regexp.MustCompile(`(?i)chromecast|google tv`), rule{Type: "tv", Brand: "Google", Conf: 90}},
	{regexp.MustCompile(`(?i)google home|nest mini|nest audio`), rule{Type: "speaker", Brand: "Google", Conf: 90}},
	{regexp.MustCompile(`(?i)nest hub`), rule{Type: "tv", Brand: "Google", Conf: 90}},
}

// portRules: puerto TCP abierto → pista. Más débil que SSDP/mDNS porque un
// puerto confirma un servicio, no el aparato exacto (7000/3689/548 solo dan
// marca: son servicios Apple -AirPlay/DAAP/AFP- que corren en TVs, altavoces o
// Macs por igual, así que no arriesgamos Type).
var portRules = map[int]rule{
	554:   {Type: "camera", Conf: 60},
	9100:  {Type: "printer", Conf: 65},
	515:   {Type: "printer", Conf: 65},
	631:   {Type: "printer", Conf: 65},
	32400: {Type: "nas", Conf: 55},
	62078: {Type: "phone", OS: "iOS", Conf: 70},
	445:   {Type: "computer", OS: "Windows", Conf: 55},
	139:   {Type: "computer", OS: "Windows", Conf: 55},
	7000:  {Brand: "Apple", Conf: 50},
	3689:  {Brand: "Apple", Conf: 50},
	548:   {Brand: "Apple", Conf: 50},
	8001:  {Type: "tv", Brand: "Samsung", Conf: 65}, // API de control remoto de Tizen
	8060:  {Type: "tv", Brand: "Roku", Conf: 70},    // Roku ECP
	8008:  {Type: "tv", Brand: "Google", Conf: 60},
	8009:  {Type: "tv", Brand: "Google", Conf: 60},
}

// hostnameRules: regex sobre el hostname (case-insensitive). Heurística débil:
// confirma lo que ya sugirieron capas superiores, o aporta el único indicio
// cuando no hay nada más (p. ej. Android con MAC aleatoria).
//
// Notas de la auditoría:
//   - "switch" se sacó de la regla de Nintendo: un switch de red ("core-switch-01")
//     no es una consola.
//   - "ps[45]" ahora exige palabra completa (\b): "ups5"/"gps4" NO deben calzar.
//   - "echo" ahora exige que el hostname EMPIECE con "echo" (^): así no calza
//     dentro de "techo" o cualquier palabra que solo contenga "echo" en medio.
//     Como contrapartida, un hostname tipo "Living-Room-Echo" (Echo no al
//     inicio) ya no calza — trade-off explícito pedido en la revisión.
var hostnameRules = []struct {
	Pattern *regexp.Regexp
	R       rule
}{
	{regexp.MustCompile(`(?i)iphone`), rule{Type: "phone", OS: "iOS", Conf: 40}},
	{regexp.MustCompile(`(?i)ipad`), rule{Type: "tablet", OS: "iOS", Conf: 40}},
	{regexp.MustCompile(`(?i)galaxy`), rule{Type: "phone", Brand: "Samsung", OS: "Android", Conf: 42}},
	{regexp.MustCompile(`(?i)pixel`), rule{Type: "phone", Brand: "Google", OS: "Android", Conf: 42}},
	{regexp.MustCompile(`(?i)xiaomi|redmi`), rule{Type: "phone", Brand: "Xiaomi", OS: "Android", Conf: 42}},
	{regexp.MustCompile(`(?i)android`), rule{Type: "phone", OS: "Android", Conf: 35}},
	{regexp.MustCompile(`(?i)macbook|imac|mac-?mini`), rule{Type: "computer", Brand: "Apple", OS: "macOS", Conf: 40}},
	{regexp.MustCompile(`(?i)^desktop-|^laptop-`), rule{Type: "computer", OS: "Windows", Conf: 40}},
	{regexp.MustCompile(`(?i)^echo`), rule{Type: "speaker", Brand: "Amazon", Conf: 42}},
	{regexp.MustCompile(`(?i)chromecast`), rule{Type: "tv", Brand: "Google", Conf: 40}},
	{regexp.MustCompile(`(?i)nintendo`), rule{Type: "console", Brand: "Nintendo", Conf: 40}},
	{regexp.MustCompile(`(?i)playstation|\bps[45]\b`), rule{Type: "console", Brand: "Sony", Conf: 40}},
	{regexp.MustCompile(`(?i)xbox`), rule{Type: "console", Brand: "Microsoft", Conf: 40}},
}

// vendorRules: clave buscada como PALABRA COMPLETA en el fabricante OUI
// (ver la nota de auditoría al principio del archivo). Se revisan en orden y
// gana la primera que calce; con palabra completa ya no hace falta ordenar
// "asustek" antes que "asus" a mano — "asus" ya no calza dentro de "ASUSTek"
// (son un solo token, sin borde de palabra entre medio) — pero "blink" sigue
// necesitando ir ANTES que "amazon": "Blink by Amazon" debe ser cámara, no
// altavoz genérico.
var vendorRules = []struct {
	Contains string
	R        rule
}{
	{"tenda", rule{Type: "router", Conf: 40}},
	{"tp-link", rule{Type: "router", Conf: 40}},
	{"netgear", rule{Type: "router", Conf: 40}},
	{"asus", rule{Type: "router", Conf: 40}},
	{"ubiquiti", rule{Type: "router", Conf: 40}},
	{"mikrotik", rule{Type: "router", Conf: 40}},
	{"routerboard", rule{Type: "router", Conf: 40}}, // MikroTik registra su OUI como "Routerboard.com", no "Mikrotik"
	{"blink", rule{Type: "camera", Conf: 40}},       // antes de "amazon": Blink (cámaras) es marca de Amazon
	{"ring", rule{Type: "camera", Conf: 40}},
	{"altobeam", rule{Type: "camera", Conf: 40}},
	{"hikvision", rule{Type: "camera", Conf: 40}},
	{"dahua", rule{Type: "camera", Conf: 40}},
	{"wyze", rule{Type: "camera", Conf: 40}},
	{"reolink", rule{Type: "camera", Conf: 40}},
	{"eufy", rule{Type: "camera", Conf: 40}},
	{"tuya", rule{Type: "iot", Conf: 35}},
	{"intellirocks", rule{Type: "iot", Conf: 40}}, // Govee (luces y tiras LED)
	{"espressif", rule{Type: "iot", Conf: 35}},
	{"shelly", rule{Type: "iot", Conf: 35}},
	{"sonoff", rule{Type: "iot", Conf: 35}},
	{"broadlink", rule{Type: "iot", Conf: 35}},
	{"nintendo", rule{Type: "console", Conf: 40}},
	{"sony interactive", rule{Type: "console", Conf: 40}},
	{"amazon", rule{Type: "speaker", Conf: 30}},
	{"apple", rule{Conf: 15}},
	{"samsung", rule{Conf: 15}},
	{"intel", rule{Type: "computer", Conf: 25}},
	{"asustek", rule{Type: "computer", Conf: 25}},
	{"giga-byte", rule{Type: "computer", Conf: 25}},
	{"micro-star", rule{Type: "computer", Conf: 25}},
	{"raspberry", rule{Type: "computer", OS: "Linux", Conf: 30}},
	{"synology", rule{Type: "nas", Conf: 60}},
	{"qnap", rule{Type: "nas", Conf: 60}},
}

// vendorRuleRe: regex por palabra completa, precompiladas, alineadas por
// índice con vendorRules.
var vendorRuleRe = func() []*regexp.Regexp {
	re := make([]*regexp.Regexp, len(vendorRules))
	for i, vr := range vendorRules {
		re[i] = compileWordBoundary(vr.Contains)
	}
	return re
}()
