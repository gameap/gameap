-- +goose Up

ALTER TABLE users
    ADD COLUMN two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN two_factor_secret TEXT DEFAULT NULL,
    ADD COLUMN two_factor_recovery_codes TEXT DEFAULT NULL,
    ADD COLUMN two_factor_last_used_step BIGINT DEFAULT NULL;

-- +goose Down

ALTER TABLE users
    DROP COLUMN IF EXISTS two_factor_enabled,
    DROP COLUMN IF EXISTS two_factor_secret,
    DROP COLUMN IF EXISTS two_factor_recovery_codes,
    DROP COLUMN IF EXISTS two_factor_last_used_step;
