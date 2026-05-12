-- +goose Up

ALTER TABLE servers_tasks ADD COLUMN node_id INTEGER DEFAULT NULL;
ALTER TABLE servers_tasks ADD COLUMN name TEXT DEFAULT NULL;
ALTER TABLE servers_tasks ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE servers_tasks ADD COLUMN overlap_policy TEXT NOT NULL DEFAULT 'skip';
ALTER TABLE servers_tasks ADD COLUMN catchup_policy TEXT NOT NULL DEFAULT 'skip';
ALTER TABLE servers_tasks ADD COLUMN created_by_user_id INTEGER DEFAULT NULL;
ALTER TABLE servers_tasks ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE servers_tasks ADD COLUMN timezone TEXT DEFAULT NULL;
ALTER TABLE servers_tasks ADD COLUMN deleted_at TEXT DEFAULT NULL;

UPDATE servers_tasks
SET node_id = (SELECT ds_id FROM servers WHERE servers.id = servers_tasks.server_id)
WHERE node_id IS NULL;

CREATE INDEX servers_tasks_node_id_index ON servers_tasks(node_id) WHERE deleted_at IS NULL;
CREATE INDEX servers_tasks_deleted_at_index ON servers_tasks(deleted_at);

CREATE TABLE servers_task_executions (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id          TEXT NOT NULL,
    server_task_id        INTEGER NOT NULL,
    server_id             INTEGER NOT NULL,
    node_id               INTEGER NOT NULL,
    command               TEXT NOT NULL,
    task_version          INTEGER NOT NULL DEFAULT 1,
    status                TEXT NOT NULL,
    exit_code             INTEGER DEFAULT NULL,
    error_message         TEXT DEFAULT NULL,
    started_at            TEXT NOT NULL,
    finished_at           TEXT DEFAULT NULL,
    duration_ms           INTEGER DEFAULT NULL,
    output_inline         TEXT DEFAULT NULL,
    output_storage_path   TEXT DEFAULT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE UNIQUE INDEX servers_task_executions_execution_id_unique ON servers_task_executions(execution_id);
CREATE INDEX servers_task_executions_task_idx ON servers_task_executions(server_task_id, started_at DESC);
CREATE INDEX servers_task_executions_server_idx ON servers_task_executions(server_id, started_at DESC);
CREATE INDEX servers_task_executions_node_idx ON servers_task_executions(node_id, started_at DESC);
CREATE INDEX servers_task_executions_running_idx ON servers_task_executions(node_id) WHERE status = 'running';

DROP TABLE IF EXISTS servers_tasks_fails;

-- +goose Down

CREATE TABLE servers_tasks_fails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_task_id INTEGER NOT NULL,
    output TEXT NOT NULL,
    created_at TEXT DEFAULT NULL,
    updated_at TEXT DEFAULT NULL
);
CREATE INDEX servers_tasks_fails_server_task_id_index ON servers_tasks_fails(server_task_id);

DROP TABLE IF EXISTS servers_task_executions;

DROP INDEX IF EXISTS servers_tasks_node_id_index;
DROP INDEX IF EXISTS servers_tasks_deleted_at_index;
