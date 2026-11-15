//go:build darwin

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

// launchAgentLabel identifica el LaunchAgent (ver docs/agente-api.md,
// "Instalación en macOS").
const launchAgentLabel = "com.baxner.vigia.agent"

// commandRunner ejecuta un comando externo (launchctl, codesign) y
// devuelve su salida combinada — el mismo contrato que
// exec.Command(...).CombinedOutput(). Inyectable para probar install/
// uninstall con un ejecutor falso, sin tocar launchctl ni codesign de
// verdad (ver install_darwin_test.go).
type commandRunner func(name string, args ...string) ([]byte, error)

// realRunner es el commandRunner de producción.
func realRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// unloadPollInterval/unloadMaxAttempts: cuánto se espera, como mucho, a
// que "launchctl bootout" termine de descargar el servicio antes de
// intentar el bootstrap (ver unloadIfLoaded). 50×200ms = 10 s: de sobra
// para un proceso que ya recibió SIGTERM de bootout, muy por debajo de
// lo que notaría un humano instalando a mano.
const (
	unloadPollInterval = 200 * time.Millisecond
	unloadMaxAttempts  = 50
)

// installCmd copia el binario a ~/Library/Application Support/Vigia/,
// escribe el plist (RunAtLoad, KeepAlive, ProcessType=Background,
// LowPriorityIO, Nice=10) y lo carga con launchctl bootstrap. Si el
// servicio ya estaba cargado (una reinstalación sobre un agente vivo), lo
// descarga primero (ver unloadIfLoaded) — sin eso, bootstrap falla con
// "Bootstrap failed: 5: Input/output error" y hay que correr `uninstall`
// a mano antes de poder instalar de nuevo.
func installCmd(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	interval := fs.Duration("interval", 5*time.Minute, "intervalo entre escaneos del servicio")
	sign := fs.String("sign", "", "identidad de firma de código para \"Vigia Agent.app\" (vacío = ad hoc, ver docs/agente-api.md, \"Instalación en macOS\")")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *interval < 30*time.Second {
		return fmt.Errorf("uso: vigia-agent install [-interval 5m] [-sign \"<identidad>\"] (mínimo 30s)")
	}
	return runInstall(*interval, *sign, realRunner, time.Sleep)
}

// runInstall es installCmd ya separado de flag.Parse, para que las
// pruebas lo llamen con un runner y un sleep falsos (ver
// install_darwin_test.go) sin pasar por os.Args ni por launchctl/codesign
// de verdad.
func runInstall(interval time.Duration, signIdentity string, run commandRunner, sleep func(time.Duration)) error {
	paths, err := agent.DefaultPaths("")
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("no se pudo ubicar el propio binario: %w", err)
	}
	// Copia "suelta" para la CLI (status, pair, tokens…): solo habla por
	// loopback, que no pasa por la privacidad de red local.
	cli := filepath.Join(paths.DataDir, "vigia-agent")
	if err := copyExecutable(self, cli); err != nil {
		return err
	}
	// El servicio corre desde una app mínima: macOS 15 bloquea (NECP) toda
	// conexión a la LAN de un proceso lanzado por launchd sin permiso de "Red
	// local", y ese permiso solo se le puede dar a algo con Info.plist.
	dest, err := buildAgentBundle(self, paths.DataDir, signIdentity, run)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("crear %s: %w", launchAgentsDir, err)
	}
	plistPath := filepath.Join(launchAgentsDir, launchAgentLabel+".plist")
	// StandardOutPath/StandardErrorPath apuntan a un archivo APARTE de
	// agent.log (ver rotatelog.go y docs/agente-api.md, "Rotación de
	// logs"): agent.log lo rota el propio proceso con su logger; este
	// otro es solo para lo que el runtime de Go escriba por su cuenta
	// ANTES de que el logger esté listo o DESPUÉS de un panic — launchd
	// redirige el fd crudo, por fuera del control del programa, así que
	// no puede compartir archivo con algo que el programa rota (rotar
	// renombraría el archivo por debajo de un fd que launchd ya tiene
	// abierto).
	launchdLogPath := paths.LogPath() + ".launchd"
	plist := fmt.Sprintf(plistTemplate, launchAgentLabel, dest, interval.String(), launchdLogPath, launchdLogPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", plistPath, err)
	}

	target := fmt.Sprintf("gui/%d", os.Getuid())
	serviceTarget := fmt.Sprintf("%s/%s", target, launchAgentLabel)
	if err := unloadIfLoaded(run, sleep, serviceTarget); err != nil {
		return err
	}
	if out, err := run("launchctl", "bootstrap", target, plistPath); err != nil {
		return fmt.Errorf("launchctl bootstrap %s %s: %w (%s)", target, plistPath, err, string(out))
	}

	fmt.Printf("Instalado: %s\n", dest)
	fmt.Printf("LaunchAgent: %s (cargado en %s)\n", plistPath, target)
	fmt.Printf("Datos: %s\n", paths.DataDir)
	fmt.Printf("Logs:  %s\n", paths.LogPath())
	return nil
}

