//go:build tools

// Mantiene golang.org/x/mobile en go.mod: el código que genera gomobile bind
// importa golang.org/x/mobile/bind/seq (ver scripts/build-aar.sh).
package mobile

import _ "golang.org/x/mobile/bind"
