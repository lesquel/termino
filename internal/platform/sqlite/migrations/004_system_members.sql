-- migration: 004
-- Insert built-in system members used by the CLI and MCP layers.

INSERT OR IGNORE INTO members (id, name, kind, created_at)
VALUES
    ('cli',    'CLI',    'human', datetime('now')),
    ('system', 'System', 'human', datetime('now'));
