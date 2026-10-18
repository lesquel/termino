-- migration: 007
-- Repositories a project stopped sharing from (`termino share off`), so
-- running termino there again does not link the project back to the
-- .termino/ files left in place.

CREATE TABLE IF NOT EXISTS share_optouts (
    project_id TEXT NOT NULL,
    root       TEXT NOT NULL,
    PRIMARY KEY (project_id, root)
);
