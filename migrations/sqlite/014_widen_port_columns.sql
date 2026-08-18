-- +goose Up
-- No-op: SQLite stores INTEGER values using a variable-length encoding of up to
-- 8 bytes (64-bit signed), so dedicated_servers.gdaemon_port and the servers
-- port columns (server_port, query_port, rcon_port) already hold the full
-- 1..65535 port range without any schema change. This migration only reserves
-- version 014 to keep the numbering aligned with the PostgreSQL migration set,
-- where these columns are widened from SMALLINT to INTEGER.

-- +goose Down
-- No-op: see the Up section.
