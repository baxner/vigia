package core

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// fingerprintHTTP busca pistas en servicios web del dispositivo, después del
// escaneo de puertos (necesita d.OpenPorts ya lleno):
//   - en un puerto web abierto (80, 8080, 8000, 5000 llano; 443/8443 con TLS
//     sin verificar) un GET "/" para el <title> y la cabecera Server.
//   - en los puertos de control remoto de TV conocidos, la API propia del
//     aparato (Samsung Tizen 8001, Roku ECP 8060, Chromecast 8008): modelo, SO,
//     nombre y a veces la MAC — esto último importa en Android 11+, donde la
//     tabla ARP está bloqueada y por eso 8 de 18 equipos salen "unknown".
//
// Todo bajo un único presupuesto de ~2s por host (no por request): si un GET
// se cuelga, el contexto corta el resto en vez de restarle tiempo al resto del
// escaneo. Sale de inmediato si no hay ningún puerto relevante abierto.
func fingerprintHTTP(ctx context.Context, d *Device) {
	fingerprintHTTPWithClient(ctx, d, lanClient)
}

func fingerprintHTTPWithClient(ctx context.Context, d *Device, client *http.Client) {
	if client == nil {
		client = lanClient
	}
	webP, webTLS := webPort(d.OpenPorts)
	apiP := apiPort(d.OpenPorts)
	if webP == 0 && apiP == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if webP != 0 {
		if title, server, ok := fetchTitleWithClient(ctx, client, d.IP, webP, webTLS); ok {
			if title != "" {
				setExtra(d, "http:title", title)
			}
			if server != "" {
				setExtra(d, "http:server", server)
			}
		}
	}

	switch apiP {
	case 8001:
		fingerprintSamsungWithClient(ctx, client, d, apiP)
	case 8060:
		fingerprintRokuWithClient(ctx, client, d, apiP)
	case 8008:
		fingerprintChromecastWithClient(ctx, client, d, apiP)
	}
}

// webPort elige el primer puerto "web" abierto para el GET "/" genérico.
// Prioriza HTTP llano (sin handshake TLS) sobre HTTPS.
func webPort(ports []int) (port int, https bool) {
	for _, p := range []int{80, 8080, 8000, 5000} {
		if hasOpenPort(ports, p) {
			return p, false
		}
	}
	for _, p := range []int{443, 8443} {
		if hasOpenPort(ports, p) {
			return p, true
		}
	}
	return 0, false
}

// apiPort: el primer puerto de API propietaria de TV conocido que esté
// abierto (a lo sumo hay uno por aparato en la práctica).
func apiPort(ports []int) int {
	for _, p := range []int{8001, 8060, 8008} {
		if hasOpenPort(ports, p) {
			return p
		}
	}
	return 0
}

func hasOpenPort(ports []int, p int) bool {
	for _, x := range ports {
		if x == p {
			return true
		}
	}
	return false
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// lanTransport: el único transporte HTTP para hablar con los equipos de la
// LAN. Sin proxy (un HTTP_PROXY del sistema no debe ver la red de casa), sin
// keep-alive (un escaneo no reusa conexiones y en la app el proceso vive
// mucho: no deben quedar sockets abiertos) y aceptando los certificados
// autofirmados de cámaras y routers.
func newLANTransport(iface string) *http.Transport {
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            newDialer(iface, 2*time.Second).DialContext,
		TLSClientConfig:        &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — LAN propia
		TLSHandshakeTimeout:    2 * time.Second,
		ResponseHeaderTimeout:  2 * time.Second,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 64 << 10,
	}
}

var lanTransport = newLANTransport("")

