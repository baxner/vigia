//go:build linux

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/baxner/vigia/agent"
)

const systemdUnitName = "vigia-agent.service"

type commandRunner func(name string, args ...string) ([]byte, error)

func realRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// installCmd instala el binario en el directorio de datos y registra una
// unidad systemd: de sistema (/etc/systemd/system/vigia-agent.service) si
// corre como root, o de usuario (~/.config/systemd/user/vigia-agent.service)
// en caso contrario.
func installCmd(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	interval := fs.Duration("interval", 5*time.Minute, "intervalo entre escaneos del servicio")
	_ = fs.String("sign", "", "identidad de firma (solo aplica en macOS)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *interval < 30*time.Second {
		return fmt.Errorf("uso: vigia-agent install [-interval 5m] (mínimo 30s)")
	}

	paths, err := agent.DefaultPaths("")
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("no se pudo ubicar el propio binario: %w", err)
	}
	isRoot := os.Geteuid() == 0
	var home string
	if !isRoot {
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	return runInstallLinux(*interval, isRoot, "/", home, self, paths, realRunner)
}

func runInstallLinux(interval time.Duration, isRoot bool, rootDir, homeDir, selfPath string, paths agent.Paths, run commandRunner) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	dest := filepath.Join(paths.DataDir, "vigia-agent")
	if err := copyExecutableAtomic(selfPath, dest); err != nil {
		return err
	}

	var unitPath, wantedBy string
	if isRoot {
		unitDir := filepath.Join(rootDir, "etc", "systemd", "system")
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			return fmt.Errorf("crear %s: %w", unitDir, err)
		}
		unitPath = filepath.Join(unitDir, systemdUnitName)
		wantedBy = "multi-user.target"
	} else {
		unitDir := filepath.Join(homeDir, ".config", "systemd", "user")
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			return fmt.Errorf("crear %s: %w", unitDir, err)
		}
		unitPath = filepath.Join(unitDir, systemdUnitName)
		wantedBy = "default.target"
	}

	unit := fmt.Sprintf(systemdUnitTemplate, dest, interval.String(), paths.DataDir, wantedBy)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", unitPath, err)
	}

	systemctl := func(subArgs ...string) error {
		var fullArgs []string
		if !isRoot {
			fullArgs = append(fullArgs, "--user")
		}
		fullArgs = append(fullArgs, subArgs...)
		if out, err := run("systemctl", fullArgs...); err != nil {
			return fmt.Errorf("systemctl %v: %w (%s)", fullArgs, err, string(out))
		}
		return nil
	}

	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", systemdUnitName); err != nil {
		return err
	}
	if err := systemctl("restart", systemdUnitName); err != nil {
		return err
	}

	fmt.Printf("Instalado: %s\n", dest)
	fmt.Printf("Servicio systemd: %s\n", unitPath)
	fmt.Printf("Datos: %s\n", paths.DataDir)
	fmt.Printf("Logs:  %s\n", paths.LogPath())
	return nil
}

// uninstallCmd deshabilita y detiene la unidad systemd y borra el archivo
// .service. Conserva los datos en el directorio del agente.
func uninstallCmd(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("uso: vigia-agent uninstall")
	}
	isRoot := os.Geteuid() == 0
	var home string
	if !isRoot {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	return runUninstallLinux(isRoot, "/", home, realRunner)
}

func runUninstallLinux(isRoot bool, rootDir, homeDir string, run commandRunner) error {
	var unitPath string
	var prefix []string
	if isRoot {
		unitPath = filepath.Join(rootDir, "etc", "systemd", "system", systemdUnitName)
	} else {
		unitPath = filepath.Join(homeDir, ".config", "systemd", "user", systemdUnitName)
		prefix = []string{"--user"}
	}

	disableArgs := append(append([]string{}, prefix...), "disable", "--now", systemdUnitName)
	if out, err := run("systemctl", disableArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "systemctl %v: %v (%s)\n", disableArgs, err, string(out))
	}

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("borrar %s: %w", unitPath, err)
	}

	reloadArgs := append(append([]string{}, prefix...), "daemon-reload")
	_, _ = run("systemctl", reloadArgs...)

	fmt.Println("Servicio systemd desinstalado y unidad borrada. Los datos se conservan (ver `vigia-agent status`).")
	return nil
}

func copyExecutableAtomic(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("leer %s: %w", src, err)
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return fmt.Errorf("escribir %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renombrar %s → %s: %w", tmp, dest, err)
	}
	return nil
}

const systemdUnitTemplate = `[Unit]
Description=Vigia Agent (Monitor de red local 24/7)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -interval %s -data %s
Restart=on-failure
RestartSec=5s
Nice=10

[Install]
WantedBy=%s
`
