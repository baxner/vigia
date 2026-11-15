package core

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// ===========================================================================
// CONTRATO A IMPLEMENTAR (módulo "descubrimiento"). Ver core/discover_extra.go
// para los helpers de cada protocolo (construcción/parseo de paquetes). No
// cambiar las firmas de aquí abajo: scan.go las llama tal cual.
// ===========================================================================

// discoverMDNS envía consultas mDNS/Bonjour (UDP multicast 224.0.0.251:5353, y la
// IPv6 ff02::fb) y escucha `timeout`. Hace:
//   - pregunta por "_services._dns-sd._udp.local" (PTR) para enumerar tipos y, de
//     forma pragmática, también por un puñado de tipos comunes
//     (_airplay._tcp, _googlecast._tcp, _spotify-connect._tcp, _raop._tcp,
//     _ipp._tcp, _printer._tcp, _homekit._tcp, _hap._tcp, _companion-link._tcp,
//     _sleep-proxy._udp, _smb._tcp, _afpovertcp._tcp, _ssh._tcp, _device-info._tcp,
//     _rfb._tcp), todo en un único paquete (una pregunta por nombre).
//   - cuando la enumeración dns-sd revela un tipo de servicio que no estaba en la
//     lista fija, repregunta ese tipo puntualmente (una sola vez) — así llegan
//     también, sin pedirlos a mano, tipos como _teamviewer._tcp u
//     _oculusal_sp._tcp (ver mdnsServiceRules en recognize_rules.go).
//   - resuelve A/AAAA de los hostnames hallados (vía SRV) y asocia cada servicio a
//     su IP; si un respondedor no mandó A/AAAA como registro adicional (lo normal
//     es que sí lo haga junto con SRV/TXT), se pregunta ese host explícitamente.
//   - procesa TODAS las secciones del paquete (answer + authority + additional)
//     parejo: un respondedor "completo" mete PTR/SRV/TXT/A como adicionales de una
//     sola respuesta. Esto importa en particular para "_device-info._tcp": en la
//     red real, macOS NO contesta una pregunta directa por ese tipo, pero cuela su
//     TXT (model=/osxvers=) como adicional de CUALQUIER otra respuesta sobre el
//     mismo host (p. ej. la de "_rfb._tcp"), sin PTR ni SRV propios — se
//     correlaciona por nombre base en discover_extra.go (ver metadataOnlyService).
//   - lee TXT para sacar modelo ("md"/"model" de Chromecast/HomeKit, "am" de
//     AirPlay, "rpMd" de companion-link), SO (Darwin "osxvers", crudo),
//     fabricante ("manufacturer"/"integrator") y nombre de instancia humano
//     ("fn" de Chromecast, más el propio nombre de la instancia DNS-SD).
//
// Devuelve un mapa IP(string) → MDNSInfo. Best-effort: ignora errores por socket
// (p. ej. sin IPv6 configurado) y nunca bloquea más allá de `timeout`.
//
// Implementación: sockets UDP multicast crudos (net.ListenMulticastUDP —  en
// macOS/Linux activa SO_REUSEADDR/SO_REUSEPORT, por eso puede convivir con
// mDNSResponder/avahi ya escuchando en :5353) + golang.org/x/net/dns/dnsmessage
// para construir y parsear los mensajes DNS (ver core/discover_extra.go).
func discoverMDNS(ctx context.Context, timeout time.Duration) map[string]MDNSInfo {
	return discoverMDNSWithIface(ctx, timeout, "")
}

func discoverMDNSWithIface(ctx context.Context, timeout time.Duration, iface string) map[string]MDNSInfo {
	// Nos acotamos también por nuestra cuenta: así la función es autosuficiente
	// aunque algún día se llame con un ctx sin deadline propio.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	acc := newMDNSAccumulator()

	// IPv4 e IPv6 son independientes: si una familia falla (p. ej. sin IPv6 en la
	// interfaz) la otra sigue su curso.
	var wg sync.WaitGroup
	for _, fam := range []struct{ network, group string }{
		{"udp4", mdnsIPv4Group},
		{"udp6", mdnsIPv6Group},
	} {
		wg.Add(1)
		go func(network, group string) {
			defer wg.Done()
			runMDNSListener(ctx, network, group, iface, acc)
		}(fam.network, fam.group)
	}
	wg.Wait()

	return acc.result()
}

// discoverSSDP envía M-SEARCH SSDP (UDP 239.255.255.250:1900; ST "ssdp:rootdevice"
// y "upnp:rootdevice" como pide el contrato, más "ssdp:all" de bonus para no
// perder dispositivos que solo contestan a ese ST) y escucha `timeout`. Por cada
// respuesta:
//   - toma la IP del remitente y la cabecera SERVER.
//   - baja el XML de LOCATION (HTTP GET, timeout corto ~3s) y extrae friendlyName,
//     manufacturer, modelName, modelNumber, deviceType.
//
// El GET de cada LOCATION corre en su propia goroutine con un timeout propio de
// ~3s independiente de la ventana de escucha: una respuesta SSDP que llega justo
// al final de `timeout` merece igual su intento de descarga, en vez de que la
// cancelemos de inmediato porque el reloj de la escucha ya iba a expirar.
// discoverSSDP espera (wg.Wait) a que todas esas descargas terminen antes de
// devolver el mapa, así que puede tardar un poco más que `timeout` en retornar.
//
// Devuelve IP(string) → SSDPInfo. Best-effort: ignora errores de socket/HTTP/XML.
func discoverSSDP(ctx context.Context, timeout time.Duration) map[string]SSDPInfo {
	return discoverSSDPWithIface(ctx, timeout, "")
}

func discoverSSDPWithIface(ctx context.Context, timeout time.Duration, iface string) map[string]SSDPInfo {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runSSDPDiscovery(ctx, timeout, iface)
}

// queryNetBIOS hace una consulta de nombre NetBIOS (UDP 137, "node status
// request", nombre "*" codificado) a cada IP de `ips`, en paralelo (acotado a 64
// a la vez) con un timeout corto por host (no por el total: así una IP muerta no
// le roba tiempo a las demás). De la respuesta saca el nombre de equipo (entrada
// única tipo workstation 0x00 o file-server 0x20) y la MAC (va justo después de
// la tabla de nombres en el paquete node-status). Devuelve IP(string) →
// NetBIOSInfo solo para las que respondieron. Útil sobre todo para equipos
// Windows (p. ej. el PC Gamer .55). Best-effort: ignora errores de socket/paquete.
func queryNetBIOS(ctx context.Context, ips []string, timeout time.Duration) map[string]NetBIOSInfo {
	return queryNetBIOSWithIface(ctx, ips, timeout, "")
}

func queryNetBIOSWithIface(ctx context.Context, ips []string, timeout time.Duration, iface string) map[string]NetBIOSInfo {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runNetBIOSQueries(ctx, ips, iface)
}

// reverseDNS resuelve el PTR de ip (DNS inverso) con el contexto dado. Devuelve el
// primer nombre sin el punto final, o "" si no hay.
//
// El contrato no le da un `timeout` explícito (scan.go la llama con el ctx "de
// fondo", sin acotar por su cuenta en ese punto de enrich()); para no colgar el
// escaneo si el resolver no responde, nos acotamos aquí con un tope propio corto.
func reverseDNS(ctx context.Context, ip string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}
