# PRD — Termino

## 1. Información general

| | |
|---|---|
| **Producto** | Termino |
| **Tipo** | Tablero de tareas en la terminal (TUI) con servidor MCP y widgets |
| **Estado** | Borrador para la demo de la charla "Agentes de IA con Kiro + Gentle-AI" |
| **Alcance actual** | Solo local, un usuario, Linux |
| **Documento técnico** | [termino-trd.md](./termino-trd.md) |

## 2. Resumen ejecutivo

**Problema.** Quien programa en la terminal tiene que salir de ella para ver y mover sus tareas en un tablero web. Además, los agentes de IA que trabajan en el repositorio no pueden leer ni actualizar ese tablero, así que el estado de las tareas queda desactualizado.

**Solución.** Termino es el tablero donde ves trabajar a tu agente. Es un tablero Kanban o Scrum que vive en la terminal: se usa con el teclado desde una TUI, se resume en un widget al abrir una terminal nueva y expone un servidor MCP para que cualquier agente consulte y modifique las tareas. Importa el plan que genera el agente y muestra en vivo cómo avanzan las tarjetas.

**Qué lo hace distinto.**

1. **Agentes como compañeros de equipo.** Las tarjetas se mueven solas mientras el agente trabaja y cada una muestra qué agente la tiene.
2. **Pensado para Kiro, abierto a todos.** Trae integración a medida con Kiro y Gentle-AI, y funciona con cualquier agente que hable MCP (Claude Code, Codex, OpenCode y otros).
3. **Se instala y se quita con un comando.** Sin restos en el sistema.
4. **Con personalidad.** Temas visuales, entre ellos uno con la estética de SpookyThon.

**Criterios de éxito (objetivos del MVP).**

1. Crear un proyecto, elegir su metodología y agregar la primera tarea toma menos de 60 segundos sin leer documentación.
2. El widget se dibuja en menos de 100 ms al abrir una terminal, con un tablero de 500 tareas.
3. Un agente conectado por MCP puede listar, crear, mover y asignar tareas sin tocar archivos del proyecto a mano.
4. La TUI responde a cada tecla en menos de 50 ms con un tablero de 1.000 tareas.
5. El 100 % de los cambios hechos por un agente queda registrado con su autor en el historial de la tarea.
6. Un cambio hecho por un agente se ve en la TUI abierta en menos de 1 segundo, sin recargar.
7. Conectar un agente compatible toma un solo comando, y `termino uninstall` deja el sistema sin ningún archivo ni línea de configuración creados por Termino.

## 3. Visión del producto

Que el tablero esté donde ya trabajas. Termino no compite con las herramientas de gestión de equipos grandes: es el tablero personal y de equipos pequeños que se abre con un comando, se ve bien en cualquier terminal y que tu agente de IA también entiende.

## 4. Usuarios

| Persona | Qué necesita |
|---|---|
| **Desarrollador individual** | Ver qué tiene pendiente sin salir de la terminal y mover tareas con el teclado. |
| **Equipo pequeño de hackathon** (1 a 3 personas) | Repartir tareas, ver quién tiene qué y saber qué falta para la entrega. |
| **Agente de IA** (Kiro, Claude Code u otro cliente MCP) | Leer las tareas asignadas, marcarlas en progreso o terminadas y crear tareas nuevas a partir de un spec. |

## 5. Funcionalidad

### 5.1 Proyectos

- Como usuario, quiero crear un proyecto y elegir si su tablero es **Kanban** o **Scrum**, para trabajar con la metodología que prefiero.
  - Al crear el proyecto se pide nombre, clave corta (por ejemplo `TER`) y metodología.
  - Un proyecto Kanban nace con las columnas *Por hacer*, *En progreso* y *Hecho*.
  - Un proyecto Scrum nace con *Backlog* y un primer sprint vacío.
  - La metodología se puede cambiar después sin perder tareas.
- Como usuario, quiero cambiar de proyecto desde la TUI, para manejar varios a la vez.

### 5.2 Tablero Kanban

- Como usuario, quiero ver las columnas lado a lado y mover una tarea con el teclado, para actualizar el estado sin usar el mouse.
  - Las columnas se pueden crear, renombrar, reordenar y eliminar.
  - Cada columna admite un límite de trabajo en curso (WIP) opcional; al superarlo, la columna se marca visualmente y con texto.

### 5.3 Tablero Scrum

- Como usuario, quiero un backlog y sprints con fecha de inicio y fin, para planificar por iteraciones.
  - Las tareas se mueven del backlog al sprint activo.
  - Cada tarea admite una estimación en puntos.
  - El tablero del sprint muestra puntos totales, puntos terminados y días restantes.
  - Al cerrar un sprint, las tareas sin terminar vuelven al backlog o pasan al siguiente sprint, a elección del usuario.

