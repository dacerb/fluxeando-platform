# Despliegue web autoalojado en un VPS

Esta guía instala una instancia web de FLUXeando con MySQL, HTTPS automático y MCP remoto opcional. La aplicación de escritorio continúa usando SQLite local y no se modifica.

## Arquitectura

```text
Internet → Nginx (80/443) → frontend web
                         ├→ API: /v1 y /health
                         └→ MCP: /mcp → backend → MySQL
```

Nginx es el único servicio publicado. El frontend, backend, MySQL y el programador de backups se comunican por una red Docker privada. No se exponen los puertos 3306 ni 8787.

Los volúmenes persistentes son `fluxeando_mysql_data`, `fluxeando_backups`, `fluxeando_letsencrypt` y `fluxeando_certbot_webroot`. No los elimines: contienen la base, las copias y los certificados.

## Requisitos

- VPS Linux con Docker Engine y Docker Compose v2.
- Dominio o subdominio con registro A (y AAAA sólo si IPv6 está configurado) apuntando al VPS.
- Puertos TCP 80 y 443 permitidos en el proveedor y firewall.
- Ningún servicio ocupando esos puertos.
- Acceso SSH con permisos para ejecutar Docker.

## Prueba local con Podman

Para validar la pila sin dominio público, agregá `127.0.0.1 fluxeando.test` a tu archivo hosts, prepará los secretos y generá un certificado local:

```bash
cp deploy/manifest.example.yaml deploy/manifest.yaml
node deploy/prepare-manifest.mjs
./deploy/prepare-local-tls.sh fluxeando.test
podman compose --env-file deploy/.env -f compose.yaml -f compose.local.yaml up -d --build
```

Abrí `https://fluxeando.test:8443`. El navegador advertirá que el certificado es autofirmado; es normal en esta prueba local. Para detenerla conservando datos: `podman compose --env-file deploy/.env -f compose.yaml -f compose.local.yaml down`.

## 1. Preparar el código

Cloná el repositorio en el VPS y entrá en su raíz. No copies una base de datos ni secretos de otra instancia salvo que estés realizando una migración planificada.

```bash
git clone https://github.com/dacerb/fluxeando-platform.git
cd fluxeando-platform
cp deploy/manifest.example.yaml deploy/manifest.yaml
```

Editá `deploy/manifest.yaml`. Este archivo define la instalación, no el código: cada persona que aloje FLUXeando establece su propio dominio, correo, horario y retención de backups.

```yaml
instance:
  name: fluxeando-produccion
public:
  domain: fluxeando.tudominio.com
  letsencrypt_email: operaciones@tudominio.com
database:
  name: fluxeando
  user: fluxeando_app
backups:
  time: "02:30"
  retention_days: 30
  timezone: America/Argentina/Buenos_Aires
```

No subas `deploy/manifest.yaml`, `deploy/.env` ni `deploy/secrets/*.txt` a Git.

## 2. Generar configuración y secretos

El instalador valida el manifiesto, genera `deploy/.env` y crea dos contraseñas MySQL aleatorias, distintas y locales. Requiere Node 22 o superior sólo durante este paso.

```bash
node deploy/prepare-manifest.mjs
chmod 600 deploy/manifest.yaml deploy/.env deploy/secrets/*.txt
```

Si no tenés Node instalado en el VPS, podés ejecutar el mismo paso con Docker:

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD":/workspace -w /workspace node:22-alpine node deploy/prepare-manifest.mjs
```

El instalador se detiene si ya existe `deploy/.env`; esto evita sobrescribir por accidente una instancia ya configurada.

## 3. Iniciar la instancia

Comprobá primero que el DNS ya apunte al VPS. Después levantá los servicios:

```bash
docker compose --env-file deploy/.env up -d --build
docker compose --env-file deploy/.env ps
docker compose --env-file deploy/.env logs -f nginx certbot backend mysql-backup
```

En la primera emisión Nginx responde el desafío de Let’s Encrypt por HTTP y mantiene la aplicación en `503` hasta que exista un certificado válido. Cuando Certbot termina, Nginx habilita HTTPS y redirige HTTP a HTTPS. Certbot intenta renovar cada 12 horas y Nginx recarga su configuración periódicamente.

Abrí `https://TU_DOMINIO/` y creá el administrador inicial. Verificá además:

```bash
curl -I https://TU_DOMINIO/health
docker compose --env-file deploy/.env ps
```

## 4. Activar MCP remoto

MCP es parte del backend y está disponible en `https://TU_DOMINIO/mcp`, pero permanece deshabilitado hasta configurarlo desde la aplicación:

1. Ingresá como administrador.
2. Abrí **Configuración → MCP**.
3. Activá agentes MCP y elegí exposición `remote`.
4. Creá una clave por agente, con el mínimo permiso necesario.
5. Configurá el agente con `https://TU_DOMINIO/mcp` y `Authorization: Bearer ...`.

La clave se muestra una vez. No la guardes en el manifiesto, archivos versionados ni chats.

## Backups y restauración

El servicio `mysql-backup` crea cada día, a la hora y zona horaria indicadas en el manifiesto, una copia lógica comprimida de MySQL en el volumen persistente `backups`. Elimina copias más antiguas que `retention_days`.

Para inspeccionarlas:

```bash
docker compose --env-file deploy/.env exec mysql-backup ls -lah /var/lib/fluxeando/backups/mysql
```

Además, desde Configuración → Backups se puede elegir la copia de seguridad funcional de FLUXeando. En un VPS, usá la ruta `/var/lib/fluxeando/backups/app` para que permanezca dentro del volumen permitido.

Una restauración debe probarse primero en un servidor aislado. Para restaurar una copia MySQL, detené el backend, elegí el archivo correcto y ejecutá la importación únicamente después de confirmar el destino:

```bash
gunzip -c BACKUP.sql.gz | docker compose --env-file deploy/.env exec -T mysql mysql -u root -p NOMBRE_BASE
```

La restauración requiere la contraseña root, que está en `deploy/secrets/mysql_root_password.txt`. No la pegues en el historial del terminal: el cliente MySQL la solicitará de forma interactiva.

Un volumen Docker protege ante recreaciones de contenedores, pero no ante pérdida completa del VPS. Copiá periódicamente el volumen de backups a un almacenamiento externo y probá una restauración.

## Actualizar sin perder datos

```bash
git pull --ff-only
docker compose --env-file deploy/.env up -d --build
docker compose --env-file deploy/.env ps
```

No ejecutes `docker compose down -v`: la opción `-v` elimina los volúmenes y con ellos base, certificados y copias. Antes de una actualización importante, verificá que exista un backup reciente.

## Diagnóstico inicial

- Si Let’s Encrypt falla, comprobá DNS, puertos 80/443 y que otro proxy no los esté usando.
- Si la web no responde, revisá `nginx`, `web` y `backend` con `docker compose ... logs`.
- Si MySQL no inicia, verificá los secretos y el estado del volumen; no lo borres para “resolver” el problema.
- Si MCP devuelve `404`, confirmá que esté habilitado y en modo `remote` desde Configuración.
- Si no aparece un backup, revisá `mysql-backup`, la hora `BACKUP_TIME`, la zona horaria `TZ` y que el servicio permanezca activo.
