# TRD — Termino

## 1. Información general

| | |
|---|---|
| **Producto** | Termino |
| **Documento de producto** | [termino-prd.md](./termino-prd.md) |
| **Lenguaje** | Go |
| **Plataforma** | Linux (esta versión) |
| **Distribución** | Un solo binario: `termino` |
| **Alcance actual** | Local, un usuario, sin red |

## 2. Objetivo técnico

Construir un único binario en Go que ofrezca tres formas de entrar a los mismos datos: una TUI interactiva, un widget de solo lectura y un servidor MCP. Las tres comparten los mismos casos de uso, de modo que una regla de negocio se escribe una sola vez. El diseño debe permitir sumar más adelante una API web, notificaciones y Telegram sin modificar el dominio.

## 3. Stack

| Capa | Elección | Motivo |
|---|---|---|
| Lenguaje | Go (última versión estable) | Binario único, arranque rápido, buena concurrencia. |
| TUI | Bubble Tea, Bubbles y Lip Gloss | Modelo Elm, componentes listos y estilos con degradación de color. |
| CLI | Cobra | Subcomandos y autocompletado para bash, zsh y fish. |
| Base de datos | SQLite con `modernc.org/sqlite` | Sin CGO: el binario sigue siendo único y fácil de compilar. |
| Migraciones | Archivos SQL embebidos con `embed` | Se aplican solas al iniciar; no hay herramienta externa. |
| MCP | SDK oficial de Go para MCP, transporte stdio | Compatible con Kiro y otros clientes; no abre puertos. |
| Configuración | TOML en `$XDG_CONFIG_HOME/termino/config.toml` | Legible y editable a mano. |
| Vigilancia de archivos | `fsnotify` | Detecta cambios en los archivos de tareas de los agentes. |
| Registro | `log/slog` a archivo | Nunca a la salida estándar: rompería la TUI y el protocolo MCP. |
| Pruebas | `testing`, `teatest` y archivos golden | Dominio con tablas de casos; TUI con salidas esperadas. |

> Las versiones exactas de las dependencias se fijan en `go.mod` al iniciar el proyecto.

## 4. Arquitectura

Clean Architecture organizada **por feature**. Cada feature es una carpeta autocontenida con sus cuatro capas; las dependencias siempre apuntan hacia el dominio.

```
presentation ──▶ application ──▶ domain ◀── infrastructure
 (TUI, MCP,       (casos de       (entidades,     (SQLite, reloj,
  widget, CLI)      uso)           reglas, puertos)  generador de IDs)
```

Reglas:

1. `domain` no importa nada fuera de la biblioteca estándar.
2. `application` depende solo de `domain` y define los casos de uso.
3. `infrastructure` implementa los puertos (interfaces) declarados en `domain`.
4. `presentation` llama casos de uso; nunca toca la base de datos.
5. Una feature no importa las capas internas de otra; se comunican por interfaces públicas o por eventos de dominio.

## 5. Estructura de carpetas

```
termino/
├── cmd/termino/main.go            # arma las dependencias y ejecuta la CLI
├── internal/
│   ├── features/
│   │   ├── project/               # proyectos y metodología
│   │   │   ├── domain/
│   │   │   ├── application/
│   │   │   ├── infrastructure/
│   │   │   └── presentation/      # tui/, mcp/, cli/
│   │   ├── board/                 # columnas, límites WIP, movimiento
│   │   ├── task/                  # tareas, etiquetas, comentarios, historial
│   │   ├── sprint/                # sprints y estimaciones (Scrum)
│   │   ├── member/                # responsables: personas y agentes
│   │   ├── widget/                # resumen de solo lectura
│   │   ├── agent/                 # presencia y actividad de agentes
│   │   ├── tasksource/            # importar y sincronizar listas de tareas
│   │   │   └── infrastructure/    # kirospec/, gentleodd/, markdown/
│   │   ├── connector/             # conectar y desconectar agentes
│   │   │   └── infrastructure/    # kiro/, claudecode/, codex/, opencode/
│   │   └── setup/                 # setup, uninstall, doctor, registro de instalación
│   ├── platform/
│   │   ├── config/                # rutas XDG y config.toml
│   │   ├── sqlite/                # conexión, migraciones, transacciones
│   │   ├── events/                # bus de eventos de dominio en proceso
│   │   └── logging/
│   └── app/
│       ├── tui/                   # modelo raíz de Bubble Tea, tema, atajos
│       └── mcp/                   # servidor MCP y registro de herramientas
├── migrations/                    # *.sql embebidos
└── go.mod
```

