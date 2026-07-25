-- +goose Up

CREATE TABLE plugin_scheduled_tasks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    plugin_id BIGINT UNSIGNED NOT NULL,
    name VARCHAR(255) NOT NULL,
    interval_ms BIGINT NOT NULL,
    error_policy VARCHAR(16) NOT NULL DEFAULT 'ignore',
    max_retries INT UNSIGNED NOT NULL DEFAULT 0,
    retry_delay_ms BIGINT NOT NULL DEFAULT 0,
    max_jitter_ms BIGINT NOT NULL DEFAULT 0,
    timeout_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT NULL,
    updated_at TIMESTAMP DEFAULT NULL,
    UNIQUE KEY plugin_scheduled_tasks_plugin_name_index (plugin_id, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down

DROP TABLE IF EXISTS plugin_scheduled_tasks;
