-- +goose Up

CREATE TABLE plugin_secrets (
    id BIGSERIAL PRIMARY KEY,
    plugin_id BIGINT NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);

CREATE UNIQUE INDEX plugin_secrets_plugin_key_index ON plugin_secrets (plugin_id, key);

-- +goose Down

DROP TABLE IF EXISTS plugin_secrets;
