-- +goose Up
ALTER TABLE servers ALTER COLUMN ram_limit TYPE BIGINT;

-- +goose Down
ALTER TABLE servers ALTER COLUMN ram_limit TYPE INTEGER;