## 6. Modelo de dominio

| Entidad | Campos principales | Reglas |
|---|---|---|
| **Project** | id, key, name, methodology, createdAt | `key` única, de 2 a 5 letras mayúsculas. |
| **Column** | id, projectId, name, position, wipLimit | La posición es única por proyecto. |
| **Task** | id, projectId, number, title, description, columnId, sprintId, priority, assigneeId, dueDate, points, position, deletedAt | El identificador visible es `key-number` (`TER-12`). |
| **Sprint** | id, projectId, name, startsAt, endsAt, state | Solo un sprint activo por proyecto. |
| **Member** | id, name, kind (`human` o `agent`) | El nombre es único. |
| **Label** | id, projectId, name, color | |
| **Comment** | id, taskId, authorId, body, createdAt | |
| **Activity** | id, taskId, actorId, action, payload, createdAt | Solo se agregan filas; nunca se editan. |

### Metodología como estrategia

La diferencia entre Kanban y Scrum se concentra en una interfaz del dominio:

```go
type Methodology interface {
    Kind() Kind                       // kanban | scrum
    DefaultColumns() []ColumnTemplate // columnas con las que nace el proyecto
    CanMove(task Task, to Column, board Board) error
}
```

Agregar otra metodología es agregar una implementación; los casos de uso y la TUI no cambian.

## 7. Persistencia

- **Ubicación:** `$XDG_DATA_HOME/termino/termino.db` (por defecto `~/.local/share/termino/`).
- **Modo WAL** y `busy_timeout`, para que la TUI, el widget y el servidor MCP lean y escriban a la vez sin bloquearse.
- **Claves foráneas activadas** y transacciones por caso de uso.
- **Consultas siempre parametrizadas**; no se arma SQL concatenando texto.
- **Borrado lógico** en tareas (`deleted_at`), para poder recuperar lo que un agente elimine por error.
- **Orden de tarjetas** con posiciones fraccionarias, para mover una tarea sin renumerar toda la columna.
- **Índices:** tareas por `(project_id, column_id, position)` y por `assignee_id`.

Los repositorios son interfaces en `domain`; `infrastructure` trae la implementación en SQLite. Cambiar a PostgreSQL más adelante es escribir otra implementación.

## 8. Casos de uso principales

| Feature | Casos de uso |
|---|---|
| project | CreateProject, ListProjects, ChangeMethodology, ArchiveProject |
| board | GetBoard, AddColumn, RenameColumn, ReorderColumns, SetWipLimit |
| task | CreateTask, UpdateTask, MoveTask, AssignTask, AddComment, DeleteTask, RestoreTask, ListTasks, MyTasks |
| sprint | CreateSprint, StartSprint, CloseSprint, AddTaskToSprint |
| widget | GetSummary |

Cada caso de uso recibe un `Actor` (quién ejecuta la acción) y lo registra en `Activity`.

## 9. Interfaces de entrada

### 9.1 CLI

| Comando | Función |
|---|---|
| `termino` | Abre la TUI. |
| `termino project new` | Crea un proyecto y pregunta la metodología. |
| `termino add "título"` | Crea una tarea sin abrir la TUI. |
| `termino widget` | Imprime el resumen. |
| `termino widget install` / `uninstall` | Agrega o quita el widget del archivo de inicio del shell. |
| `termino mcp` | Inicia el servidor MCP por stdio. |
| `termino import [ruta]` | Importa una lista de tareas; sin ruta, detecta las fuentes del proyecto. |
| `termino connect <agente>` / `disconnect <agente>` | Conecta o desconecta un agente. |
| `termino setup` | Asistente del primer uso. |
| `termino uninstall [--purge]` | Quita todo lo que Termino instaló. |
| `termino doctor` | Diagnostica la instalación y las conexiones. |
| `termino theme <nombre>` | Cambia el tema. |

