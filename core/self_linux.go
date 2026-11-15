//go:build linux

package core

import (
	"os"
	"runtime"
	"strings"
)

// selfExtra: en Linux (excluyendo Android, que comparte tag linux) lee
// /etc/os-release y /proc/device-tree/model para identificar distro y placa/equipo.
func selfExtra() map[string]string {
	if runtime.GOOS == "android" {
		return nil
	}
	out := map[string]string{}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		if osName := parseOSRelease(string(data)); osName != "" {
			out["self:os"] = osName
		}
	}
	if data, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		if model := parseDeviceTreeModel(string(data)); model != "" {
			out["self:model"] = model
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseOSRelease(data string) string {
	var pretty, name, version string
	for _, ln := range strings.Split(data, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		v = strings.TrimSpace(v)
		switch k {
		case "PRETTY_NAME":
			pretty = v
		case "NAME":
			name = v
		case "VERSION_ID":
			version = v
		}
	}
	if pretty != "" {
		return pretty
	}
	if name != "" && version != "" {
		return name + " " + version
	}
	return name
}

func parseDeviceTreeModel(data string) string {
	return strings.TrimSpace(strings.ReplaceAll(data, "\x00", ""))
}
