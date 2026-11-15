# Guía para contribuir a Vigía

¡Gracias por tu interés en contribuir a Vigía! Antes de abrir un *Pull Request*, revisa estas pautas para mantener el código limpio, multiplataforma y libre de datos personales.

## Licencias de tus contribuciones

- Las contribuciones al motor en Go, al Agente 24/7, a los scripts y a la documentación (`core/`, `mobile/`, `agent/`, `cmd/`, `scripts/`, `deploy/`, `docs/`) se aceptan bajo la licencia **Apache License 2.0** ([`LICENSE`](LICENSE)).
- Las contribuciones a las apps móviles (`android/` e `ios/`) se aceptan bajo los términos aplicables de [`android/LICENSE`](android/LICENSE) y [`ios/LICENSE`](ios/LICENSE), permitiendo al proyecto mantener la distribución oficial bajo [`TRADEMARK.md`](TRADEMARK.md).

## Flujo de trabajo

1. Haz un *fork* del repositorio y crea una rama descriptiva (`fix/...`, `feat/...`, `docs/...`). Nunca hagas commits directos sobre `main`.
2. Mantén los cambios acotados y acompañados de pruebas unitarias para cualquier regla nueva o corrección de bug.
3. Si modificas la API HTTP del agente o flags del CLI, actualiza [`docs/agente-api.md`](docs/agente-api.md) o [`docs/instalacion.md`](docs/instalacion.md).

## Comprobaciones locales obligatorias

### Motor Go y Agente (`core/`, `mobile/`, `agent/`, `cmd/`)

```sh
gofmt -s -w .
go vet ./...
go test -race ./...
```

### App Android (`android/`)

Si cambiaste código en `core/` o `mobile/`, regenera primero el `.aar`:

```sh
./scripts/build-aar.sh
cd android
./gradlew testDebugUnitTest
./gradlew --stop
```

### App iOS (`ios/`, en macOS)

Si cambiaste código en `core/` o `mobile/`, regenera el `.xcframework`:

```sh
./scripts/build-xcframework.sh
cd ios && xcodegen generate
```

Y verifica la compilación de tipos Swift o el build en Xcode.

## Reglas estrictas de privacidad (Anti-PII y Anti-Secretos)

La integración continua (**CI**) bloqueará cualquier *Pull Request* que contenga datos personales o archivos sensibles:

- **Usa siempre datos sintéticos de laboratorio** en tests, *fixtures*, capturas y documentación:
  - Subredes de ejemplo: `192.168.1.0/24`, `10.0.0.0/24` o documentación RFC 5737 (`192.0.2.0/24`), y rango RFC 6598 (`100.64.0.x`) para ejemplos de Tailscale.
  - Direcciones MAC localmente administradas (`02:00:00:xx:xx:xx`).
  - Nombres genéricos (`mi-servidor`, `Mi Teléfono`, `PC-Escritorio`).
- **Nunca subas al repositorio:** volcados reales de tu red doméstica, direcciones IP públicas, números de serie reales, tokens de acceso, `.keystore`, `.jks`, `keystore.properties`, `.pem`, `.key`, `.crt`, bases `.db` ni archivos `.env`.

## Convención de commits

Se recomienda seguir [Conventional Commits](https://www.conventionalcommits.org/):

- `feat(core): ...`
- `fix(agent): ...`
- `fix(android): ...`
- `feat(ios): ...`
- `docs: ...`
- `ci: ...`

Consulta también nuestro [Código de Conducta](CODE_OF_CONDUCT.md).
