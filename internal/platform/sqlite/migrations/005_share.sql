-- migration: 005
-- Repository sharing: the project's share root, the last local editor of a
-- task, and the content hash of every file last written or read under
-- <share_root>/.termino/ so processes can skip unchanged files.

ALTER TABLE projects ADD COLUMN share_root TEXT NOT NULL DEFAULT '';

ALTER TABLE tasks ADD COLUMN updated_by TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS share_files (
    project_id TEXT NOT NULL,
    path       TEXT NOT NULL,
    hash       TEXT NOT NULL,
    PRIMARY KEY (project_id, path)
);