### 5.4 Tareas

- Como usuario, quiero crear una tarea en una sola línea, para no interrumpir lo que estoy haciendo.
  - Una tarea tiene título, descripción, estado, prioridad, etiquetas, responsable, fecha límite y estimación.
  - Cada tarea tiene un identificador legible (`TER-12`).
  - Se puede filtrar por responsable, etiqueta, prioridad y texto.
  - Cada tarea guarda un historial de cambios con autor (persona o agente) y fecha.

### 5.5 Responsables

- Como usuario, quiero asignar una tarea a una persona o a un agente, para saber quién la tiene.
  - Los miembros del proyecto se definen localmente con un nombre y un tipo (persona o agente).
  - La vista "Mis tareas" muestra lo asignado al usuario actual en todos los proyectos.

### 5.6 TUI

- Como usuario, quiero una interfaz clara y atractiva en la terminal, para que usarla sea agradable.
  - Navegación completa con teclado; atajos al estilo Vim (`h j k l`) y con flechas.
  - Pantalla de ayuda con todos los atajos (`?`).
  - Se adapta al tamaño de la terminal desde 80×24; por debajo de ese tamaño muestra una vista de una sola columna.
  - Respeta el tema claro u oscuro de la terminal y la variable `NO_COLOR`.
  - El estado nunca se comunica solo con color: siempre hay un texto o símbolo.

### 5.7 Widget de terminal

- Como usuario, quiero ver un resumen de mis tareas cada vez que abro una terminal, para empezar sabiendo qué sigue.
  - El widget muestra el proyecto activo, mis tareas asignadas, lo que está en progreso y lo que vence pronto.
  - Se ajusta al ancho de la terminal y nunca ocupa más de un tercio de su alto.
  - Es de solo lectura y no bloquea el inicio de la terminal.
  - Se instala y desinstala con un comando para bash, zsh y fish.
  - Disponible solo en Linux en esta versión.

### 5.8 Servidor MCP

- Como agente de IA, quiero herramientas para consultar y modificar el tablero, para mantenerlo al día mientras trabajo.
  - Herramientas de lectura: listar proyectos, ver un tablero, listar y ver tareas, ver "mis tareas".
  - Herramientas de escritura: crear, editar, mover y asignar tareas; agregar comentarios.
  - Las acciones destructivas (eliminar tareas, columnas o proyectos) exigen una confirmación explícita en la llamada.
  - Todo cambio hecho por un agente queda en el historial con el nombre del agente.

### 5.9 Agentes en vivo

- Como usuario, quiero ver en mi tablero lo que hace el agente mientras trabaja, para seguir su avance sin leer el chat.
  - La TUI abierta refleja los cambios de cualquier agente en menos de 1 segundo.
  - La tarjeta que un agente tomó muestra su nombre y la marca "trabajando".
  - Un panel de actividad lista los últimos cambios con autor y hora.
  - El filtro "por agente" muestra solo las tarjetas de un agente.
- Como usuario, quiero pasarle una tarea a un agente y recuperarla, para repartir el trabajo entre personas y agentes.

### 5.10 Importar el plan del agente

- Como usuario, quiero convertir en tarjetas el plan que ya generó mi agente, para no copiar tareas a mano.
  - `termino import` lee una lista de tareas en Markdown y crea una tarjeta por cada ítem.
  - Formatos admitidos: `tasks.md` de un spec de Kiro, el documento de tareas de Gentle-AI (`odd/tasks/<feature>.md`) y cualquier lista de casillas en Markdown.
  - La importación se puede repetir sin duplicar tarjetas.
  - Cuando el agente marca una casilla en el archivo, la tarjeta pasa a *Hecho*; cuando la tarjeta se mueve a *Hecho*, la casilla se marca.

### 5.11 Integración con agentes

Termino no depende de ningún agente: todo pasa por MCP. Sobre esa base hay integraciones a medida.

| Agente | Qué incluye |
|---|---|
| **Kiro** (integración principal) | Conexión MCP con un comando, importación y sincronización del `tasks.md` de cada spec, y un archivo de steering que le indica al agente mantener el tablero al día. |
| **Gentle-AI** | Importación y sincronización de `odd/tasks/`, y una skill que le enseña al agente a usar el tablero dentro de su flujo ODD o SDD. Funciona en cualquier agente que Gentle-AI configure. |
| **Claude Code, Codex, OpenCode** | Conexión MCP con un comando. |
| **Cualquier otro cliente MCP** | Conexión manual con `termino mcp`. |

- `termino connect <agente>` deja la conexión lista y `termino disconnect <agente>` la quita.
- Agregar un agente nuevo no cambia el tablero ni las tareas.

