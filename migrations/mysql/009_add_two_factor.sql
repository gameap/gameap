-- +goose Up

ALTER TABLE users
    ADD COLUMN two_factor_enabled tinyint(1) NOT NULL DEFAULT 0,
    ADD COLUMN two_factor_secret text DEFAULT NULL,
    ADD COLUMN two_factor_recovery_codes text DEFAULT NULL,
    ADD COLUMN two_factor_last_used_step bigint(20) DEFAULT NULL;

-- +goose Down

ALTER TABLE users
    DROP COLUMN two_factor_enabled,
    DROP COLUMN two_factor_secret,
    DROP COLUMN two_factor_recovery_codes,
    DROP COLUMN two_factor_last_used_step;
