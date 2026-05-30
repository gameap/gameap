-- +goose Up
ALTER TABLE users ADD COLUMN metadata TEXT DEFAULT NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN metadata;
