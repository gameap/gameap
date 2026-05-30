-- +goose Up
ALTER TABLE users ADD COLUMN metadata JSONB DEFAULT NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN metadata;
