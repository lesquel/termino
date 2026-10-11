-- migration: 001

CREATE TABLE IF NOT EXISTS projects (
    id            TEXT    PRIMARY KEY,
    key           TEXT    NOT NULL UNIQUE,
    name          TEXT    NOT NULL,
    methodology   TEXT    NOT NULL DEFAULT 'kanban',
    created_at    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS columns (
    id         TEXT    PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects(id),
    name       TEXT    NOT NULL,
    position   REAL    NOT NULL,
    wip_limit  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sprints (
    id         TEXT    PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects(id),
    name       TEXT    NOT NULL,
    starts_at  TEXT,
    ends_at    TEXT,
    state      TEXT    NOT NULL DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS members (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL DEFAULT 'human'
);

CREATE TABLE IF NOT EXISTS labels (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT    PRIMARY KEY,
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    number        INTEGER NOT NULL,
    title         TEXT    NOT NULL,
    description   TEXT    NOT NULL DEFAULT '',
    column_id     TEXT    REFERENCES columns(id),
    sprint_id     TEXT    REFERENCES sprints(id),
    priority      TEXT    NOT NULL DEFAULT 'medium',
    assignee_id   TEXT    REFERENCES members(id),
    due_date      TEXT,
    points        INTEGER NOT NULL DEFAULT 0,
    position      REAL    NOT NULL DEFAULT 0.0,
    deleted_at    TEXT,
    source        TEXT    NOT NULL DEFAULT '',
    source_path   TEXT    NOT NULL DEFAULT '',
    source_ref    TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    UNIQUE(project_id, number)
);

CREATE TABLE IF NOT EXISTS task_sources (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    source      TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_ref  TEXT NOT NULL,
    synced_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_labels (
    task_id  TEXT NOT NULL REFERENCES tasks(id),
    label_id TEXT NOT NULL REFERENCES labels(id),
    PRIMARY KEY (task_id, label_id)
);

CREATE TABLE IF NOT EXISTS comments (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES tasks(id),
    author_id  TEXT REFERENCES members(id),
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS activity (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES tasks(id),
    actor_id   TEXT REFERENCES members(id),
    action     TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_sessions (
    agent_id        TEXT PRIMARY KEY,
    last_seen_at    TEXT NOT NULL,
    current_task_id TEXT REFERENCES tasks(id)
);

CREATE TABLE IF NOT EXISTS installation_registry (
    id         TEXT PRIMARY KEY,
    path       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    marker     TEXT NOT NULL DEFAULT '',
    backup     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
