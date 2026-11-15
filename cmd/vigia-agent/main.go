// vigia-agent: el agente 24/7 de Vigía (ver docs/agente-api.md). Vigila la
// red local desde un host fijo (macOS, Linux o Raspberry Pi) y expone el
// contrato HTTP que usa la app.
//
//	vigia-agent run [-interval 5m] [-port 7341] [-data DIR] [-name NOMBRE] [-no-mdns] [-http-legacy=false]
//	vigia-agent install                          (macOS: LaunchAgent)
//	vigia-agent uninstall
//	vigia-agent pair [-port 7341] [-data DIR]
//	vigia-agent cert [-data DIR] [-rotate]
//	vigia-agent tokens [-data DIR]
//	vigia-agent revoke <cliente> [-data DIR]
//	vigia-agent status [-port 7341] [-data DIR]
//	vigia-agent webhook add [-types T1,T2] [-secret S] [-data DIR] <url>
//	vigia-agent webhook list [-data DIR]
//	vigia-agent webhook rm [-data DIR] <id>
//	vigia-agent webhook test [-data DIR] <id>
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "run":
		err = runCmd(args)
	case "install":
		err = installCmd(args)
	case "uninstall":
		err = uninstallCmd(args)
	case "pair":
		err = pairCmd(args)
	case "cert":
		err = certCmd(args)
	case "tokens":
		err = tokensCmd(args)
	case "revoke":
		err = revokeCmd(args)
	case "status":
		err = statusCmd(args)
	case "webhook":
		err = webhookCmd(args)
	case "-h", "-help", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `vigia-agent: agente 24/7 de Vigía

Uso:
  vigia-agent run [-interval 5m] [-port 7341] [-data DIR] [-name NOMBRE] [-no-mdns] [-http-legacy=false]
  vigia-agent install [-interval 5m] [-sign "<identidad>"]
  vigia-agent uninstall
  vigia-agent pair [-port 7341] [-data DIR]
  vigia-agent cert [-data DIR] [-rotate]
  vigia-agent tokens [-data DIR]
  vigia-agent revoke <cliente> [-data DIR]
  vigia-agent status [-port 7341] [-data DIR]
  vigia-agent webhook add [-types T1,T2] [-secret S] [-data DIR] <url>
  vigia-agent webhook list [-data DIR]
  vigia-agent webhook rm [-data DIR] <id>
  vigia-agent webhook test [-data DIR] <id>
`)
}
