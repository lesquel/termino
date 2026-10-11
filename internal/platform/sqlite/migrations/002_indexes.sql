-- migration: 002

CREATE INDEX IF NOT EXISTS idx_tasks_project_column_position
    ON tasks(project_id, column_id, position);

CREATE INDEX IF NOT EXISTS idx_tasks_assignee
    ON tasks(assignee_id);

CREATE INDEX IF NOT EXISTS idx_activity_task
    ON activity(task_id);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent
    ON agent_sessions(agent_id);
