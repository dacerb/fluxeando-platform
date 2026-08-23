# Sistema de color de Fluxeando

La interfaz usa roles semánticos de Material 3. Los componentes no deben elegir
hexadecimales; consumen variables `--ui-*`, que se resuelven desde el esquema
`--m3-*` activo.

## Inventario inicial

El relevamiento encontró **335 valores hexadecimales únicos** en el frontend:
**159** corresponden a TypeScript (incluidas las definiciones de paleta) y
**223** aparecen en CSS, con solapamientos entre ambos conjuntos. También había
**407 referencias** de color directo en CSS, producto de iteraciones visuales
anteriores. A partir de esta normalización, los colores directos quedan
limitados a los esquemas Material y a fallbacks de arranque; las superficies,
controles y estados consumen roles semánticos. La UI debe usar los siguientes
roles:

| Intención | Variable |
| --- | --- |
| Fondo de aplicación | `--ui-canvas` |
| Superficie y contenedores | `--ui-surface*` |
| Texto principal, secundario y deshabilitado | `--ui-text*` |
| Borde normal y destacado | `--ui-border*` |
| Acción principal | `--ui-accent`, `--ui-on-accent` |
| Selección y hover | `--ui-selected`, `--ui-hover` |
| Error | `--ui-danger*` |
| Foco | `--ui-focus-ring` |
| Navegación lateral | `--ui-sidebar`, `--ui-on-sidebar` |

## Paletas completas

Cada paleta define roles completos para claro y oscuro: primary, secondary,
tertiary, error, background, surface, surface containers, on-colors, outline e
inverse colors. La fuente de verdad es `materialSchemes` en
`apps/desktop/src/main.tsx`.

| Paleta | Claro | Oscuro | Uso visual |
| --- | --- | --- | --- |
| Grafito | neutros nacarados y gris grafito | carbón con acento frambuesa suave | opción sobria predeterminada |
| Océano | blanco azulado y petróleo | grafito azulado y cian | opción fría y técnica |
| Violeta | blanco lavanda y violeta | carbón violáceo y lila | opción expresiva |

La selección de una paleta cambia simultáneamente fondo, superficies, texto,
bordes, acento, hover, selección, foco, controles, calendario y login. No se
debe usar un color de otra paleta como excepción.
