# Termino — Sharing a board through the repository

Status: approved design (2026-10-17), revised after a code review. Moves the PRD v2.0 item "team sync" forward, scoped to git-based sharing.

## 1. Goal

Collaborators who share a git repository see the same backlog and progress in termino without a server, accounts, or extra steps. Termino keeps plain-text files inside the repo in sync with its local database; git carries them between people.

## 2. Non-goals

- Termino never runs `git commit`, `git push` or `git pull`. Users ship `.termino/` with their normal git flow.
- No real-time sync, server, accounts or permissions.
- Activity history and comments are not shared in this version; they stay in the local database.
- No GitHub/Jira integration.

## 3. Decisions

| Topic | Decision | Reason |
|---|---|---|
| Transport | Files in the repo, carried by git | No infrastructure; history and review come for free |
| Format | TOML | One key per line gives clean diffs; multi-line descriptions stay readable with `"""`; strict spec (no YAML ambiguity); already a dependency (themes) |
| Layout | One file per task | Edits to different tasks never conflict |
| File name | Task UUID (`<uuid>.toml`), not `PROJ-3` | Two people creating a task at once produce two files, not a git add/add conflict |
| Source of truth on disk | `.termino/` files; SQLite is a local cache | Same model as beads (JSONL + SQLite cache) |
| Automation | Auto export on every change, auto import on file change and on every command in a shared project | User asked for automatic sync without auto commits |
| Export hook | Decorators around the task, project, board and member use cases, wired once in `internal/cli/container.go` | The event bus has no subscribers and most mutations publish nothing; decorators cover TUI, CLI and MCP in one place |

Prior art: beads (`.beads/issues.jsonl` + local SQLite, auto export/import, custom merge driver because of a single file); git-bug (operation log inside git objects, conflict-free but invisible and complex).

## 4. Layout

```
<repo>/.termino/
  project.toml
  tasks/
    <task-uuid>.toml
```

`project.toml`:

```toml
version = 1
key = "PROJ"
name = "termino"
methodology = "kanban"

[[columns]]
name = "Backlog"
position = 0
wip_limit = 0

[[members]]
name = "Ana"
kind = "human"

[[labels]]
name = "ui"
color = "#7aa2f7"

[[sprints]]          # scrum projects only
name = "Sprint 1"
goal = "MVP"
start = 2026-10-20
end = 2026-11-03
state = "active"
```

`tasks/<task-uuid>.toml`:

```toml
version = 1
id = "3f6c2a9e-8d1b-4c57-9a0e-2b7d4e1f6a33"
number = 3
title = "Probar los dialogos nuevos"
column = "Backlog"
position = 2
priority = "medium"
points = 3
assignee = "Ana"
labels = ["ui"]
sprint = "Sprint 1"
due = 2026-10-30
created_at = 2026-10-17T18:00:00Z
updated_at = 2026-10-17T18:10:00Z
updated_by = "Ana"
description = """
Multi-line text
kept as written.
"""
```

Field scope:

| Exported | Local only |
|---|---|
| id, number, title, description, column (by name), position, priority, points, assignee (member name), labels (names), sprint (name), due, created_at, updated_at, updated_by | comments, activity, `source`, `source_path`, `source_ref` (they hold local absolute paths) |

Rules:
- Fixed key order as shown; empty optional keys are omitted; timestamps in UTC RFC 3339; output is byte-for-byte deterministic for the same task.
- `ref` is not stored; it is always `<project key>-<number>`.
- Columns, members, labels and sprints are referenced by name, never by local database IDs. Column names are already unique per project (case-insensitive) by domain rule.
- Members are global in the database: import matches them by name against the global `members` table. Built-in members are filtered by ID (`cli`, `system`) and never exported.
- `version` allows future format changes; files with an unknown higher version are skipped with a warning.

## 5. Enabling sharing

- `termino share init` (run inside the repo): creates `.termino/` at the git root of the current directory, stores that root on the project, sets `associated_path` to it when empty, and exports everything. Fails clearly outside a git repo.
- `termino share status`: shows the share root, file count, and files skipped on the last import with the reason.
- `termino share off`: stops syncing (files stay on disk; `share_root` is cleared).
- The setup wizard and the settings screen (`s`) offer the same on/off toggle.
- Storage: new column `projects.share_root` (migration 005 in `internal/platform/sqlite/migrations/`, the embedded copy).
- Cloning: when termino runs in a directory whose git root holds `.termino/project.toml`:
  - no local project with that key → create it with `share_root` set, import everything;
  - a local project with that key and no `share_root` → link it (set `share_root`) and import, merging by task UUID;
  - a local project with that key already shared from another root → error naming both roots; nothing is changed.
  - If the config `user_name` is not among the exported members, it is added as a human member and exported.

