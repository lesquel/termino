# Agent evaluation set (PRD 6)

Twenty natural-language requests an agent (e.g. Kiro) sends while connected
to `termino mcp`. The goal from PRD 6: **at least 18 of 20 leave the board in
the expected state**, verified by reading the database, not the agent's reply.

`eval_test.go` checks this file deterministically: there are exactly 20
cases, every case has all four fields, and every tool and argument named under
**Calls** exists in the MCP server's tool list and input schema. It does not
run an agent; the scoring run is manual (see "How to run").

## Fixture board

Start every case from a fresh database seeded with:

- Project `WEB` "Web app", Kanban, columns `Backlog`, `In Progress`, `Done`,
  associated with the working directory.
- Project `OPS` "Operations", Kanban.
- Members: `ana` (human), `luis` (human), `kiro` (agent, registered on first
  write).
- WEB tasks:
  - WEB-1 "Fix login redirect" · Backlog · high
  - WEB-2 "Write API docs" · Backlog · low · ana
  - WEB-3 "Add rate limiting" · In Progress · critical · luis
  - WEB-4 "Refactor session store" · Backlog · medium
  - WEB-5 "Upgrade Go toolchain" · Done · low
  - WEB-6 "Flaky checkout test" · In Progress · high · kiro
  - WEB-7 "Dark mode toggle" · Backlog · low
  - WEB-8 "Remove legacy cron" · Backlog · medium
- OPS tasks: OPS-1 "Rotate TLS certs" · Backlog · high · ana

Tasks are addressed by key (`WEB-4`); the MCP tools take task IDs, so the
agent resolves task keys through `get_board` or `list_tasks` (each task
carries its `Number`). Every `project_id` argument accepts either the project
ID or its key in any case (`WEB`, `web`), so a known project needs no
`list_projects` lookup. Lookups listed under **Calls** are the minimum expected; extra
read calls do not fail a case, extra or wrong writes do.

## How to run

1. `XDG_DATA_HOME=$(mktemp -d) termino setup` and seed the fixture (CLI:
   `project new`, `member add`, `add`, `task move`, `task assign`).
2. Connect the agent (`termino connect kiro`) and send the prompt verbatim.
3. Check the **Pass** criteria with `sqlite3 "$XDG_DATA_HOME/termino/termino.db"`
   or the CLI (`termino my-tasks --member …`, `termino board`).
4. Reset the database between cases. Record pass/fail per case; the run
   passes at 18/20 or better.

## Cases

### E01 · Move by key

- **Prompt:** "Move WEB-4 to In Progress."
- **Calls:** `get_board(project_id)` → `move_task(task_id, target_column_id)`
- **Expected:** WEB-4 is in `In Progress`; nothing else moved.
- **Pass:** `tasks` row for WEB-4 has the In Progress `column_id`; one `moved` activity for WEB-4 whose actor is the agent.

### E02 · Create one task

- **Prompt:** "Add a task to the web project: 'Add CSRF tokens to forms', high priority."
- **Calls:** `create_task(project_id, title, priority)` (project_id `WEB`)
- **Expected:** a new WEB task in `Backlog` with priority high.
- **Pass:** exactly one new task in WEB titled "Add CSRF tokens to forms", priority `high`, column Backlog, `created` activity by the agent.

### E03 · Create several tasks from a spec

- **Prompt:** "Create three tasks from this spec: users can reset their password by email; the reset link expires after 30 minutes; the old sessions are revoked after a reset."
- **Calls:** `list_projects()` → `create_task(project_id, title, description)` ×3
- **Expected:** three new WEB tasks, one per requirement.
- **Pass:** exactly three new WEB tasks whose titles cover reset-by-email, link expiry, session revocation; all in Backlog; no other task changed.

### E04 · Assign to a person

- **Prompt:** "Assign WEB-1 to Ana."
- **Calls:** `get_board(project_id)` → `assign_task(task_id, assignee_id)`
- **Expected:** WEB-1 assigned to ana.
- **Pass:** WEB-1 `assignee_id` is ana's member ID; `assigned` activity by the agent.

### E05 · Claim and start

- **Prompt:** "I'll take WEB-8, start working on it."
- **Calls:** `get_board(project_id)` → `claim_task(task_id, project_id)`
- **Expected:** WEB-8 assigned to the agent and in `In Progress`.
- **Pass:** WEB-8 `assignee_id` is the agent's member; column In Progress; `assigned` and `moved` activities by the agent.

### E06 · Complete a task

- **Prompt:** "Mark WEB-6 as done."
- **Calls:** `get_board(project_id)` → `complete_task(task_id, project_id)`
- **Expected:** WEB-6 in `Done`.
- **Pass:** WEB-6 column is Done (highest position); `moved` activity by the agent.

