# termino

A local-first task board for the terminal. termino keeps projects, boards and
tasks in a single SQLite file on your machine and gives you three ways in:

- a **TUI** Kanban board with live refresh,
- a **CLI** for scripting and quick edits,
- an **MCP server** (stdio) so AI agents such as Kiro can read and update the
  same board you are looking at.

No accounts, no network: the binary opens no ports and makes no outbound
connections.

## Install

### Install script (Linux, macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/lesquel/termino/main/install.sh | sh
```

The script detects your OS (`linux`, `darwin`) and architecture (`amd64`,
`arm64`), resolves the latest release tag from the GitHub API, downloads the
matching archive plus `checksums.txt`, verifies the SHA-256 and refuses to
install on a mismatch. The binary goes to `$HOME/.local/bin`; if that is not
on your `PATH`, the script prints a note so you can add it.

| Variable | Default | Meaning |
|---|---|---|
| `TERMINO_VERSION` | `latest` | Release tag to install, e.g. `v0.3.2` |
| `TERMINO_INSTALL_DIR` | `$HOME/.local/bin` | Where the binary goes |

Windows is not covered by the script: download the `.zip` from the
[releases page](https://github.com/lesquel/termino/releases) and put
`termino.exe` on your `PATH`.

### Go

```sh
go install github.com/lesquel/termino/cmd/termino@latest
```

Requires Go 1.25+. No cgo is needed (`modernc.org/sqlite` is pure Go).

### Release archives

Every release publishes `termino_<version>_<os>_<arch>.tar.gz` (`.zip` on
Windows) for linux, darwin and windows on amd64 and arm64, and a
`checksums.txt` with SHA-256 sums. Download the archive for your platform,
verify it against `checksums.txt`, extract the `termino` binary and put it on
your `PATH`.

### First run

```sh
termino            # 1. first run: setup wizard, then the board
termino setup      #    (re-run any time to edit setup or add a project)
termino connect kiro   # 2. register the MCP server so Kiro can use the board
termino doctor     # 3. confirm the install, database and integrations are healthy
```

### Updating and uninstalling

Re-run the install script to update to the latest release (same command;
`TERMINO_VERSION` pins a specific tag). `go install ...@latest` updates a
Go-installed binary the same way.

```sh
termino uninstall           # remove integrations and registered files
termino uninstall --purge   # also delete the database, config and state directory
```

## Quick start

```sh
termino                                     # first run: setup wizard, then the board
termino setup                               # wizard: name, project, theme, widget
termino project new --name "Web app" --key WEB --associate-pwd
termino add Fix login redirect --priority high   # quotes are optional
termino ls                                  # list the tasks of the current project
termino                                     # open the board
termino widget install                      # status summary in bash/zsh prompt
termino connect kiro                        # register the MCP server in Kiro
```

### Setup

Running `termino` before any project exists starts the same wizard and opens
the board when it is done (without a terminal it prints a pointer to
`termino setup` instead). `termino setup` opens a full-screen wizard (8 steps, `esc`/`shift+tab` goes
back, `ctrl+c` quits without changing anything): your name (default `$USER`),
project name (default: the current directory's name), project key (suggested
from the name, e.g. "Web App" → `WA`, validated as you type), methodology
(kanban or scrum), whether to associate the current directory, a theme with a
live board preview, and the bash/zsh shell widget (fish is not supported yet
and is skipped). A summary shows every choice before anything is written.

Re-running it offers **Edit current setup**, **Add another project** or
**Cancel**. The current project is the one associated with the working
directory, as everywhere else. Editing updates it in place (name, methodology,
directory association) and never creates a duplicate; the key stays
read-only because existing task IDs (`KEY-N`) use it — add another project
for a new key.

Without a terminal, or with `--no-tui`, setup asks line by line with defaults
in brackets. For scripts, `--yes` applies flags plus defaults without asking;
every value is validated first, so a bad flag writes nothing:

```sh
termino setup --yes --name Ana --project "Web App" --key WA \
  --methodology scrum --theme spooky --no-associate --widget
