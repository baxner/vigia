//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baxner/vigia/agent"
)

func TestRunInstallAndUninstallLinux_UserAndRoot(t *testing.T) {
	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "vigia-agent-src")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile fakeBin: %v", err)
	}

	paths := agent.Paths{
		DataDir: filepath.Join(tmp, "data"),
		LogDir:  filepath.Join(tmp, "logs"),
	}

	var calls []string
	runner := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}

	// 1. Modo usuario (no root)
	home := filepath.Join(tmp, "home")
	if err := runInstallLinux(5*time.Minute, false, tmp, home, fakeBin, paths, runner); err != nil {
		t.Fatalf("runInstallLinux user: %v", err)
	}
	userUnit := filepath.Join(home, ".config", "systemd", "user", systemdUnitName)
	data, err := os.ReadFile(userUnit)
	if err != nil {
		t.Fatalf("ReadFile userUnit: %v", err)
	}
	if !strings.Contains(string(data), "WantedBy=default.target") {
		t.Fatalf("userUnit = %s, falta WantedBy=default.target", string(data))
	}
	if err := runUninstallLinux(false, tmp, home, runner); err != nil {
		t.Fatalf("runUninstallLinux user: %v", err)
	}
	if _, err := os.Stat(userUnit); !os.IsNotExist(err) {
		t.Fatalf("userUnit sigue existiendo tras uninstall: %v", err)
	}

	// 2. Modo sistema (root)
	calls = nil
	rootDir := filepath.Join(tmp, "root")
	if err := runInstallLinux(2*time.Minute, true, rootDir, home, fakeBin, paths, runner); err != nil {
		t.Fatalf("runInstallLinux root: %v", err)
	}
	sysUnit := filepath.Join(rootDir, "etc", "systemd", "system", systemdUnitName)
	sysData, err := os.ReadFile(sysUnit)
	if err != nil {
		t.Fatalf("ReadFile sysUnit: %v", err)
	}
	if !strings.Contains(string(sysData), "WantedBy=multi-user.target") {
		t.Fatalf("sysUnit = %s, falta WantedBy=multi-user.target", string(sysData))
	}
	if err := runUninstallLinux(true, rootDir, home, runner); err != nil {
		t.Fatalf("runUninstallLinux root: %v", err)
	}
	if _, err := os.Stat(sysUnit); !os.IsNotExist(err) {
		t.Fatalf("sysUnit sigue existiendo tras uninstall: %v", err)
	}
}
