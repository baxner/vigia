package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/store"
)

// openStoreForCLI abre la base en modo lectura/escritura directa, para los
// subcomandos que no necesitan que el agente esté corriendo (tokens,
// revoke, status). Comparte el mismo archivo que una instancia de `run`
// que esté corriendo a la vez sin pisarla (WAL + busy_timeout, ver
// agent/store).
func openStoreForCLI(dataDir string) (*store.Store, func(), error) {
	paths, err := agent.DefaultPaths(dataDir)
	if err != nil {
		return nil, nil, err
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(context.Background(), paths.DBPath())
	if err != nil {
		return nil, nil, fmt.Errorf("abrir %s: %w", paths.DBPath(), err)
	}
	return st, func() { _ = st.Close() }, nil
}

// tokensCmd lista los clientes vinculados, vigentes y revocados (ver
// docs/agente-api.md, "Vinculación": el agente solo guarda el hash).
func tokensCmd(args []string) error {
	fs := flag.NewFlagSet("tokens", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	clients, err := st.Clients(context.Background())
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		fmt.Println("(sin clientes vinculados todavía)")
		return nil
	}
	for _, c := range clients {
		estado := "vigente"
		if !c.Active() {
			estado = "revocado " + c.RevokedAt.Local().Format("2006-01-02 15:04")
		}
		ultimoUso := "nunca"
		if c.LastUsedAt != nil {
			ultimoUso = c.LastUsedAt.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("%-24s  desde %s  último uso: %-16s  %s\n",
			c.Name, c.CreatedAt.Local().Format("2006-01-02 15:04"), ultimoUso, estado)
	}
	return nil
}

// revokeCmd revoca todos los tokens vigentes de <cliente> (ver
// store.RevokeByName: puede haber más de uno con el mismo nombre si el
// teléfono se vinculó más de una vez).
func revokeCmd(args []string) error {
	fs := flag.NewFlagSet("revoke", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("uso: vigia-agent revoke <cliente>")
	}
	name := fs.Arg(0)

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	n, err := st.RevokeByName(context.Background(), name, time.Now().UTC())
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("ningún cliente vigente llamado %q", name)
	}
	plural := ""
	if n > 1 {
		plural = "s"
	}
	fmt.Printf("Revocado%s %d token%s de %q.\n", plural, n, plural, name)
	return nil
}
