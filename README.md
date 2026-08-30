# FLUXeando

Aplicación local para registrar y analizar movimientos de caja. La interfaz de escritorio usa Electron + React; el servicio local usa Go y SQLite.

## Descargar e instalar

Las versiones publicadas están en [Releases](https://github.com/dacerb/fluxeando-platform/releases). Descargá el archivo que corresponde a tu sistema:

| Sistema | Archivo |
| --- | --- |
| Windows 64 bits | Instalador `.exe` |
| macOS con Apple Silicon (M1 o posterior) | Imagen `.dmg` |

En macOS la aplicación todavía no está firmada ni notarizada. Si Gatekeeper impide abrirla, consultá [la guía de instalación para macOS](docs/instalacion.md#macos-gatekeeper).

## Ejecutar en desarrollo

Requisitos: Node.js 22, pnpm 9 y Go (la versión indicada por `services/cashflow-api/go.mod`).

```bash
pnpm install
pnpm dev
```

El comando inicia Vite, la API local y Electron. Para ejecutar solamente la interfaz web con la API local:

```bash
pnpm --filter @cashflow/desktop web
```

La API queda disponible únicamente en `127.0.0.1`. En modo de producción Electron la inicia y la mantiene detrás de un puente IPC limitado.

## Ubicación de la base de datos SQLite

La aplicación guarda la base local en un único archivo SQLite. La ubicación predeterminada depende de cómo se ejecute:

| Entorno | Ubicación predeterminada |
| --- | --- |
| Web local y desarrollo (`pnpm dev`) | `services/cashflow-api/cashflow.db` dentro del repositorio |
| Aplicación instalada en Windows | `%APPDATA%\\CashFlow\\cashflow.db` |
| Aplicación instalada en macOS | `~/Library/Application Support/CashFlow/cashflow.db` |

En las aplicaciones de escritorio se puede elegir otra ubicación desde la configuración de almacenamiento. Si se selecciona una base existente o se crea una en otra carpeta, esa ruta reemplaza a la predeterminada.

## Crear instaladores locales

```bash
# Desde macOS Apple Silicon
pnpm --filter @cashflow/desktop package:mac

# Desde Windows de 64 bits
pnpm --filter @cashflow/desktop package:win
```

Los instaladores quedan en `apps/desktop/dist/`. Cada comando construye también el binario Go correcto para su plataforma.

## Versiones y publicaciones

Se usa versionado semántico: `MAYOR.MENOR.PARCHE`.

- `MAYOR`: cambios incompatibles.
- `MENOR`: funciones nuevas compatibles.
- `PARCHE`: correcciones compatibles.

La versión de la app vive en `apps/desktop/package.json`. Para publicar, actualizala, confirmá los cambios y creá un tag que coincida exactamente:

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions validará la coincidencia entre tag y versión, compilará en macOS Apple Silicon y Windows x64, y creará la Release con ambos instaladores. Crear un commit no publica nada: solamente el tag activa este flujo.

## Documentación

La guía de uso está en [docs/README.md](docs/README.md). Las capturas de cada pantalla se incorporarán al cerrar el diseño de esas secciones, para que la documentación refleje el producto final.

## Seguridad

Las contraseñas y códigos de recuperación se almacenan con hash Argon2id. Las exportaciones CSV excluyen credenciales y secretos. Conservá el código de recuperación de un solo uso en un gestor de contraseñas.
