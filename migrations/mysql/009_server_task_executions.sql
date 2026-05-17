-- +goose Up

ALTER TABLE servers_tasks
    ADD COLUMN node_id int(11) DEFAULT NULL,
    ADD COLUMN name varchar(128) DEFAULT NULL,
    ADD COLUMN enabled tinyint(1) NOT NULL DEFAULT 1,
    ADD COLUMN overlap_policy varchar(16) NOT NULL DEFAULT 'skip',
    ADD COLUMN catchup_policy varchar(16) NOT NULL DEFAULT 'skip',
    ADD COLUMN created_by_user_id int(11) DEFAULT NULL,
    ADD COLUMN version bigint(20) NOT NULL DEFAULT 1,
    ADD COLUMN timezone varchar(64) DEFAULT NULL,
    ADD COLUMN deleted_at timestamp NULL DEFAULT NULL;

UPDATE servers_tasks st
JOIN servers s ON s.id = st.server_id
SET st.node_id = s.ds_id
WHERE st.node_id IS NULL;

CREATE INDEX servers_tasks_node_id_index ON servers_tasks (node_id);
CREATE INDEX servers_tasks_deleted_at_index ON servers_tasks (deleted_at);

CREATE TABLE servers_task_executions (
    id                    bigint(20) unsigned NOT NULL AUTO_INCREMENT,
    execution_id          char(36) NOT NULL,
    server_task_id        bigint(20) unsigned NOT NULL,
    server_id             int(11) NOT NULL,
    node_id               int(11) NOT NULL,
    command               varchar(16) NOT NULL,
    task_version          bigint(20) NOT NULL DEFAULT 1,
    status                varchar(16) NOT NULL,
    exit_code             int(11) DEFAULT NULL,
    error_message         text DEFAULT NULL,
    started_at            timestamp NOT NULL,
    finished_at           timestamp NULL DEFAULT NULL,
    duration_ms           bigint(20) DEFAULT NULL,
    output_inline         text DEFAULT NULL,
    output_storage_path   varchar(512) DEFAULT NULL,
    created_at            timestamp NOT NULL,
    updated_at            timestamp NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY servers_task_executions_execution_id_unique (execution_id),
    KEY servers_task_executions_task_idx (server_task_id, started_at),
    KEY servers_task_executions_server_idx (server_id, started_at),
    KEY servers_task_executions_node_idx (node_id, started_at),
    KEY servers_task_executions_status_idx (node_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP TABLE IF EXISTS servers_tasks_fails;

-- +goose Down

CREATE TABLE servers_tasks_fails (
    id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
    server_task_id bigint(20) unsigned NOT NULL,
    output longtext NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    PRIMARY KEY (id),
    KEY servers_tasks_fails_server_task_id_index (server_task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP TABLE IF EXISTS servers_task_executions;

DROP INDEX servers_tasks_node_id_index ON servers_tasks;
DROP INDEX servers_tasks_deleted_at_index ON servers_tasks;

ALTER TABLE servers_tasks
    DROP COLUMN node_id,
    DROP COLUMN name,
    DROP COLUMN enabled,
    DROP COLUMN overlap_policy,
    DROP COLUMN catchup_policy,
    DROP COLUMN created_by_user_id,
    DROP COLUMN version,
    DROP COLUMN timezone,
    DROP COLUMN deleted_at;
