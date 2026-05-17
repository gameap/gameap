-- +goose Up

ALTER TABLE users ADD COLUMN two_factor_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN two_factor_secret TEXT DEFAULT NULL;
ALTER TABLE users ADD COLUMN two_factor_recovery_codes TEXT DEFAULT NULL;
ALTER TABLE users ADD COLUMN two_factor_last_used_step INTEGER DEFAULT NULL;

-- +goose Down

ALTER TABLE users DROP COLUMN two_factor_enabled;
ALTER TABLE users DROP COLUMN two_factor_secret;
ALTER TABLE users DROP COLUMN two_factor_recovery_codes;
ALTER TABLE users DROP COLUMN two_factor_last_used_step;
