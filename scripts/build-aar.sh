#!/bin/sh
# Compila el motor de Go (core + mobile) a la librería Android que usa la app:
# android/app/libs/vigia-core.aar (arm64 para teléfonos, x86_64 para el emulador).
#
# Requisitos: Android SDK + NDK, JDK 17 y gomobile:
#   go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest
#   gomobile init
#
# Uso: scripts/build-aar.sh [salida.aar]
set -eu
cd "$(dirname "$0")/.."

out="${1:-android/app/libs/vigia-core.aar}"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
if [ -z "${ANDROID_NDK_HOME:-}" ]; then
	ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)"
	export ANDROID_NDK_HOME
fi
export JAVA_HOME="${JAVA_HOME:-/usr/local/opt/openjdk@17}"
PATH="$(go env GOPATH)/bin:$PATH"

command -v gomobile >/dev/null || {
	echo "falta gomobile (ver los requisitos al inicio de este script)" >&2
	exit 1
}

# -trimpath y -s -w: sin rutas locales ni símbolos de depuración en el .so.
mkdir -p "$(dirname "$out")"
gomobile bind \
	-target=android/arm64,android/amd64 \
	-androidapi 26 \
	-javapkg=com.baxner.vigia.engine \
	-trimpath -ldflags="-s -w" \
	-o "$out" ./mobile

rm -f "${out%.aar}-sources.jar"
ls -lh "$out"
