# Política de Marca y Distribución Oficial (Vigía)

**Copyright 2026 baxner.**

Vigía se publica con total transparencia para que cualquier persona pueda auditar cómo funciona, compilarlo desde el código fuente y ejecutarlo en su propia red doméstica sin nube ni rastreo. Al mismo tiempo, la identidad del proyecto y su distribución oficial están protegidas bajo el esquema de licencia dual y marca descrito en este documento.

## 1. Resumen de licencias del código fuente

| Componente | Directorios | Licencia | Alcance |
|---|---|---|---|
| **Motor Go, Agente 24/7, Scripts y Docs** | `core/`, `mobile/`, `agent/`, `cmd/`, `scripts/`, `deploy/`, `docs/` | **Apache License 2.0** ([`LICENSE`](LICENSE)) | Libre para auditar, modificar, compilar, auto-hospedar e integrar respetando avisos de copyright y marca |
| **App Android (Kotlin / Compose)** | `android/` | **PolyForm Noncommercial License 1.0.0** ([`android/LICENSE`](android/LICENSE)) | Libre para auditar, compilar y usar gratuitamente en uso personal o no comercial; prohibida su explotación o distribución comercial por terceros |
| **App iOS / iPhone (Swift / SwiftUI)** | `ios/` | **PolyForm Noncommercial License 1.0.0** ([`ios/LICENSE`](ios/LICENSE)) | Libre para auditar, compilar y usar gratuitamente en uso personal o no comercial; prohibida su explotación o distribución comercial por terceros |

## 2. Derechos de marca reservados

Ni la licencia **Apache-2.0** (Sección 6: *Trademarks*) ni la licencia **PolyForm Noncommercial 1.0.0** (*No Other Rights*) otorgan derechos sobre las marcas del autor. En consecuencia, quedan reservados exclusivamente a **baxner**:

- El nombre **"Vigía"** / **"Vigia"** aplicado a software de monitoreo de red, aplicaciones móviles o servicios relacionados.
- Los logotipos, iconos de aplicación, gráficos distintivos y marcas denominativas oficiales del proyecto.
- La **distribución oficial y comercial** de paquetes y binarios precompilados publicados bajo el nombre **"Vigía"** en tiendas de aplicaciones (**Google Play Store**, **Apple App Store** u otras tiendas/catálogos comerciales o públicos).

## 3. Qué puedes hacer sin pedir permiso

- **Uso personal y auto-hospedaje:** descargar el código, auditarlo, compilar el agente (`vigia-agent`) y las apps móviles para Android o iPhone e instalarlos en tus propios equipos y teléfonos para vigilar tu red doméstica o laboratorio personal.
- **Contribuciones al proyecto:** enviar *Pull Requests* al repositorio oficial conforme a [`CONTRIBUTING.md`](CONTRIBUTING.md).
- **Bifurcaciones (*forks*) de código fuente:** crear y mantener un *fork* en GitHub o GitLab siempre que:
  1. Conserves todos los avisos de copyright y archivos `LICENSE` originales.
  2. Si publicas binarios o aplicaciones compiladas a partir de un *fork* (incluidas versiones modificadas de `android/` o `ios/` bajo uso no comercial), **cambies el nombre de la aplicación, el *Application ID* / *Bundle ID* y los logotipos** para que no puedan confundirse con el producto oficial **Vigía**.
- **Uso referencial (nominativo):** indicar de forma veraz que un proyecto o guía *"es compatible con el Agente Vigía"* o *"está basado en el código fuente de Vigía"*, siempre que no sugiera respaldo oficial ni autoría de baxner.

## 4. Qué está prohibido sin autorización expresa

- Publicar, subir o comercializar copias originales o modificadas de las aplicaciones Android o iOS bajo el nombre **Vigía** (o un nombre/icono confusamente similar) en **Google Play Store**, **Apple App Store**, tiendas alternativas o repositorios de paquetes comerciales.
- Vender, empaquetar comercialmente o ofrecer como servicio de pago (*SaaS* / dispositivo comercial preinstalado) las aplicaciones móviles de `android/` o `ios/`, conforme a la licencia **PolyForm Noncommercial 1.0.0**.
- Utilizar la marca o los logotipos de **Vigía** para implicar respaldo oficial, certificación o afiliación comercial sin acuerdo previo por escrito con **baxner**.

Para consultas sobre licenciamiento comercial o uso autorizado de marca, contacta al titular a través del repositorio oficial en GitHub.
