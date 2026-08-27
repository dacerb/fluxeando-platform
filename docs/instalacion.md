# Instalación y primeros pasos

Volvé al [índice de documentación](README.md).

## Windows 64 bits

1. Descargá el instalador `.exe` de la última [Release](https://github.com/dacerb/fluxeando-platform/releases).
2. Ejecutalo y seguí el asistente de instalación.
3. Abrí FLUXeando desde el menú Inicio.

## macOS Apple Silicon

1. Descargá el archivo `.dmg` de la última Release.
2. Abrilo y arrastrá FLUXeando a la carpeta Aplicaciones.
3. Abrí la aplicación desde Aplicaciones.

## macOS: Gatekeeper

Por ahora CashFlow no está firmada con un certificado de Apple ni notarizada. Usá estos pasos únicamente si descargaste el archivo desde la [Release oficial](https://github.com/dacerb/fluxeando-platform/releases).

### Por qué macOS muestra el aviso

Al descargar una app desde Internet, macOS agrega el atributo de cuarentena al archivo. Gatekeeper revisa ese atributo al abrirla por primera vez. Normalmente, la firma identifica al desarrollador y la notarización confirma que Apple procesó la app; como esta versión todavía no cuenta con esas dos validaciones, Gatekeeper no puede verificar su procedencia y bloquea la apertura por precaución.

Quitar la cuarentena no firma ni modifica la aplicación: solamente indica a macOS que confiás en **esa copia local**. No desactives Gatekeeper globalmente y no ejecutes el comando en apps descargadas de fuentes desconocidas.

### Opción recomendada: aprobar una vez desde macOS

1. Intentá abrir la app una vez y cerrá el aviso de macOS.
2. Abrí **Configuración del Sistema > Privacidad y seguridad**.
3. En la parte inferior aparecerá el aviso de que macOS bloqueó CashFlow.
4. Elegí **Abrir de todos modos** y confirmá **Abrir**.

Como alternativa, mantené presionada la tecla Control, hacé clic en la app y elegí **Abrir**. No hace falta desactivar Gatekeeper para todo el sistema.

### Opción por Terminal: quitar la cuarentena de CashFlow

Usá este método si el aviso sigue apareciendo después de instalar la app en Aplicaciones.

1. Abrí **Terminal** desde Aplicaciones > Utilidades, o buscala con Spotlight (`⌘` + Espacio y escribí `Terminal`).
2. Copiá y pegá este comando. La ruta entre comillas es importante:

   ```bash
   xattr -dr com.apple.quarantine "/Applications/CashFlow.app"
   ```

3. Presioná Retorno. Si no aparece ningún mensaje, el comando se ejecutó correctamente.
4. Abrí CashFlow normalmente desde la carpeta Aplicaciones.

El comando elimina de forma recursiva el atributo `com.apple.quarantine` de los archivos internos de **esa** aplicación. Si Terminal informa que no tenés permisos, verificá que CashFlow esté en `/Applications` y que tu cuenta tenga permisos de administración. Como último recurso, ejecutá el mismo comando anteponiendo `sudo`; macOS pedirá la contraseña de tu cuenta y no mostrará caracteres mientras la escribís:

```bash
sudo xattr -dr com.apple.quarantine "/Applications/CashFlow.app"
```

## Primera ejecución

En el primer inicio, seleccioná o creá la base de datos local. La aplicación usa SQLite y conserva los datos en tu equipo.
