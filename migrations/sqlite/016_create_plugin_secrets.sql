-- +goose Up

CREATE TABLE plugin_secrets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id INTEGER NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TEXT DEFAULT NULL,
    updated_at TEXT DEFAULT NULL
);

CREATE UNIQUE INDEX plugin_secrets_plugin_key_index ON plugin_secrets (plugin_id, key);

-- +goose Down

DROP TABLE IF EXISTS plugin_secrets;
