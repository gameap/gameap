-- +goose Up
ALTER TABLE dedicated_servers ALTER COLUMN gdaemon_port TYPE INTEGER;
ALTER TABLE servers
    ALTER COLUMN server_port TYPE INTEGER,
    ALTER COLUMN query_port TYPE INTEGER,
    ALTER COLUMN rcon_port TYPE INTEGER;

-- +goose Down
ALTER TABLE dedicated_servers ALTER COLUMN gdaemon_port TYPE SMALLINT;
ALTER TABLE servers
    ALTER COLUMN server_port TYPE SMALLINT,
    ALTER COLUMN query_port TYPE SMALLINT,
    ALTER COLUMN rcon_port TYPE SMALLINT;