// lanClient sigue redirecciones solo dentro del mismo equipo: un aparato no
// puede mandar al escaneo a pedir nada fuera de la LAN.
var lanClient = &http.Client{
	Transport: lanTransport,
	Timeout:   2 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || len(via) == 0 || req.URL.Hostname() != via[0].URL.Hostname() {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func lanClientFor(iface string) *http.Client {
	if strings.TrimSpace(iface) == "" {
		return lanClient
	}
	return &http.Client{
		Transport:     newLANTransport(iface),
		Timeout:       2 * time.Second,
		CheckRedirect: lanClient.CheckRedirect,
	}
}

// genericTitleRe: títulos que no dicen nada del aparato (páginas de error,
// listados de directorio, páginas por defecto de servidores web).
var genericTitleRe = regexp.MustCompile(`(?i)^(\d{3}\b.*|index of\b.*|.*\b(forbidden|not found|unauthorized|bad request|error)\b.*|welcome to nginx.*|.*default page.*|it works!?|iis windows server|untitled|document|home|login|redirect(ing)?\b.*)$`)

// fetchTitle hace un GET "/" (≤64 KB, sin seguir redirecciones a otro host,
// User-Agent "Vigia") y devuelve el <title> (decodificado de entidades HTML)
// y la cabecera Server.
func fetchTitle(ctx context.Context, ip string, port int, https bool) (title, server string, ok bool) {
	return fetchTitleWithClient(ctx, lanClient, ip, port, https)
}

func fetchTitleWithClient(ctx context.Context, client *http.Client, ip string, port int, https bool) (title, server string, ok bool) {
	if client == nil {
		client = lanClient
	}
	scheme := "http"
	if https {
		scheme = "https"
	}
	u := fmt.Sprintf("%s://%s/", scheme, net.JoinHostPort(ip, itoa(port)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("User-Agent", "Vigia")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()

	server = strings.TrimSpace(resp.Header.Get("Server"))
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if m := titleRe.FindSubmatch(body); m != nil {
		title = strings.Join(strings.Fields(html.UnescapeString(string(m[1]))), " ")
		if genericTitleRe.MatchString(title) {
			title = ""
		}
	}
	return title, server, title != "" || server != ""
}

// httpGET: GET llano acotado por ctx y por 64 KB, con nuestro User-Agent y el
// cliente de la LAN (sin proxy, sin salir del equipo). Para las APIs propietarias de abajo,
// que no redirigen ni necesitan HTTPS.
func httpGET(ctx context.Context, ip string, port int, path string) ([]byte, bool) {
	return httpGETWithClient(ctx, lanClient, ip, port, path)
}

func httpGETWithClient(ctx context.Context, client *http.Client, ip string, port int, path string) ([]byte, bool) {
	if client == nil {
		client = lanClient
	}
	u := fmt.Sprintf("http://%s%s", net.JoinHostPort(ip, itoa(port)), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Vigia")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, false
	}
	return body, true
}

// adoptAPIMac: si la API del aparato trajo una MAC y todavía no tenemos una
// (lo normal en Android 11+, donde la tabla ARP está bloqueada), la adoptamos
// junto con el fabricante por OUI y "api" como fuente — así se ve de dónde
// salió, igual que "arp"/"mdns"/"ssdp".
func adoptAPIMac(d *Device, mac string) {
	if d.MAC != "" || mac == "" {
		return
	}
	norm := normalizeMAC(mac)
	if norm == "" {
		return
	}
	d.MAC = norm
	d.Vendor = vendorForMAC(norm)
	addSource(d, "api")
}

// ---------------------------------------------------------------------------
// Samsung Tizen (puerto 8001)
// ---------------------------------------------------------------------------

type samsungAPIInfo struct {
	Device struct {
		OS        string `json:"OS"`
		ModelName string `json:"modelName"`
		Name      string `json:"name"`
		WifiMac   string `json:"wifiMac"`
	} `json:"device"`
}

// fingerprintSamsung lee /api/v2/ (verificado en la red real: TV .118 →
// {"device":{"modelName":"QN75Q70CDFXZA","OS":"Tizen","wifiMac":"F8:4E:..."}}).
func fingerprintSamsung(ctx context.Context, d *Device, port int) {
	fingerprintSamsungWithClient(ctx, lanClient, d, port)
}

func fingerprintSamsungWithClient(ctx context.Context, client *http.Client, d *Device, port int) {
	body, ok := httpGETWithClient(ctx, client, d.IP, port, "/api/v2/")
	if !ok {
		return
	}
	var info samsungAPIInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return
	}
	setExtra(d, "api:type", "tv")
	if info.Device.OS != "" {
		setExtra(d, "api:os", info.Device.OS)
	}
	if info.Device.ModelName != "" {
		setExtra(d, "api:model", info.Device.ModelName)
	}
	if name := strings.TrimSpace(html.UnescapeString(info.Device.Name)); name != "" {
		setExtra(d, "api:name", name)
	}
	adoptAPIMac(d, info.Device.WifiMac)
}

// ---------------------------------------------------------------------------
// Roku ECP (puerto 8060)
// ---------------------------------------------------------------------------

type rokuDeviceInfo struct {
	XMLName      xml.Name `xml:"device-info"`
	ModelName    string   `xml:"model-name"`
	VendorName   string   `xml:"vendor-name"`
	WifiMAC      string   `xml:"wifi-mac"`
	FriendlyName string   `xml:"friendly-device-name"`
}

// fingerprintRoku lee /query/device-info (XML ECP): model-name, vendor-name,
// wifi-mac, friendly-device-name.
func fingerprintRoku(ctx context.Context, d *Device, port int) {
	fingerprintRokuWithClient(ctx, lanClient, d, port)
}

func fingerprintRokuWithClient(ctx context.Context, client *http.Client, d *Device, port int) {
	body, ok := httpGETWithClient(ctx, client, d.IP, port, "/query/device-info")
	if !ok {
		return
	}
	var info rokuDeviceInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return
	}
	setExtra(d, "api:type", "tv")
	setExtra(d, "api:os", "Roku OS")
	if info.ModelName != "" {
		setExtra(d, "api:model", info.ModelName)
	}
	if info.FriendlyName != "" {
		setExtra(d, "api:name", info.FriendlyName)
	}
	adoptAPIMac(d, info.WifiMAC)
}

// ---------------------------------------------------------------------------
// Chromecast (puerto 8008) — best effort: sin contrato público estable.
// ---------------------------------------------------------------------------

// fingerprintChromecast lee /setup/eureka_info. A diferencia de Samsung/Roku
// no hay formato estable documentado, así que es best effort: lo que falte
// simplemente no se llena, sin error.
func fingerprintChromecast(ctx context.Context, d *Device, port int) {
	fingerprintChromecastWithClient(ctx, lanClient, d, port)
}

func fingerprintChromecastWithClient(ctx context.Context, client *http.Client, d *Device, port int) {
	body, ok := httpGETWithClient(ctx, client, d.IP, port, "/setup/eureka_info")
	if !ok {
		return
	}
	var info struct {
		Name      string `json:"name"`
		ModelName string `json:"model_name"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return
	}
	setExtra(d, "api:type", "tv")
	setExtra(d, "api:os", "Cast")
	if info.ModelName != "" {
		setExtra(d, "api:model", info.ModelName)
	}
	if name := strings.TrimSpace(html.UnescapeString(info.Name)); name != "" {
		setExtra(d, "api:name", name)
	}
}