```

With `--yes` on an existing setup it edits the current project, or the
project whose `--key` you pass; a `--key` no project uses yet adds a project.

### Board keys

| Key | Action |
|---|---|
| `h` `j` `k` `l` / arrows | Move between columns and cards |
| `H` / `L` (shift+arrows) | Move the selected card one column left/right |
| `m` | Move the card to a column (picker) |
| `a` | Assign the card to a member (picker) |
| `d` | Delete the card (asks y/N; soft delete) |
| `/` | Filter: free text, `assignee:`, `label:`, `priority:`, `agent:` (`esc` clears) |
| `n` | New task |
| `enter` | Task detail (`esc` or `enter` closes it) |
| `p` | Project selector |
| `s` | Settings: theme, shell widget, motion |
| `?` | Help: every key, by screen (`↑`/`↓` scroll) |
| `q` / `ctrl+c` | Quit |

The header names the project (`termino · Web app (WEB)`) and the active
filter; cards are shown as `KEY-N`. The footer always lists the main keys and
shortens itself on narrow terminals. Status messages appear on a fixed row
under the header and clear after about 3 seconds. An empty board says
"No tasks yet — press n to create one"; a filter with no results says so.

Moving a card into the last column (Done) plays a short celebration, unless
motion is off or `NO_COLOR` is set. Columns over their WIP limit are flagged.
The selected card is marked with `>`, so the selection stays visible without
color. Cards start with the theme's priority symbol (`▲` high, `!!!`
critical, `▼` low in the default theme; medium has none). Applying a filter
selects the first matching card. The new-task form (`n`) has a title, a priority (`←`/`→`) and
comma-separated labels; `tab` moves between fields and `enter` creates. The
task detail shows the assignee, labels and `KEY-N`; `e` edits and `ctrl+s`
saves (`enter` also saves from the title field, and inserts a newline in the
description). The project selector (`p`) marks the current project; `q` goes
back to the board. The built-in `CLI` and `System` members are not offered in
the assign picker.

The task detail, help, pickers, delete prompt, new-task form, project selector
and settings open as dialogs centered over the board, which stays visible
around them. Dialogs that do not fit the terminal scroll (`↑`/`↓`); with
`NO_COLOR` they are drawn with a border and plain text only.

Settings (`s`) changes four things without leaving the board, confirmed on a
status line (`✓ Theme set to nord`):

- **Theme** — the list previews the highlighted theme; `enter` applies it to
  the running board at once.
- **Shell widget** — shows the detected shell and its rc file; `enter`
  installs or removes the widget block, exactly like `termino widget
  install`/`uninstall` (fish is not supported yet).
- **Motion** — turns the Done celebration on or off (always off while
  `NO_COLOR` is set).
- **Share via repo** — shares the current project through the git
  repository of the working directory, or stops sharing it, like `termino
  share init`/`off` (see [Sharing with your team](#sharing-with-your-team)).

`↑`/`↓` move, `esc` goes back; other keys (`q`, `p`, `?`) do nothing while
settings is open.

### Connecting an agent (Kiro)

```sh
termino connect kiro       # adds a "termino" entry to ~/.kiro/settings/mcp.json
                           # and writes .kiro/steering/termino.md in the current dir
termino disconnect kiro    # removes only what connect added
```

The entry runs `termino mcp`. Thirteen tools are exposed: read tools
`list_projects`, `get_board`, `list_tasks`, `get_task`, `my_tasks`, and write
tools `create_task`, `update_task`, `move_task`, `assign_task`, `claim_task`,
`complete_task`, `add_comment`, `delete_task`. `termino mcp --read-only`
exposes only the read tools; `--actor <name>` sets the agent identity
(default: the MCP client name).

### Widget

`termino widget` prints a short summary of the current project when a shell
starts (after `termino widget install`):

```text
Web app (WEB)
Backlog 3 · In Progress 1 · Done 2
In progress
  WEB-4  Fix login redirect
My tasks
  WEB-7  Write the release notes · 2d
