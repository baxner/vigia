package core

import (
	"encoding/binary"
	"net"
	"os"
	"strconv"
	"strings"
)

func defaultGatewayFromProcNetRoute() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return parseProcNetRoute(string(data), binary.NativeEndian)
}

// parseProcNetRoute extrae el gateway de la ruta por defecto (Destination=00000000,
// Flags RTF_UP|RTF_GATEWAY) eligiendo el de menor métrica. /proc/net/route codifica
// las direcciones IPv4 en hexadecimal según el endianness del host.
func parseProcNetRoute(data string, bo binary.ByteOrder) string {
	const rtfUpGateway = 0x1 | 0x2
	bestMetric := -1
	bestGW := ""

	lines := strings.Split(data, "\n")
	for i, ln := range lines {
		if i == 0 {
			continue // cabecera
		}
		f := strings.Fields(ln)
		if len(f) < 8 {
			continue
		}
		if f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 16)
		if err != nil || flags&rtfUpGateway != rtfUpGateway {
			continue
		}
		gwHex, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil || gwHex == 0 {
			continue
		}
		var b [4]byte
		bo.PutUint32(b[:], uint32(gwHex))
		gw := net.IPv4(b[0], b[1], b[2], b[3]).String()

		metric, err := strconv.Atoi(f[6])
		if err != nil {
			metric = 0
		}
		if bestMetric == -1 || metric < bestMetric {
			bestMetric = metric
			bestGW = gw
		}
	}
	return bestGW
}