// unloadIfLoaded descarga serviceTarget si está cargado y espera a que
// termine, para que el bootstrap que sigue nunca se encuentre al agente
// viejo todavía ahí (ver installCmd). "launchctl bootout" sobre un
// servicio que NO está cargado también falla — eso se ignora a propósito,
// igual que uninstallCmd ya hace: no hay forma de distinguir ese error de
// cualquier otro solo con el texto que imprime launchctl entre versiones
// de macOS, y el efecto que importa (que no quede cargado) se vuelve a
// comprobar de todas formas con "launchctl print" a continuación.
func unloadIfLoaded(run commandRunner, sleep func(time.Duration), serviceTarget string) error {
	run("launchctl", "bootout", serviceTarget) //nolint:errcheck // ver comentario arriba.

	for attempt := 0; attempt < unloadMaxAttempts; attempt++ {
		if _, err := run("launchctl", "print", serviceTarget); err != nil {
			return nil // launchctl print falla cuando el servicio ya no está cargado.
		}
		sleep(unloadPollInterval)
	}
	return fmt.Errorf("launchctl bootout %s: el servicio sigue cargado después de %s", serviceTarget, unloadMaxAttempts*unloadPollInterval)
}

// uninstallCmd descarga el LaunchAgent y borra el plist. Los datos
// (agent.db, agent.json) se conservan — igual que el binario copiado, que
// uninstall no borra: el contrato solo dice "lo descarga y borra el
// plist".
func uninstallCmd(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("uso: vigia-agent uninstall")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")

	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), launchAgentLabel)
	if out, err := realRunner("launchctl", "bootout", target); err != nil {
		// No fatal: si ya no estaba cargado, launchctl se queja igual; seguimos a borrar el plist.
		fmt.Fprintf(os.Stderr, "launchctl bootout %s: %v (%s)\n", target, err, string(out))
	}

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("borrar %s: %w", plistPath, err)
	}

	fmt.Println("LaunchAgent descargado y plist borrado. Los datos se conservan (ver `vigia-agent status`).")
	return nil
}

func copyExecutable(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("leer %s: %w", src, err)
	}
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return fmt.Errorf("escribir %s: %w", dest, err)
	}
	return nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
		<string>-interval</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>Nice</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`

// buildAgentBundle arma "Vigia Agent.app" en dir (Info.plist con la razón de
// uso de la red local, binario en Contents/MacOS) y la firma, y devuelve
// la ruta del ejecutable. La primera vez que el agente toca la LAN, macOS
// pide permiso en pantalla.
//
// signIdentity == "": firma ad hoc ("-", como antes de -sign): macOS la
// reconoce entre reinstalaciones DENTRO de la misma corrida, pero el
// requisito designado que guarda el permiso de "Red local" se arma a
// partir del CDHash del binario exacto — cambia con cada build, así que
// la primera actualización del agente le hace perder el permiso a macOS
// (ver docs/agente-api.md, "Instalación en macOS" > "Firma estable").
// signIdentity != "": identidad de firma de código real (de un
// certificado propio en el llavero, ver la misma sección de los docs): el
// requisito designado por defecto que arma codesign para un certificado
// de este tipo referencia el propio certificado (su CN), no el binario —
// se mantiene igual mientras se reuse el mismo certificado para firmar
// cada build, y el permiso sobrevive a las actualizaciones. Fallar al
// firmar con una identidad EXPLÍCITA es un error fatal (si se pidió
// -sign, un fallback silencioso a ad hoc derrotaría el propósito); fallar
// al firmar ad hoc solo es un aviso, igual que antes de -sign.
func buildAgentBundle(self, dir, signIdentity string, run commandRunner) (string, error) {
	app := filepath.Join(dir, "Vigia Agent.app")
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return "", fmt.Errorf("crear %s: %w", macos, err)
	}
	exe := filepath.Join(macos, "vigia-agent")
	if err := copyExecutable(self, exe); err != nil {
		return "", err
	}
	plistPath := filepath.Join(app, "Contents", "Info.plist")
	if err := os.WriteFile(plistPath, []byte(fmt.Sprintf(bundleInfoPlist, version)), 0o644); err != nil {
		return "", fmt.Errorf("escribir %s: %w", plistPath, err)
	}

	identity := signIdentity
	if identity == "" {
		identity = "-"
	}
	if out, err := run("codesign", "--force", "--sign", identity, app); err != nil {
		if signIdentity != "" {
			return "", fmt.Errorf("firmar %s con la identidad %q: %w (%s)", app, signIdentity, err, out)
		}
		fmt.Fprintf(os.Stderr, "aviso: no se pudo firmar %s: %v (%s)\n", app, err, out)
	}
	return exe, nil
}

const bundleInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.baxner.vigia.agent</string>
	<key>CFBundleName</key>
	<string>Vigía Agent</string>
	<key>CFBundleDisplayName</key>
	<string>Vigía Agent</string>
	<key>CFBundleExecutable</key>
	<string>vigia-agent</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSLocalNetworkUsageDescription</key>
	<string>Vigía revisa qué equipos hay en tu red de casa para avisarte de intrusos y desconexiones. Nada sale de tu red.</string>
	<key>NSBonjourServices</key>
	<array>
		<string>_vigia._tcp</string>
		<string>_services._dns-sd._udp</string>
	</array>
</dict>
</plist>
`
