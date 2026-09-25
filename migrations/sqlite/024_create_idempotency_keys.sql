-- +goose Up

-- Outcome of a mutating request sent with an Idempotency-Key header, replayed
-- to retries with the same key until expires_at. The client key and the
-- request are stored only as keyed hashes.
CREATE TABLE idempotency_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    key_hash TEXT NOT NULL,
    request_method TEXT NOT NULL,
    request_path TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_headers TEXT NOT NULL,
    response_body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    -- Unix milliseconds: TEXT timestamps do not compare reliably in SQL.
    expires_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idempotency_keys_user_key_index ON idempotency_keys (user_id, key_hash);
CREATE INDEX idempotency_keys_expires_at_index ON idempotency_keys (expires_at);

-- +goose Down

DROP TABLE IF EXISTS idempotency_keys;
