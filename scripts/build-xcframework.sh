#!/bin/sh
# Compila el motor de Go (core + mobile) a un XCFramework para iOS:
# ios/Frameworks/VigiaCore.xcframework (ios/arm64 para iPhone + iossimulator/arm64,amd64).
#
# Evita el bug de gomobile bind -target=ios donde cmd/gomobile pasa dos veces
# el mismo -framework .../iphonesimulator/VigiaCore.framework a
# `xcodebuild -create-xcframework` cuando hay más de una arquitectura de
# simulador, interponiendo un wrapper temporal de xcodebuild que deduplica
# los argumentos `-framework <ruta>` repetidos.
#
# Requisitos: Xcode con SDKs de iOS y gomobile:
#   go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest
#   gomobile init
#
# Uso: scripts/build-xcframework.sh [salida.xcframework]
set -eu
cd "$(dirname "$0")/.."

out="${1:-ios/Frameworks/VigiaCore.xcframework}"
target="${VIGIA_IOS_TARGET:-ios/arm64,iossimulator/arm64,iossimulator/amd64}"
PATH="$(go env GOPATH)/bin:$PATH"

command -v gomobile >/dev/null || {
	echo "falta gomobile (ver los requisitos al inicio de este script)" >&2
	exit 1
}

real_xcodebuild="$(command -v xcodebuild || true)"
if [ -z "$real_xcodebuild" ]; then
	echo "falta xcodebuild (instala Xcode y ejecuta sudo xcode-select -s /Applications/Xcode.app)" >&2
	exit 1
fi

tmpbin="$(mktemp -d "${TMPDIR:-/tmp}/vigia-xcfw.XXXXXX")"
trap 'rm -rf "$tmpbin"' EXIT

cat >"$tmpbin/xcodebuild" <<'EOF'
#!/usr/bin/env python3
import os
import sys

real = os.environ.get("VIGIA_REAL_XCODEBUILD", "/usr/bin/xcodebuild")
args = sys.argv[1:]
if args and args[0] == "-create-xcframework":
    out = []
    seen = set()
    i = 0
    while i < len(args):
        if args[i] == "-framework" and i + 1 < len(args):
            fw = os.path.normpath(args[i + 1])
            if fw not in seen:
                seen.add(fw)
                out.extend(["-framework", args[i + 1]])
            i += 2
        else:
            out.append(args[i])
            i += 1
    args = out

os.execv(real, [real] + args)
EOF
chmod +x "$tmpbin/xcodebuild"

mkdir -p "$(dirname "$out")"
rm -rf "$out"

VIGIA_REAL_XCODEBUILD="$real_xcodebuild" \
PATH="$tmpbin:$PATH" \
gomobile bind \
	-target="$target" \
	-trimpath -ldflags="-s -w" \
	-o "$out" ./mobile

ls -ld "$out"
