-- +goose Up

-- Why the last load or the last guest call failed: shown in the admin UI and
-- cleared once the plugin loads again. Plugins with status "error" are
-- retried on the next panel start; "disabled" stays the operator's state.
ALTER TABLE plugins ADD COLUMN last_error TEXT DEFAULT NULL;
ALTER TABLE plugins ADD COLUMN last_error_at TEXT DEFAULT NULL;

-- +goose Down

ALTER TABLE plugins DROP COLUMN last_error;
ALTER TABLE plugins DROP COLUMN last_error_at;
