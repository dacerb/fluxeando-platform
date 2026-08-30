# Despliegue web autoalojado

Esta instalación mantiene tres formas de uso compatibles:

- Aplicación de escritorio en macOS y Windows, con SQLite local.
- Desarrollo web desde el repositorio, con la API local.
- Web autoalojada en Linux con MySQL, HTTPS y MCP remoto opcional.

El archivo `compose.yaml` está diseñado para la tercera opción. No sustituye ni modifica los modos local o de escritorio.

## Arquitectura

Nginx es el único servicio publicado en Internet. Sirve la interfaz web y reenvía `/v1/`, `/health` y `/mcp` al backend. MySQL, la interfaz web y el backend permanecen en una red Docker interna, sin puertos publicados.

```text
Internet → Nginx (80/443) → web / backend → MySQL
                              └── /mcp
```

MCP forma parte del backend: no hay un proceso ni una contraseña adicional para ese servicio. Se habilita desde Configuración → MCP, se selecciona exposición `remote` y se crea una clave específica para cada agente.

## Requisitos previos

- Un VPS Linux con Docker Engine y Docker Compose v2.
- Un dominio, por ejemplo `fluxeando.tudominio.com`, con un registro A/AAAA que apunte al VPS.
- Puertos TCP 80 y 443 permitidos en el firewall y en el proveedor del VPS.
- Ningún otro servicio ocupando esos puertos.

## Preparar la configuración

Desde la raíz del proyecto:

```bash
cp deploy/.env.example deploy/.env
cp deploy/secrets/mysql_app_password.txt.example deploy/secrets/mysql_app_password.txt
cp deploy/secrets/mysql_root_password.txt.example deploy/secrets/mysql_root_password.txt
chmod 600 deploy/.env deploy/secrets/*.txt
```

Editá `deploy/.env` con el dominio y un correo válido. Reemplazá los dos archivos de `deploy/secrets/` con contraseñas distintas, aleatorias y de al menos 32 caracteres. Por ejemplo:

```bash
openssl rand -base64 36 > deploy/secrets/mysql_app_password.txt
openssl rand -base64 36 > deploy/secrets/mysql_root_password.txt
chmod 600 deploy/secrets/*.txt
```

Los archivos `.txt` y `.env` están ignorados por Git. No los copies a tickets, logs ni chats.

## Iniciar

```bash
docker compose --env-file deploy/.env up -d --build
docker compose --env-file deploy/.env ps
```

Durante la emisión inicial, Nginx sólo entrega el desafío de Let’s Encrypt; el resto devuelve `503` hasta que exista un certificado válido. Esto evita publicar la aplicación sin HTTPS. Certbot obtiene y renueva el certificado de forma periódica, y Nginx recarga su configuración para usarlo.

Comprobá el estado:

```bash
docker compose --env-file deploy/.env logs -f nginx certbot backend
curl -I https://TU_DOMINIO/health
```

El primer acceso a `https://TU_DOMINIO/` muestra la configuración inicial de FLUXeando. Creá el administrador desde allí.

## MCP remoto

1. Ingresá como administrador.
2. Abrí Configuración → MCP y activá agentes MCP.
3. Seleccioná `remote` únicamente si el dominio ya responde por HTTPS.
4. Creá una clave por agente, con un nombre reconocible y el menor permiso necesario.
5. Configurá el agente con `https://TU_DOMINIO/mcp` y el encabezado `Authorization: Bearer ...`.

Las llamadas MCP se autentican por clave, pasan por HTTPS, se limitan en Nginx y quedan en la auditoría con su conexión y origen. La clave no se almacena ni se registra en texto plano.

## Operación segura

- No publiques el puerto 3306 ni el 8787.
- Mantené Docker, la imagen de MySQL, Nginx y Certbot actualizados de forma planificada.
- Generá una clave MCP distinta para cada integración; revocala si se filtra o deja de usarse.
- Respaldá el volumen `fluxeando_mysql_data` antes de actualizar. Una copia lógica puede hacerse con `mysqldump` dentro del contenedor.
- Revisá `docker compose logs` y la auditoría de FLUXeando después de habilitar MCP remoto.
- Para restaurar, probá primero la copia de MySQL en un servidor aislado.

## Actualizar la aplicación

```bash
git pull
docker compose --env-file deploy/.env up -d --build
```

No ejecutes `docker compose down -v`: el modificador `-v` elimina los volúmenes y, con ellos, la base MySQL y los certificados.