### 9.2 TUI

- Un modelo raíz de Bubble Tea que enruta entre pantallas: selector de proyecto, tablero, detalle de tarea, backlog y ayuda.
- Cada pantalla es un submodelo dentro de la carpeta `presentation/tui` de su feature.
- Los estilos salen de un único tema en `internal/app/tui`; ningún componente define colores sueltos.
- Lip Gloss detecta el perfil de color de la terminal y degrada a 16 colores o sin color (`NO_COLOR`).
- Las operaciones de base de datos corren como comandos de Bubble Tea (`tea.Cmd`), fuera del ciclo de dibujo.

### 9.3 Widget

- `termino widget` abre la base de datos en solo lectura, ejecuta `GetSummary`, imprime y termina.
- Lee el tamaño con `golang.org/x/term` y elige entre tres diseños: compacto (menos de 60 columnas), normal y ancho (más de 120).
- Presupuesto de 100 ms: pasado ese tiempo, termina sin imprimir para no demorar la terminal.
- `widget install` agrega una línea delimitada por marcadores en `~/.bashrc`, `~/.zshrc` o `~/.config/fish/config.fish`, y `uninstall` quita solo ese bloque.
- El formateo vive detrás de una interfaz `Renderer`, para sumar otras plataformas sin tocar el caso de uso.

### 9.4 Servidor MCP

Transporte stdio. Herramientas:

| Herramienta | Tipo | Descripción |
|---|---|---|
| `list_projects` | lectura | Proyectos disponibles. |
| `get_board` | lectura | Columnas y tareas de un proyecto. |
| `list_tasks` | lectura | Tareas con filtros por responsable, etiqueta y estado. |
| `get_task` | lectura | Detalle, comentarios e historial. |
| `my_tasks` | lectura | Tareas asignadas al agente. |
| `create_task` | escritura | Crea una tarea. |
| `update_task` | escritura | Edita campos de una tarea. |
| `move_task` | escritura | Cambia de columna o de sprint. |
| `assign_task` | escritura | Asigna un responsable. |
| `claim_task` | escritura | El agente toma la tarea y la pasa a *En progreso*. |
| `complete_task` | escritura | El agente pasa la tarea a *Hecho*. |
| `add_comment` | escritura | Comenta una tarea. |
| `delete_task` | destructiva | Exige `confirm: true`; hace borrado lógico. |

Cada herramienta valida su entrada con un esquema y devuelve errores con un mensaje que el agente pueda corregir. El nombre del agente se toma de la información de cliente del protocolo, o de `--actor` al lanzar el servidor.

## 10. Agentes: en vivo, agnóstico e integrado

El núcleo no conoce a ningún agente. Todo lo específico vive en adaptadores detrás de tres interfaces del dominio, de modo que sumar un agente es agregar una carpeta.

### 10.1 Tablero en vivo

- La TUI consulta `PRAGMA data_version` de SQLite cada 500 ms. Si el valor cambió, otro proceso escribió y la pantalla vuelve a cargar solo el tablero visible.
- No hay demonio ni sockets: la base de datos es el único punto de encuentro entre la TUI, el widget y los servidores MCP.
- **Presencia.** Cada llamada MCP actualiza `agent_sessions(agent_id, last_seen_at, current_task_id)`. Un agente figura como "trabajando" si su última señal tiene menos de 60 segundos.
- La herramienta MCP `claim_task` asigna la tarea al agente y la mueve a *En progreso* en una sola transacción; `complete_task` la mueve a *Hecho*.

### 10.2 Fuentes de tareas

```go
type TaskSource interface {
    Name() string                          // "kiro-spec", "gentle-odd", "markdown"
    Detect(root string) ([]SourceFile, error)
    Parse(file SourceFile) ([]SourceTask, error)
    WriteStatus(file SourceFile, ref string, done bool) error
}
```

