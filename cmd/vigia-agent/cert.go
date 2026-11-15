package main

import (
	"flag"
	"fmt"

	"github.com/baxner/vigia/agent"
)

// certCmd muestra la huella SHA-256 del certificado TLS propio del agente
// (ver TLS v1.5, docs/agente-api.md: "para comprobar a mano" contra lo que
// ve la app o un `curl -k`), o lo rota con -rotate.
func certCmd(args []string) error {
	fs := flag.NewFlagSet("cert", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	rotate := fs.Bool("rotate", false, "generar un certificado nuevo (los teléfonos vinculados tendrán que volver a vincularse)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	paths, err := agent.DefaultPaths(*dataDir)
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	if *rotate {
		c, err := agent.RotateCert(paths.DataDir)
		if err != nil {
			return err
		}
		fmt.Println("Certificado rotado.")
		fmt.Printf("Huella SHA-256: %s\n", c.FingerprintSHA256)
		fmt.Println("Los teléfonos ya vinculados van a rechazar la conexión (el certificado cambió, ver TLS v1.5): hay que vincularlos de nuevo.")
		fmt.Println("Si `vigia-agent run` ya está corriendo, sigue usando el certificado anterior hasta que se reinicie (lo carga una sola vez al arrancar).")
		return nil
	}

	c, err := agent.LoadOrCreateCert(paths.DataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Huella SHA-256: %s\n", c.FingerprintSHA256)
	fmt.Printf("Certificado:    %s\n", paths.CertPath())
	return nil
}
