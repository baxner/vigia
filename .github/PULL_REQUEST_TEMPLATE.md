## Resumen

Describe brevemente qué resuelve o agrega este *Pull Request*.

## Componentes modificados

- [ ] Motor Go (`core/`, `mobile/`)
- [ ] Agente 24/7 (`agent/`, `cmd/vigia-agent/`, `deploy/`, Docker)
- [ ] App Android (`android/`)
- [ ] App iOS (`ios/`)
- [ ] Documentación / CI (`README.md`, `docs/`, `.github/`)

## Lista de verificación

- [ ] `gofmt -s -l .` sin cambios pendientes y `go vet ./...` / `go test -race ./...` en verde (si aplica a Go).
- [ ] Pruebas unitarias de Android (`./gradlew testDebugUnitTest`) o validación de tipos iOS ejecutadas (si aplica).
- [ ] No se incluyen datos personales, MACs reales, IPs domésticas/públicas, tokens, `.keystore`, `.pem` ni bases `.db` (solo datos sintéticos RFC 5737 / `02:00:00:...`).
- [ ] Documentación actualizada (`docs/agente-api.md` / `docs/instalacion.md`) si cambiaron endpoints, flags o instrucciones de build.
- [ ] Las contribuciones respetan el esquema de licencia dual (`LICENSE`, `android/LICENSE`, `ios/LICENSE`) y [`TRADEMARK.md`](TRADEMARK.md).