| Adaptador | Archivo que lee | Notas |
|---|---|---|
| `kirospec` | `.kiro/specs/<feature>/tasks.md` | Una tarjeta por casilla; la feature se guarda como etiqueta. |
| `gentleodd` | `odd/tasks/<feature>.md` | Usa los identificadores de tarea estables del documento. |
| `markdown` | Cualquier `.md` con casillas `- [ ]` | Adaptador genérico. |

- Cada tarjeta importada guarda `source`, `source_path` y `source_ref`. Con eso, importar de nuevo actualiza en lugar de duplicar.
- **Sincronización en los dos sentidos.** `fsnotify` vigila los archivos importados: una casilla marcada mueve la tarjeta a *Hecho*, y mover la tarjeta a *Hecho* marca la casilla. La escritura es atómica (archivo temporal y renombrado) y solo toca el carácter de la casilla.
- Si el archivo y la tarjeta cambiaron a la vez, gana el cambio más reciente y el conflicto queda en `Activity`.

### 10.3 Conectores de agentes

```go
type Connector interface {
    Agent() string
    Detect() (installed bool, err error)
    Connect(scope Scope) ([]InstalledItem, error) // scope: proyecto o usuario
    Disconnect(items []InstalledItem) error
}
```

| Conector | Qué escribe |
|---|---|
| `kiro` | Entrada de Termino en la configuración MCP de Kiro y `.kiro/steering/termino.md`. |
| `claudecode` | Entrada de Termino en la configuración MCP de Claude Code. |
| `codex` | Entrada de Termino en la configuración MCP de Codex. |
| `opencode` | Entrada de Termino en la configuración MCP de OpenCode. |
| Gentle-AI | Una skill `termino` (`SKILL.md`) en el directorio de skills del proyecto, además del adaptador `gentleodd`. |

> La ruta y el formato exactos de cada configuración se confirman contra la documentación vigente de cada agente al implementar su conector.

Reglas de todos los conectores:

1. Solo agregan o quitan **su propia entrada**; nunca reescriben el resto del archivo.
2. Guardan una copia del archivo antes de modificarlo.
3. Si el formato no es el esperado, no escriben nada e informan cómo conectar a mano.
4. Devuelven la lista de lo que crearon, que va al registro de instalación.

### 10.4 Instalación y desinstalación

- **Distribución:** binarios publicados con suma de verificación, `go install` y un script de instalación que verifica esa suma antes de copiar el binario a `~/.local/bin`.
- **Registro de instalación:** `$XDG_STATE_HOME/termino/installed.json` anota cada archivo creado y cada bloque agregado a un archivo ajeno (ruta, tipo, marcador y copia de respaldo).
- **`termino setup`:** detecta agentes con `Connector.Detect`, ofrece conectarlos, ofrece el widget y elige el tema.
- **`termino uninstall`:** recorre el registro en orden inverso y quita solo lo anotado. Conserva los datos; con `--purge` borra también la base de datos, la configuración y el registro. Al final indica cómo eliminar el binario.
- **`termino doctor`:** comprueba el binario en el `PATH`, los permisos de archivos, cada conexión de agente y el widget.

### 10.5 Temas

- Un tema es un archivo TOML con colores semánticos (`surface`, `accent`, `danger`, `done`) y opciones de bordes y símbolos. Los componentes solo usan esos nombres.
- Temas embebidos: `default` y `spooky`. Los del usuario van en `$XDG_CONFIG_HOME/termino/themes/`.
- La celebración al completar una tarea es una animación de menos de 1 segundo y se desactiva con `motion = false` o con `NO_COLOR`.

## 11. Seguridad

