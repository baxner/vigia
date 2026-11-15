# Política de Seguridad (`SECURITY.md`)

Vigía está diseñado para operar de forma privada dentro de tu red local y sin depender de servicios en la nube. Agradecemos enormemente a quienes auditan el código y reportan vulnerabilidades de forma responsable.

## Versiones soportadas

| Versión / Rama | Soporte de seguridad |
|---|---|
| `main` (último lanzamiento `0.1.x`) | ✅ Soportada |
| Versiones anteriores | ❌ Actualizar a `main` |

## Cómo reportar una vulnerabilidad

**Por favor, no abras un Issue público para vulnerabilidades de seguridad.**

1. Utiliza la función **Private Vulnerability Reporting / Security Advisories** en la pestaña **Security** del repositorio en GitHub.
2. Incluye la mayor cantidad de detalle posible:
   - Componente afectado (`core/`, `agent/`, `cmd/vigia-agent`, `android/` o `ios/`).
   - Pasos de reproducción o prueba de concepto.
   - Impacto estimado y configuración utilizada (p. ej. si estaba habilitado `-http-legacy` o exposición en red pública).
   - **Importante:** elimina direcciones MAC reales, IPs públicas, tokens Bearer, certificados privados o datos personales de tus capturas o registros antes de enviar el reporte.

Nos comprometemos a acusar recibo del reporte lo antes posible y coordinar una corrección antes de su divulgación pública.

## Modelo de amenazas y recomendaciones de despliegue

- **Alcance de red esperado:** el Agente 24/7 está pensado para ejecutarse en tu **LAN confiable** o detrás de una **VPN privada** (como Tailscale). Si tu servidor tiene direcciones IPv6 públicas globales, utiliza la bandera `-bind <IP_LAN>:7341` o filtra el puerto `7341/tcp` en tu firewall.
- **TLS y pinning:** desde v1.5, el agente genera un certificado autofirmado ECDSA P-256 cuya huella SHA-256 se verifica y fija en Android/iOS durante la vinculación. Si ya migraste tus apps clientes, se recomienda arrancar el agente con `-http-legacy=false` para desactivar HTTP sin cifrar en el puerto 7341.
- **Webhooks:** el agente valida URLs `http`/`https`, prohíbe credenciales incrustadas, bloquea redirecciones HTTP y rechaza conexiones hacia direcciones `loopback` (`127.0.0.0/8`, `::1`), no especificadas (`0.0.0.0`, `::`) y `link-local`/metadata (`169.254.0.0/16`, `fe80::/10`).
- **Sondas de Internet (Fase 7):** las mediciones de conectividad hacia `1.1.1.1`, `8.8.8.8`, `[2606:4700:4700::1111]:443` y `speed.cloudflare.com` son exclusivamente sondas de latencia/velocidad y nunca transmiten inventario, MACs ni nombres de dispositivos locales.