## 6. Sync

### Export
- The share decorators run after every successful mutation from TUI, CLI or MCP and write the affected files with an atomic write (temp file `.<name>.tmp` + rename).
- Single-file mutations: create, update, move, reorder, assign, label, delete (removes the file), restore (re-creates the file).
- Multi-file mutations: renaming a column, member, label or sprint, and changing the project key, rewrite `project.toml` and every task file that references the old value.
- `updated_at` is the mutation time; `updated_by` is the local actor (config `user_name` member).
- Task timestamps move into the domain: `Task` gains `CreatedAt`, `UpdatedAt`, `UpdatedBy`; migration 005 adds `tasks.updated_by`; the repository gains a save variant that keeps given timestamps (used by import) instead of stamping `now`.

### Import
- Triggers: a debounced (300 ms) fsnotify watcher on `.termino/` while the TUI or MCP server runs, and a scan at the start of every CLI command, board open and MCP start in a shared project (read-only commands and `mcp --read-only` included: import writes only the local database).
- Change detection uses a shared table `share_files(project_id, path, hash)` in SQLite, so every process (TUI, MCP, CLI) knows what was last written or read. A file whose hash matches is skipped. Exports update the table in the same transaction as the database write.
- Imported changes never go through the export decorators (they call the undecorated use cases / repositories), so there is no export loop.
- Import is idempotent: equal content produces no write.
- Files starting with `.` or ending in `.tmp` are ignored.
- A missing file soft-deletes the task.
- Unknown column names are created at the end; unknown member names are added as human members; unknown labels are created with a default color; unknown sprints are created as planned.
- The TUI refreshes through the existing poll/change path.

### Concurrency
- SQLite WAL with `busy_timeout` already serializes writers across processes.
- Two processes exporting the same file write identical content derived from the database; the last rename wins harmlessly.
- Number allocation keeps the existing `UNIQUE(project_id, number)` constraint; a create that hits it retries with the next number.

## 7. Conflicts and errors

- Different tasks: no conflict by construction.
- Same task edited by two people: git reports a conflict in that file. A file with conflict markers or invalid TOML is skipped; the board status line and `share status` name the file; everything else keeps working. After the user resolves it, the next import picks it up.
- Duplicate numbers (two tasks with number 3 created concurrently on different machines): on import, the task with the later `created_at` (ties broken by UUID) is renumbered to the next free number and its file rewritten. The import runs in one transaction: renumber the loser first (new repository method `Renumber(ctx, id, n)`), then upsert. Every machine applies the same rule on the same exported timestamps, so they converge. Known effect: a `PROJ-3` mentioned in a comment or commit message may now point to a different task.
- Errors are logged to the termino log file, never printed to MCP stdout.

## 8. Architecture

New feature package `internal/features/share/`:
- `domain`: snapshot types (ProjectFile, TaskFile), format version, duplicate-number rule.
- `application`: `ShareService` (Init, Status, Off, Export*, Import, Link) and the use-case decorators; depends on ports for task, project, board, member, label and sprint repositories, a file store, and the `share_files` hash store.
- `infrastructure`: TOML codec (deterministic encoder), filesystem store (atomic writes, hashes), fsnotify watcher, git root lookup, SQLite `share_files` repository.
- `presentation`: `termino share` cobra commands; TUI hooks (settings toggle, status warning).

Wiring: `internal/cli/container.go` wraps the use cases with the share decorators when the current project has a `share_root`.

## 9. Testing

- Codec round trip (domain → TOML → domain) and golden files for deterministic output.
- Each single-file mutation writes exactly one file; delete removes it; restore re-creates it; renames rewrite every referencing file.
- Import: new, updated, deleted, unknown column/member/label/sprint, invalid file and conflict markers skipped with warning, timestamps preserved.
- No export loop after import; two processes sharing `share_files` do not re-import each other's writes.
- Two simulated machines in temp dirs with a real git repo: create on A, commit, pull on B, B sees it; concurrent creates on A and B converge to the same numbering after merge.
- Cloning flows: new project, link existing key, key shared from another root.
- `share init` outside git fails; `share off` stops writes.

## 10. Later

- Optional sharing of comments and activity history.
- Optional `termino share sync` helper that pulls, imports, commits `.termino/` and pushes.
