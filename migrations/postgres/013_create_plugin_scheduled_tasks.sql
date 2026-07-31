-- +goose Up

CREATE TABLE plugin_scheduled_tasks (
    id BIGSERIAL PRIMARY KEY,
    plugin_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    interval_ms BIGINT NOT NULL,
    error_policy VARCHAR(16) NOT NULL DEFAULT 'ignore',
    max_retries INTEGER NOT NULL DEFAULT 0,
    retry_delay_ms BIGINT NOT NULL DEFAULT 0,
    max_jitter_ms BIGINT NOT NULL DEFAULT 0,
    timeout_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);

CREATE UNIQUE INDEX plugin_scheduled_tasks_plugin_name_index ON plugin_scheduled_tasks (plugin_id, name);

-- +goose Down

DROP TABLE IF EXISTS plugin_scheduled_tasks;