### E07 · My tasks

- **Prompt:** "What am I working on right now? Include the description."
- **Calls:** `my_tasks()` → `get_task(task_id)`
- **Expected:** the agent lists WEB-6 only, with its description; no writes.
- **Pass:** no new rows in `activity`; reply mentions WEB-6 and no other task.

### E08 · Read the board

- **Prompt:** "Give me a summary of the web board: how many tasks per column?"
- **Calls:** `list_projects()` → `get_board(project_id)`
- **Expected:** Backlog 5, In Progress 2, Done 1; no writes.
- **Pass:** no new `activity` rows; reply counts match.

### E09 · Filter by assignee

- **Prompt:** "List everything assigned to Luis."
- **Calls:** `list_tasks(assignee_id)`
- **Expected:** WEB-3 only; no writes.
- **Pass:** no new `activity` rows; reply lists WEB-3 only.

### E10 · Search

- **Prompt:** "Is there already a task about rate limiting?"
- **Calls:** `list_tasks(project_id, search_text)`
- **Expected:** finds WEB-3; does not create a duplicate.
- **Pass:** task count unchanged; reply names WEB-3.

### E11 · Raise priority

- **Prompt:** "WEB-7 is now urgent, set it to critical."
- **Calls:** `get_board(project_id)` → `update_task(task_id, priority)`
- **Expected:** WEB-7 priority critical; title unchanged.
- **Pass:** WEB-7 `priority = 'critical'`, title still "Dark mode toggle"; `updated` activity by the agent.

### E12 · Rename

- **Prompt:** "Rename WEB-2 to 'Write public API reference'."
- **Calls:** `get_board(project_id)` → `update_task(task_id, title)`
- **Expected:** WEB-2 retitled; assignee and column unchanged.
- **Pass:** WEB-2 title matches; still assigned to ana and in Backlog.

### E13 · Comment

- **Prompt:** "Leave a note on WEB-3: the limiter must be per API key, not per IP."
- **Calls:** `get_board(project_id)` → `add_comment(task_id, body)`
- **Expected:** one comment on WEB-3 authored by the agent.
- **Pass:** one new `comments` row for WEB-3 containing "per API key", author is the agent member.

### E14 · Delete with confirmation

- **Prompt:** "Delete WEB-8, we dropped the legacy cron work."
- **Calls:** `get_board(project_id)` → `delete_task(task_id, confirm)`
- **Expected:** WEB-8 soft-deleted (hidden from the board, row kept).
- **Pass:** WEB-8 row still exists with `deleted_at` set; `deleted` activity by the agent; no other task deleted.

### E15 · Reassign and move

- **Prompt:** "Luis is out this week. Give WEB-3 to Ana and put it back in the backlog."
- **Calls:** `get_board(project_id)` → `assign_task(task_id, assignee_id)` → `move_task(task_id, target_column_id)`
- **Expected:** WEB-3 assigned to ana and in `Backlog`.
- **Pass:** both fields changed on WEB-3; two activities (`assigned`, `moved`) by the agent.

### E16 · Unassign

- **Prompt:** "Nobody should own WEB-2 for now."
- **Calls:** `get_board(project_id)` → `assign_task(task_id)`
- **Expected:** WEB-2 unassigned.
- **Pass:** WEB-2 `assignee_id` is NULL.

### E17 · Cross-project

- **Prompt:** "In the operations project, move the TLS certificate task to In Progress."
- **Calls:** `list_projects()` → `get_board(project_id)` → `move_task(task_id, target_column_id)`
- **Expected:** OPS-1 in OPS `In Progress`; WEB untouched.
- **Pass:** OPS-1 column is OPS's In Progress; no WEB task changed.

### E18 · Bulk move by condition

- **Prompt:** "Move every low-priority task in the backlog to Done, we are not doing them."
- **Calls:** `get_board(project_id)` → `move_task(task_id, target_column_id)` ×2
- **Expected:** WEB-2 and WEB-7 in `Done`; WEB-1, WEB-4, WEB-8 stay in Backlog.
- **Pass:** exactly those two tasks moved.

### E19 · Spanish request (PRD example)

- **Prompt:** "Mueve WEB-4 a en progreso."
- **Calls:** `get_board(project_id)` → `move_task(task_id, target_column_id)`
- **Expected:** same end state as E01.
- **Pass:** WEB-4 in In Progress; one `moved` activity by the agent.

### E20 · Refuse an unsafe ambiguous request

- **Prompt:** "Delete the old tasks."
- **Calls:** `get_board(project_id)`
- **Expected:** the agent asks which tasks instead of deleting; no writes.
- **Pass:** no task has `deleted_at` set and no new `activity` rows.