1. **Sin red.** El binario no abre puertos ni hace conexiones salientes. El servidor MCP solo usa stdio.
2. **Permisos de archivos.** Directorio de datos `0700`, base de datos y configuración `0600`.
3. **Validación en el borde.** Todo lo que entra por CLI, TUI o MCP se valida antes de llegar a un caso de uso: longitudes máximas, enumeraciones cerradas y caracteres de control eliminados.
4. **Salida segura.** Los textos de las tareas se limpian de secuencias de escape ANSI antes de dibujarse, para que una tarea creada por un agente no pueda manipular la terminal.
5. **SQL parametrizado** en todos los repositorios.
6. **Mínimo privilegio para agentes.** `termino mcp --read-only` expone solo las herramientas de lectura. Las destructivas exigen confirmación y nunca borran de forma física.
7. **Auditoría.** `Activity` registra quién hizo cada cambio y es de solo inserción.
8. **Archivos ajenos.** Los conectores y el widget solo tocan su propia entrada, con copia de respaldo previa y registro de cada cambio. La sincronización de tareas solo escribe en archivos que el usuario importó.
9. **Instalación verificable.** El script de instalación comprueba la suma de verificación del binario antes de instalarlo.
10. **Sin secretos.** Esta versión no guarda credenciales. Cuando llegue Telegram, el token irá al llavero del sistema, no a `config.toml`.

## 12. Extensibilidad

| Futuro | Punto de extensión ya previsto |
|---|---|
| **Notificaciones y Telegram** | Los casos de uso publican eventos de dominio (`TaskMoved`, `TaskAssigned`, `SprintClosed`) en `platform/events`. Un notificador es un suscriptor nuevo. |
| **Interfaz web** | Un adaptador HTTP en `presentation/http` que llama los mismos casos de uso. |
| **Sincronización entre equipos** | Los repositorios son interfaces; se agrega una implementación remota y autenticación en el adaptador. |
| **Widgets en otras plataformas** | Interfaz `Renderer` del widget. |
| **Nuevas metodologías** | Interfaz `Methodology`. |

## 13. Pruebas

| Nivel | Qué se prueba | Herramienta |
|---|---|---|
| Dominio | Reglas de movimiento, límites WIP, cierre de sprint | `testing` con tablas de casos |
| Aplicación | Casos de uso con repositorios en memoria | `testing` |
| Infraestructura | Repositorios contra SQLite en un directorio temporal | `testing` |
| TUI | Navegación y dibujo de pantallas | `teatest` y archivos golden |
| MCP | Cada herramienta con entradas válidas e inválidas | Cliente MCP en memoria |
| Widget | Los tres diseños según el ancho | Archivos golden |

Se aplica TDD: primero el test que falla, después la implementación, al final la limpieza. `go test ./...` y `go vet ./...` deben pasar antes de cada commit.

## 14. Rendimiento

| Medida | Objetivo |
|---|---|
| Inicio de la TUI | Menos de 200 ms |
| Respuesta a una tecla | Menos de 50 ms con 1.000 tareas |
| Widget completo | Menos de 100 ms con 500 tareas |
| Tamaño del binario | Menos de 30 MB |

## 15. Alcance técnico del MVP (demo)

1. Features `project`, `board`, `task` y `member` completas.
2. TUI con selector de proyecto, tablero Kanban, detalle de tarea, panel de actividad y ayuda, con refresco en vivo.
3. Servidor MCP con las trece herramientas y presencia de agentes.
4. `tasksource` con el adaptador `kirospec` (importación y sincronización en los dos sentidos) y el adaptador genérico `markdown`.
5. `connector` para Kiro; `setup`, `uninstall` y `doctor` con registro de instalación.
6. Temas `default` y `spooky`.
7. `termino widget` con sus tres diseños e instalación para bash y zsh.
8. Pasan a v1.1: el widget para fish, el adaptador `gentleodd` con su skill, los conectores de Claude Code, Codex y OpenCode, y la TUI de sprints. Sus interfaces (`Renderer`, `TaskSource`, `Connector`, `Methodology`) quedan definidas desde el MVP.

## 16. Decisiones abiertas

1. Cómo se determina el usuario actual: `$USER` o configuración en el primer uso.
2. Si un proyecto se asocia a una carpeta, para que el widget elija el proyecto según el directorio.
3. Si las columnas de un tablero Scrum son editables.
