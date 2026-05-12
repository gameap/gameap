-- +goose Up

ALTER TABLE servers_tasks
    ADD COLUMN node_id INTEGER DEFAULT NULL,
    ADD COLUMN name VARCHAR(128) DEFAULT NULL,
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN overlap_policy VARCHAR(16) NOT NULL DEFAULT 'skip',
    ADD COLUMN catchup_policy VARCHAR(16) NOT NULL DEFAULT 'skip',
    ADD COLUMN created_by_user_id INTEGER DEFAULT NULL,
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN timezone VARCHAR(64) DEFAULT NULL,
    ADD COLUMN deleted_at TIMESTAMPTZ DEFAULT NULL;

UPDATE servers_tasks st
SET node_id = s.ds_id
FROM servers s
WHERE s.id = st.server_id
  AND st.node_id IS NULL;

CREATE INDEX servers_tasks_node_id_index ON servers_tasks (node_id) WHERE deleted_at IS NULL;
CREATE INDEX servers_tasks_deleted_at_index ON servers_tasks (deleted_at);

CREATE TABLE servers_task_executions (
    id                    BIGSERIAL PRIMARY KEY,
    execution_id          UUID NOT NULL,
    server_task_id        BIGINT NOT NULL,
    server_id             INTEGER NOT NULL,
    node_id               INTEGER NOT NULL,
    command               VARCHAR(16) NOT NULL,
    task_version          BIGINT NOT NULL DEFAULT 1,
    status                VARCHAR(16) NOT NULL,
    exit_code             INTEGER DEFAULT NULL,
    error_message         TEXT DEFAULT NULL,
    started_at            TIMESTAMPTZ NOT NULL,
    finished_at           TIMESTAMPTZ DEFAULT NULL,
    duration_ms           BIGINT DEFAULT NULL,
    output_inline         TEXT DEFAULT NULL,
    output_storage_path   VARCHAR(512) DEFAULT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX servers_task_executions_execution_id_unique ON servers_task_executions (execution_id);
CREATE INDEX servers_task_executions_task_idx ON servers_task_executions (server_task_id, started_at DESC);
CREATE INDEX servers_task_executions_server_idx ON servers_task_executions (server_id, started_at DESC);
CREATE INDEX servers_task_executions_node_idx ON servers_task_executions (node_id, started_at DESC);
CREATE INDEX servers_task_executions_running_idx ON servers_task_executions (node_id) WHERE status = 'running';

DROP TABLE IF EXISTS servers_tasks_fails;

-- +goose Down

CREATE TABLE servers_tasks_fails (
    id BIGSERIAL PRIMARY KEY,
    server_task_id BIGINT NOT NULL,
    output TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX servers_tasks_fails_server_task_id_index ON servers_tasks_fails (server_task_id);

DROP TABLE IF EXISTS servers_task_executions;

DROP INDEX IF EXISTS servers_tasks_node_id_index;
DROP INDEX IF EXISTS servers_tasks_deleted_at_index;

ALTER TABLE servers_tasks
    DROP COLUMN IF EXISTS node_id,
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS enabled,
    DROP COLUMN IF EXISTS overlap_policy,
    DROP COLUMN IF EXISTS catchup_policy,
    DROP COLUMN IF EXISTS created_by_user_id,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS deleted_at;
