-- migration: 006
-- Repository sharing import state: when each shared project was last
-- imported and which shared files that import had to skip (and why), so
-- `termino share status` and the board warning see what any process found.

CREATE TABLE IF NOT EXISTS share_imports (
    project_id  TEXT NOT NULL PRIMARY KEY,
    imported_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS share_skipped (
    project_id TEXT NOT NULL,
    path       TEXT NOT NULL,
    reason     TEXT NOT NULL,
    PRIMARY KEY (project_id, path)
);
