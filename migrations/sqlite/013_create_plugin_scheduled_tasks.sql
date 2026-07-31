-- +goose Up

CREATE TABLE plugin_scheduled_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    interval_ms INTEGER NOT NULL,
    error_policy TEXT NOT NULL DEFAULT 'ignore',
    max_retries INTEGER NOT NULL DEFAULT 0,
    retry_delay_ms INTEGER NOT NULL DEFAULT 0,
    max_jitter_ms INTEGER NOT NULL DEFAULT 0,
    timeout_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT NULL,
    updated_at TEXT DEFAULT NULL
);

CREATE UNIQUE INDEX plugin_scheduled_tasks_plugin_name_index ON plugin_scheduled_tasks (plugin_id, name);

-- +goose Down

DROP TABLE IF EXISTS plugin_scheduled_tasks;
