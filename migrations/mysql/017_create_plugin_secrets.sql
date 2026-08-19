-- +goose Up

CREATE TABLE plugin_secrets (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    plugin_id BIGINT UNSIGNED NOT NULL,
    -- Binary collation: secrets are looked up by exact key, so the unique
    -- index must be case-sensitive like the other backends.
    `key` VARCHAR(255) COLLATE utf8mb4_bin NOT NULL,
    -- Ciphertext produced by pkg/secret: "enc:" + base64(nonce||ciphertext).
    `value` TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NULL,
    updated_at TIMESTAMP DEFAULT NULL,
    UNIQUE KEY plugin_secrets_plugin_key_index (plugin_id, `key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down

DROP TABLE IF EXISTS plugin_secrets;
