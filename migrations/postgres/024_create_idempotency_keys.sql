-- +goose Up

-- Outcome of a mutating request sent with an Idempotency-Key header, replayed
-- to retries with the same key until expires_at. The client key and the
-- request are stored only as keyed hashes.
CREATE TABLE idempotency_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    request_method VARCHAR(10) NOT NULL,
    request_path VARCHAR(255) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    response_status SMALLINT NOT NULL,
    response_headers TEXT NOT NULL,
    response_body BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX idempotency_keys_user_key_index ON idempotency_keys (user_id, key_hash);
CREATE INDEX idempotency_keys_expires_at_index ON idempotency_keys (expires_at);

-- +goose Down

DROP TABLE IF EXISTS idempotency_keys;
