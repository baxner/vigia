//go:build !darwin && !linux

package main

import (
	"fmt"
	"runtime"
)

func installCmd(args []string) error {
	return fmt.Errorf("instalación automática solo disponible en macOS y Linux (sistema actual: %s); ejecuta `vigia-agent run` como servicio del sistema", runtime.GOOS)
}

func uninstallCmd(args []string) error {
	return fmt.Errorf("instalación automática solo disponible en macOS y Linux (sistema actual: %s)", runtime.GOOS)
}
