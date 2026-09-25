-- +goose Up

-- Outcome of a mutating request sent with an Idempotency-Key header, replayed
-- to retries with the same key until expires_at. The client key and the
-- request are stored only as keyed hashes.
CREATE TABLE idempotency_keys (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    key_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    request_method VARCHAR(10) NOT NULL,
    request_path VARCHAR(255) NOT NULL,
    request_fingerprint VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    response_status SMALLINT UNSIGNED NOT NULL,
    response_headers TEXT NOT NULL,
    response_body MEDIUMBLOB NOT NULL,
    -- DATETIME, not TIMESTAMP: no session time zone conversion and no implicit
    -- defaults on NOT NULL columns; the repository binds UTC values.
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    UNIQUE KEY idempotency_keys_user_key_index (user_id, key_hash),
    KEY idempotency_keys_expires_at_index (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down

DROP TABLE IF EXISTS idempotency_keys;
