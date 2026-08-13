-- +goose Up
-- No-op: MySQL declares dedicated_servers.gdaemon_port and the servers port
-- columns (server_port, query_port, rcon_port) as int(10) unsigned, which
-- already holds the full 1..65535 port range. This migration only reserves
-- version 014 to keep the numbering aligned with the PostgreSQL migration set,
-- where these columns are widened from SMALLINT to INTEGER.

-- +goose Down
-- No-op: see the Up section.