→ termino board
```

It lists what is in progress, your tasks (assigned to `user_name`) and open
tasks due within a week. When nothing is in progress or assigned it shows the
next tasks of the first column, so open work is never hidden; a project
without tasks shows `No tasks yet — termino add "..."`. Wide terminals add
due-date (only when a listed task has one) and column fields; terminals
narrower than 60 columns get a three-line version. Colors follow `NO_COLOR`
and are dropped when the output is not a terminal. The summary is read-only
and gives up silently after 100 ms.

### Sharing with your team

Share a board through the git repository you already use; there is no
server and no account. Termino keeps plain-text files under `.termino/`
(`project.toml` plus one TOML file per task) in sync with the board, and git
carries them between people. Termino never commits, pushes or pulls.

```sh
cd my-repo
termino share init              # writes .termino/ at the git root
git add .termino && git commit -m "chore: share the termino board" && git push
```

Every change you make (TUI, CLI or an agent over MCP) rewrites the affected
files. Teammates clone or pull the repository and run `termino`: the board is
created (or their existing project with the same key is linked and merged by
task) and every command, the open board and the MCP server pick up the files
git brought in. Comments and activity history stay local.

- **Conflicts**: edits to different tasks never conflict (one file per
  task). If two people edit the same task, git reports a conflict in that
  file; until you resolve it the file is skipped, the board's status line
  warns `⚠ 1 shared file skipped — termino share status`, and everything else
  keeps working. Two tasks created at once with the same number are
  renumbered the same way on every machine (the later one moves).
- `termino share status` shows the repository, the file count, the last
  import and any skipped file with the reason.
- `termino share off` stops syncing; the files stay in the repository.
- The settings screen (`s` on the board) has the same on/off toggle, and
  `termino setup` offers it inside a git repository (`--share` without a
  terminal).

## CLI reference

Run `termino <command> --help` for full flags.

| Command | Description |
|---|---|
| `termino` | Open the TUI |
| `termino board` | Open the Kanban board (TUI) |
| `termino setup [--name --project --key --methodology --theme --associate\|--no-associate --widget --share --yes --no-tui]` | Setup wizard; re-run to edit the current setup or add a project (see [Setup](#setup)) |
| `termino project new [--name --key --methodology kanban\|scrum --path --associate-pwd]` | Create a project |
| `termino project list` | List projects (current marked `*`) |
| `termino project use <key>` | Make a project the current one |
| `termino add <title...> [--column --description --priority --label]` | Add a task to the current project; every argument is part of the title; `--label` (repeatable) creates missing labels |
| `termino task list [--column --assignee --label --priority]` / `termino ls` | List the current project's tasks column by column (`--assignee none` = unassigned) |
| `termino task move <task> <column>` | Move a task |
| `termino task assign <task> <member\|none>` | Assign or unassign |
| `termino task done <task>` | Move a task to Done (the column named "Done", else the last column) |
| `termino task sprint <task> <sprint>` | Put a task in a sprint |
| `termino my-tasks [--member]` | Tasks assigned to you across projects |
| `termino member add <name> [--kind human\|agent]` / `list` / `remove <member>` | Manage members |
| `termino column add <name>` / `list` / `rename` / `reorder` / `delete` / `wip <column> <limit>` | Manage columns (`wip … 0` removes the limit) |
| `termino sprint new <name> [--start --end]` / `list` / `start <sprint>` / `close <sprint> [--unfinished backlog\|next_sprint]` / `add <sprint> <task>...` | Manage sprints |
| `termino import [path] [--watch]` | Import Markdown checkbox tasks (Kiro `tasks.md`, `odd/tasks/*.md`, any `- [ ]` list); `--watch` keeps syncing |
| `termino widget` | Print the project summary (see [Widget](#widget)) |
| `termino widget install [--shell bash\|zsh]` / `uninstall` | Add/remove the widget block in `~/.bashrc` or `~/.zshrc` |
| `termino connect <agent>` / `termino disconnect <agent>` | Connect/disconnect an agent integration (`kiro`) |
| `termino mcp [--actor --read-only]` | Run the MCP server on stdio |
| `termino share init` / `status` / `off` | Share the current project through the git repository (see [Sharing with your team](#sharing-with-your-team)) |
| `termino theme [name]` | List themes (current marked `*`), or set one: `default`, `spooky`, `dracula`, `nord`, `gruvbox`, `catppuccin-mocha`, `tokyo-night`, `solarized-light` (for light terminals), `high-contrast` (monochrome, ASCII priority symbols), or your own |
| `termino doctor` | Diagnose the installation |
| `termino uninstall [--purge]` | Remove integrations; `--purge` also deletes data and config |
| `termino version` | Print the version |
| `termino completion <shell>` | Shell completion script |

Tasks are referenced as `KEY-N`, `N` / `#N` (current project) or by ID;
columns, members and sprints by ID or case-insensitive name. Column names are
unique per project, ignoring case. When a name is not found, the error lists
the valid ones. A wrong number of arguments or a bad flag prints the
command's usage line; an unknown subcommand (`termino task mvoe`) fails and
suggests the closest one.

The "current project" is the one associated with the working directory
(`project new --path` / `--associate-pwd`, longest matching path wins); outside
any associated directory it is the project last created or selected (`termino project use <key>`, or `p` on the board).

## Configuration

termino follows the XDG base directory spec:

| Path | Content |
|---|---|
| `$XDG_CONFIG_HOME/termino/config.toml` (`~/.config/termino`) | Settings |
| `$XDG_CONFIG_HOME/termino/themes/*.toml` | User themes |
| `$XDG_DATA_HOME/termino/termino.db` (`~/.local/share/termino`) | SQLite database |
| `$XDG_STATE_HOME/termino/termino.log` (`~/.local/state/termino`) | Log file (never stdout) |
| `$XDG_STATE_HOME/termino/installed.json` | Install registry used by `uninstall` |

`config.toml` keys:

```toml
user_name = "ana"
current_project_id = ""
theme = "default"
motion = true        # false disables animations
```

Environment: `NO_COLOR` (any non-empty value) disables animations.

## Architecture

Clean Architecture organised by feature. Each feature under
`internal/features/<name>/` has `domain`, `application`, `infrastructure` and
`presentation` layers, and dependencies always point at the domain.

```
cmd/termino            entry point
internal/cli           Cobra commands and dependency wiring
internal/app/tui       root Bubble Tea model (screens, live refresh)
internal/app/mcp       MCP server (mark3labs/mcp-go, stdio)
internal/features/     project, board, task, member, sprint, agent,
                       tasksource, connector, widget, setup
internal/platform/     config (XDG), sqlite (embedded migrations), logging,
                       events, sanitize (terminal-escape stripping)
```

The TUI polls SQLite `PRAGMA data_version` so changes made by agents or other
processes appear without a restart. All text from tasks is stripped of escape
sequences before it is drawn.

## Development

```sh
make build             # ./bin/termino with version injected
make test              # go test ./...
make vet               # go vet ./...
make lint              # fails if gofmt -l reports files
make race              # go test -race ./... (needs cgo)
make bench             # latency benchmarks
make install           # go install with version ldflags
make release-snapshot  # goreleaser build into ./dist without publishing
make clean
```

### Performance

`make bench` runs `internal/bench` against a temporary SQLite database seeded
with 500 (widget) or 1,000 (board, MCP) tasks. Measured on an Intel i7-1255U,
Linux, `go test -bench`:

| Measure (TRD 14) | Target | Measured |
|---|---|---|
| Widget summary: open DB + query + render, 500 tasks | < 100 ms | ~2.5 ms |
| Board key + redraw, 1,000 tasks | < 50 ms | ~11 ms |
| Board reload after a change, 1,000 tasks | — | ~8.7 ms |
| MCP `get_board` / `list_tasks` / `move_task`, 1,000 tasks | — | ~10.6 / ~10.4 / ~3.8 ms |
| Binary size (linux/amd64, stripped) | < 30 MB | 13.3 MB |

`TestWidgetSummaryP50WithinTarget` fails the normal test run if the widget
p50 exceeds 100 ms (skipped with `-short` and under `-race`). Process start-up
is not included in the in-process numbers.

### Agent evaluation

`eval/prompts.md` holds the PRD 6 evaluation set: 20 natural-language
requests with the expected MCP calls and database-level pass criteria (target:
18/20). `go test ./eval` checks the set stays consistent with the server's
tool names and argument schemas; scoring an agent is a manual run described in
the file.

Releases: pushing a `v*` tag runs `.github/workflows/release.yml`, which
builds with GoReleaser and publishes a GitHub release with archives and
`checksums.txt`, so `install.sh` picks it up right away. Pre-release tags
(for example `v1.2.0-rc1`) are marked as pre-releases and skipped by
`install.sh`.

## License

MIT. See [LICENSE](LICENSE).
