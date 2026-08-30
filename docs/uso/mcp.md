# MCP y agentes

FLUXeando puede exponer un servidor MCP para que un agente autorizado consulte información financiera o registre movimientos bajo permisos controlados.

## Activación y claves

1. Ingresá con un usuario administrador y abrí **Configuración > MCP**.
2. Activá **Permitir agentes MCP**.
3. Elegí **Sólo local** para usarlo en la misma computadora. El modo remoto requiere HTTPS antes de exponerse.
4. Creá una clave con el permiso mínimo necesario. La aplicación genera el token y lo muestra una sola vez.

El token es Base64URL, incorpora su versión y fecha de creación, y contiene 32 bytes aleatorios. La base de datos guarda únicamente su hash Argon2id. No se puede crear ni editar manualmente desde la interfaz.

## Ejemplo de configuración

Usá la URL efectiva que muestra la aplicación. En desarrollo, el puerto puede cambiar en cada inicio.

```json
{
  "mcpServers": {
    "fluxeando": {
      "url": "http://127.0.0.1:8787/mcp",
      "headers": {
        "Authorization": "Bearer cf_mcp_eyJ2IjoxLCJjcmVhdGVkQXQiOiIyMDI2LTA4LTMwVDAzOjAwOjAwWiIsIm5vbmNlIjoiZXhhbXBsZSJ9"
      }
    }
  }
}
```

La clave del ejemplo no funciona. Reemplazala por la clave recién creada desde Configuración y no la compartas ni la incluyas en repositorios, capturas o registros.

## Permisos iniciales

- `read`: consultar cuentas y categorías.
- `read,write`: además permite registrar movimientos. Cada movimiento se valida y audita como una operación de CashFlow.

Podés revocar una clave inmediatamente desde Configuración. Al revocarla, las conexiones nuevas con esa clave se rechazan.
