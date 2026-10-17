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

The script downloads the release archive for your OS/architecture plus
`checksums.txt`, verifies the SHA-256 and refuses to install on a mismatch.

| Variable | Default | Meaning |
|---|---|---|
| `TERMINO_VERSION` | `latest` | Release tag to install, e.g. `v0.1.0` |
| `TERMINO_INSTALL_DIR` | `$HOME/.local/bin` | Where the binary goes |

### Go

```sh
go install github.com/lesquel/termino/cmd/termino@latest
```

Requires Go 1.25+. No cgo is needed (`modernc.org/sqlite` is pure Go).

### Release archives

Every release publishes `termino_<version>_<os>_<arch>.tar.gz` (`.zip` on
Windows) for linux, darwin and windows on amd64 and arm64, and a
`checksums.txt` with SHA-256 sums.

## Quick start

```sh
termino setup                               # user name, first project, widget, agents
termino project new --name "Web app" --key WEB --associate-pwd
termino add "Fix login redirect" --priority high
termino                                     # open the board
termino widget install                      # status summary in bash/zsh prompt
termino connect kiro                        # register the MCP server in Kiro
```

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
| `enter` | Task detail |
| `p` | Project selector |
| `?` | Help |
| `q` / `ctrl+c` | Quit |

Moving a card into the last column (Done) plays a short celebration, unless
motion is off or `NO_COLOR` is set. Columns over their WIP limit are flagged.

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

## CLI reference

Run `termino <command> --help` for full flags.

| Command | Description |
|---|---|
| `termino` | Open the TUI |
| `termino board` | Open the Kanban board (TUI) |
| `termino setup` | Interactive setup: user name, first project, integrations |
| `termino project new [--name --key --methodology kanban\|scrum --path --associate-pwd]` | Create a project |
| `termino project list` | List projects |
| `termino add <title> [--column --description --priority]` | Add a task to the current project |
| `termino task move <task> <column>` | Move a task |
| `termino task assign <task> <member\|none>` | Assign or unassign |
| `termino task done <task>` | Move a task to Done |
| `termino my-tasks [--member]` | Tasks assigned to you across projects |
| `termino member add <name> [--kind human\|agent]` / `list` / `remove <member>` | Manage members |
| `termino column add <name>` / `list` / `rename` / `reorder` / `delete` / `wip <column> <limit>` | Manage columns (`wip … 0` removes the limit) |
| `termino sprint new <name> [--start --end]` / `list` / `start <sprint>` / `close <sprint> [--unfinished backlog\|next_sprint]` | Manage sprints |
| `termino import [path] [--watch]` | Import Markdown checkbox tasks (Kiro `tasks.md`, `odd/tasks/*.md`, any `- [ ]` list); `--watch` keeps syncing |
| `termino widget` | Print the status-bar summary |
| `termino widget install` / `uninstall` | Add/remove the widget block in `~/.bashrc` or `~/.zshrc` |
| `termino connect <agent>` / `termino disconnect <agent>` | Connect/disconnect an agent integration (`kiro`) |
| `termino mcp [--actor --read-only]` | Run the MCP server on stdio |
| `termino theme <name>` | Set the colour theme (`default`, `spooky`, or your own) |
| `termino doctor` | Diagnose the installation |
| `termino uninstall [--purge]` | Remove integrations; `--purge` also deletes data and config |
| `termino version` | Print the version |
| `termino completion <shell>` | Shell completion script |

Tasks are referenced as `KEY-N`, `N` / `#N` (current project) or by ID;
columns, members and sprints by ID or case-insensitive name.

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

Releases: pushing a `v*` tag runs `.github/workflows/release.yml`, which
builds with GoReleaser and creates a **draft** GitHub release with archives
and `checksums.txt`; publish the draft to make it visible to `install.sh`.

## License

License: TBD.
