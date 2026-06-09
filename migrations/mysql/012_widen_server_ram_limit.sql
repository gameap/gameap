-- +goose Up
ALTER TABLE servers MODIFY ram_limit bigint unsigned DEFAULT NULL;

-- +goose Down
ALTER TABLE servers MODIFY ram_limit int(10) unsigned DEFAULT NULL;
