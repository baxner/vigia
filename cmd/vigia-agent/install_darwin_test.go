//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunnerCall es una llamada que el fakeRunner registró, para que las
// pruebas comprueben el orden (p. ej. "bootout" antes de "bootstrap").
type fakeRunnerCall struct {
	name string
	args []string
}

// fakeRunner es un commandRunner de prueba: nunca toca launchctl/codesign
// de verdad (ver la regla del encargo: nada de launchctl/codesign sobre
// el agente real). printLoadedFor controla cuántas veces seguidas
// "launchctl print" debe simular "todavía cargado" antes de simular
// "descargado" (error) — así se prueba unloadIfLoaded sin dormir de
// verdad. codesignErr, si no es nil, es lo que debe devolver cualquier
// llamada a "codesign".
type fakeRunner struct {
	calls          []fakeRunnerCall
	printLoadedFor int
	printCalls     int
	codesignErr    error
	bootstrapErr   error
	sleeps         int
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeRunnerCall{name: name, args: args})
	switch name {
	case "launchctl":
		if len(args) > 0 && args[0] == "print" {
			f.printCalls++
			if f.printCalls <= f.printLoadedFor {
				return []byte("cargado"), nil // "sigue cargado": print no da error.
			}
			return nil, fmt.Errorf("No such process") // ya descargado.
		}
		if len(args) > 0 && args[0] == "bootstrap" {
			if f.bootstrapErr != nil {
				return []byte("boom"), f.bootstrapErr
			}
			return nil, nil
		}
		if len(args) > 0 && args[0] == "bootout" {
			return nil, nil // bootout real también puede fallar si no estaba cargado; no importa aquí.
		}
	case "codesign":
		if f.codesignErr != nil {
			return []byte("rejected"), f.codesignErr
		}
		return nil, nil
	}
	return nil, nil
}

func (f *fakeRunner) sleep(time.Duration) { f.sleeps++ }

// testHomeFakeSelf prepara $HOME en un directorio temporal (para que
// install nunca toque ~/Library de verdad, ver la regla del encargo) y un
// archivo cualquiera que sirva de "propio binario" para os.Executable()
// — install solo lo copia con os.ReadFile/WriteFile, nunca lo ejecuta.
func testHomeFakeSelf(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
}

func TestUnloadIfLoaded_NoEstabaCargado_NoEspera(t *testing.T) {
	fr := &fakeRunner{printLoadedFor: 0}
	if err := unloadIfLoaded(fr.run, fr.sleep, "gui/501/com.baxner.vigia.agent"); err != nil {
		t.Fatalf("unloadIfLoaded: %v", err)
	}
	if fr.sleeps != 0 {
		t.Fatalf("sleeps = %d, want 0 (print ya dio error en el primer intento)", fr.sleeps)
	}
	if fr.printCalls != 1 {
		t.Fatalf("printCalls = %d, want 1", fr.printCalls)
	}
}

func TestUnloadIfLoaded_EstabaCargado_EsperaYLuegoConfirma(t *testing.T) {
	fr := &fakeRunner{printLoadedFor: 3}
	if err := unloadIfLoaded(fr.run, fr.sleep, "gui/501/com.baxner.vigia.agent"); err != nil {
		t.Fatalf("unloadIfLoaded: %v", err)
	}
	if fr.sleeps != 3 {
		t.Fatalf("sleeps = %d, want 3 (una por cada print que dijo \"sigue cargado\")", fr.sleeps)
	}
	if fr.printCalls != 4 {
		t.Fatalf("printCalls = %d, want 4", fr.printCalls)
	}
	// bootout se llamó una sola vez, antes que cualquier print.
	if len(fr.calls) == 0 || fr.calls[0].name != "launchctl" || fr.calls[0].args[0] != "bootout" {
		t.Fatalf("primera llamada = %+v, want bootout primero", fr.calls)
	}
}

func TestUnloadIfLoaded_NuncaSeDescarga_Error(t *testing.T) {
	fr := &fakeRunner{printLoadedFor: unloadMaxAttempts + 10}
	err := unloadIfLoaded(fr.run, fr.sleep, "gui/501/com.baxner.vigia.agent")
	if err == nil {
		t.Fatal("unloadIfLoaded: want error (nunca se descargó)")
	}
	if fr.sleeps != unloadMaxAttempts {
		t.Fatalf("sleeps = %d, want %d (se agotaron los intentos)", fr.sleeps, unloadMaxAttempts)
	}
}

