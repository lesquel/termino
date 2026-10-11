-- migration: 003
-- Adds columns required by infrastructure layer implementations.

-- Projects: archive support and path detection.
ALTER TABLE projects ADD COLUMN archived_at TEXT;
ALTER TABLE projects ADD COLUMN associated_path TEXT;

-- Members: creation timestamp.
ALTER TABLE members ADD COLUMN created_at TEXT NOT NULL DEFAULT (datetime('now'));

-- Agent sessions: agent display name.
ALTER TABLE agent_sessions ADD COLUMN agent_name TEXT NOT NULL DEFAULT '';
