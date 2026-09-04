# Backups automáticos

Cada cambio confirmado reinicia una única tarea de backup. Si llega otro cambio antes de diez segundos, la tarea anterior se reemplaza; por lo tanto sólo se genera una copia diez segundos después del último cambio. Si no hay cambios, no se genera ninguna copia.

La configuración comienza desactivada: hasta que el administrador elige y guarda un destino en **Configuración → Copias de seguridad automáticas**, no se programa ni genera ninguna copia. El destino local incluye una carpeta y un prefijo; el nombre final usa `prefijo-AAAAMMDDTHHMMSSZ.json`.

- **Sistema de archivos:** en escritorio se selecciona una carpeta nativa. En una instalación web hospedada la ruta corresponde al filesystem del servidor, no al equipo del navegador. `CASHFLOW_BACKUP_ROOT` puede restringir las rutas permitidas.
- **Google Drive:** se indica el ID de carpeta. La autorización OAuth requiere registrar la aplicación de Google y configurar en el servidor el client ID, client secret y URL de retorno; esos secretos nunca se guardan ni se envían al navegador.

Las copias son archivos JSON portables con cuentas, categorías, movimientos, usuarios y auditoría. El estado de la última copia o el último error se muestra en la misma sección.