// TestRunInstall_ServicioYaCargado_BootoutAntesDeBootstrap reproduce el
// bug del encargo: con el servicio ya cargado, install debe descargarlo
// (bootout, esperar) ANTES de bootstrap — nunca "Bootstrap failed: 5:
// Input/output error" por intentar cargar encima de uno que ya estaba
// cargado.
func TestRunInstall_ServicioYaCargado_BootoutAntesDeBootstrap(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{printLoadedFor: 2}

	if err := runInstall(5*time.Minute, "", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	var sawBootout, sawBootstrap, sawBootoutBeforeBootstrap bool
	for _, c := range fr.calls {
		if c.name == "launchctl" && len(c.args) > 0 {
			switch c.args[0] {
			case "bootout":
				sawBootout = true
			case "bootstrap":
				sawBootstrap = true
				if sawBootout {
					sawBootoutBeforeBootstrap = true
				}
			}
		}
	}
	if !sawBootout || !sawBootstrap || !sawBootoutBeforeBootstrap {
		t.Fatalf("calls = %+v, want bootout y bootstrap, en ese orden", fr.calls)
	}
}

func TestRunInstall_ServicioNoCargado_InstalaSinEsperar(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{printLoadedFor: 0}

	if err := runInstall(5*time.Minute, "", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v", err)
	}
	if fr.sleeps != 0 {
		t.Fatalf("sleeps = %d, want 0 (no estaba cargado)", fr.sleeps)
	}
}

func TestRunInstall_SignVacio_FirmaAdHoc(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{}
	if err := runInstall(5*time.Minute, "", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v", err)
	}
	found := false
	for _, c := range fr.calls {
		if c.name == "codesign" {
			found = true
			if len(c.args) < 3 || c.args[2] != "-" {
				t.Fatalf("codesign args = %v, want firma ad hoc (\"-\")", c.args)
			}
		}
	}
	if !found {
		t.Fatal("codesign nunca se llamó")
	}
}

func TestRunInstall_SignConIdentidad_UsaEsaIdentidad(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{}
	if err := runInstall(5*time.Minute, "Vigia Agent Signing", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v", err)
	}
	found := false
	for _, c := range fr.calls {
		if c.name == "codesign" {
			found = true
			if len(c.args) < 3 || c.args[2] != "Vigia Agent Signing" {
				t.Fatalf("codesign args = %v, want la identidad pedida", c.args)
			}
		}
	}
	if !found {
		t.Fatal("codesign nunca se llamó")
	}
}

func TestRunInstall_SignConIdentidad_FirmaFallaEsFatal(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{codesignErr: fmt.Errorf("identidad no encontrada en el llavero")}
	err := runInstall(5*time.Minute, "Vigia Agent Signing", fr.run, fr.sleep)
	if err == nil {
		t.Fatal("runInstall: want error (se pidió -sign explícito y codesign falló)")
	}
	for _, c := range fr.calls {
		if c.name == "launchctl" && len(c.args) > 0 && c.args[0] == "bootstrap" {
			t.Fatal("bootstrap no debió llamarse: la firma pedida explícitamente falló")
		}
	}
}

func TestRunInstall_SignVacio_FirmaFallaSoloAvisa(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{codesignErr: fmt.Errorf("sin identidad ad hoc disponible")}
	if err := runInstall(5*time.Minute, "", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v (firma ad hoc fallida no debería ser fatal)", err)
	}
	sawBootstrap := false
	for _, c := range fr.calls {
		if c.name == "launchctl" && len(c.args) > 0 && c.args[0] == "bootstrap" {
			sawBootstrap = true
		}
	}
	if !sawBootstrap {
		t.Fatal("bootstrap debió llamarse igual: la firma ad hoc fallida solo avisa")
	}
}

// TestRunInstall_PlistUsaArchivoDeLogsAparteParaLaunchd comprueba que el
// plist NO redirige StandardOutPath/StandardErrorPath al mismo agent.log
// que rota el propio proceso (ver rotatelog.go): si compartieran archivo,
// rotar le arrancaría el archivo de abajo al fd que launchd ya tiene
// abierto.
func TestRunInstall_PlistUsaArchivoDeLogsAparteParaLaunchd(t *testing.T) {
	testHomeFakeSelf(t)
	fr := &fakeRunner{}
	if err := runInstall(5*time.Minute, "", fr.run, fr.sleep); err != nil {
		t.Fatalf("runInstall: %v", err)
	}
	home := os.Getenv("HOME")
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("leer plist: %v", err)
	}
	plist := string(data)
	agentLog := filepath.Join(home, "Library", "Logs", "Vigia", "agent.log")
	if !strings.Contains(plist, agentLog+".launchd") {
		t.Fatalf("plist no referencia %s.launchd:\n%s", agentLog, plist)
	}
	if strings.Contains(plist, ">"+agentLog+"<") {
		t.Fatalf("plist referencia agent.log directo (debería ser agent.log.launchd):\n%s", plist)
	}
}