### 5.12 Temas

- Como usuario, quiero elegir un tema, para que el tablero se vea como me gusta.
  - Temas incluidos: *default* y *spooky* (naranja y violeta, con el fantasma en ASCII).
  - Al mover una tarea a *Hecho* hay una celebración breve, que se puede desactivar.
  - Los temas son archivos de configuración; crear uno no requiere programar.

### 5.13 Instalación y desinstalación

- Como usuario, quiero instalar y quitar Termino sin dejar restos, para probarlo sin miedo.
  - Se instala con un solo comando y es un único binario.
  - `termino setup` guía el primer uso: detecta los agentes instalados, ofrece conectarlos y ofrece instalar el widget.
  - `termino uninstall` quita el widget, las conexiones con agentes y los archivos de steering o skills que Termino creó. Los datos se conservan salvo que se pida borrarlos con `--purge`.
  - Antes de modificar un archivo de configuración ajeno, Termino guarda una copia.
  - `termino doctor` revisa que todo esté bien conectado y explica cómo corregir lo que falla.

## 6. Requisitos del sistema de IA

- **Herramientas necesarias:** un cliente MCP en el agente (Kiro lo incluye) y el binario de Termino en el `PATH`.
- **Evaluación:** un conjunto de 20 pedidos en lenguaje natural ("mueve TER-4 a en progreso", "crea tres tareas a partir de este spec") ejecutados contra un tablero de prueba. El objetivo es que al menos 18 de 20 dejen el tablero en el estado esperado, verificado leyendo la base de datos.

## 7. Requisitos no funcionales

| Área | Requisito |
|---|---|
| **Privacidad** | Los datos se guardan solo en el equipo del usuario. Sin telemetría y sin conexiones de red en esta versión. |
| **Seguridad** | El servidor MCP no abre puertos; solo habla por la entrada y salida estándar del proceso que lo lanzó. Los archivos de datos son legibles solo por el usuario. |
| **Rendimiento** | Ver los criterios 2 y 4 de la sección 2. |
| **Portabilidad** | Un solo binario sin dependencias externas. Linux en esta versión. |
| **Confiabilidad** | La TUI, el widget y el servidor MCP pueden usar la misma base de datos al mismo tiempo sin corromperla. |
| **Extensibilidad** | Agregar un nuevo canal (web, Telegram) o una nueva metodología no debe exigir cambios en el dominio existente. |

## 8. Fuera de alcance

- Sincronización entre equipos o en la nube.
- Cuentas, inicio de sesión o permisos por usuario.
- Interfaz web o móvil.
- Notificaciones y conexión con Telegram.
- Widgets para macOS y Windows.
- Reportes avanzados (burndown, velocidad histórica).
- Integración con GitHub, Jira u otros gestores.

## 9. Hoja de ruta

| Fase | Contenido |
|---|---|
| **MVP (demo)** | Proyectos, tablero Kanban, tareas, TUI en vivo, servidor MCP, conexión e importación para Kiro, widget (bash y zsh), tema *spooky*, `setup` y `uninstall`. |
| **v1.1** | Tablero Scrum completo (sprints, puntos), integración con Gentle-AI, conexión para Claude Code, Codex y OpenCode, widget para fish, filtros guardados. |
| **v1.2** | Notificaciones locales y conexión con Telegram. |
| **v2.0** | Sincronización entre equipos, interfaz web y widgets para macOS y Windows. |

## 10. Riesgos

| Riesgo | Mitigación |
|---|---|
| Un agente borra o desordena tareas por error. | Confirmación explícita en acciones destructivas, historial por tarea y borrado lógico recuperable. |
| El widget hace lento el inicio de la terminal. | Presupuesto de 100 ms, consulta de solo lectura y salida silenciosa si se excede el tiempo. |
| La TUI se ve mal en terminales con pocos colores o fuentes sin símbolos. | Degradación a 16 colores y a caracteres ASCII. |
| Los agentes cambian el formato de sus archivos de configuración o de tareas. | Cada integración vive en un adaptador aislado con sus propias pruebas; si un formato no se reconoce, Termino avisa y no modifica nada. |
| La desinstalación borra algo que no era de Termino. | Un registro de instalación anota cada archivo y bloque creado; solo se quita lo que figura ahí. |
| El alcance crece antes de la demo. | El MVP excluye Scrum completo; la metodología se modela desde el inicio para agregarlo sin rehacer. |

## 11. Preguntas abiertas

1. ¿El usuario actual se toma del sistema (`$USER`) o se configura en el primer uso?
2. ¿Un proyecto se asocia a una carpeta o repositorio, para que el widget muestre el proyecto según dónde se abre la terminal?
3. ¿Las columnas de un tablero Scrum son fijas o editables como en Kanban?
