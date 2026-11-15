package core

import (
	"context"
	"time"
)

// Funciones exportadas para quien use el motor fuera del paquete (la capa de
// enlace con Android en mobile/, y el agente). Scan está en scan.go.

// VendorForMAC devuelve el fabricante de una MAC por su OUI, o "" si no se
// conoce o la MAC es aleatoria (localmente administrada). La app la usa también
// para los BSSID del escáner Wi-Fi.
func VendorForMAC(mac string) string {
	return vendorForMAC(mac)
}

// BrandForMAC devuelve la marca limpia del fabricante de una MAC ("Tenda" en
// vez de "Tenda Technology,Ltd.Dongguan branch"), o "" si no se conoce. Para
// mostrar en espacios cortos, como la lista de redes Wi-Fi.
func BrandForMAC(mac string) string {
	if v := vendorForMAC(mac); v != "" {
		return cleanBrand(v)
	}
	return ""
}

// ScanHostPorts escanea los puertos TCP `ports` de un solo host y devuelve los
// abiertos, ordenados. Para el detalle de un dispositivo ("escanear más puertos").
// timeout es por puerto; cero = DefaultPortTimeout.
func ScanHostPorts(ctx context.Context, ip string, ports []int, timeout time.Duration) []int {
	return scanPorts(ctx, ip, ports, timeout)
}

// ScanHostPortsOnInterface es equivalente a ScanHostPorts pero enlaza los
// sockets TCP a `iface` (p. ej. "en0") cuando está disponible, con respaldo
// transparente si la interfaz no existe o el SO no soporta el enlace.
func ScanHostPortsOnInterface(ctx context.Context, ip string, ports []int, timeout time.Duration, iface string) []int {
	return scanPortsWithIface(ctx, ip, ports, timeout, iface)
}
