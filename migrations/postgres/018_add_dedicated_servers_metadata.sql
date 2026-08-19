-- +goose Up
ALTER TABLE dedicated_servers ADD COLUMN metadata JSONB DEFAULT NULL;

-- +goose Down
ALTER TABLE dedicated_servers DROP COLUMN metadata;
