-- +goose Up
-- No-op: SQLite stores INTEGER values using a variable-length encoding of up to
-- 8 bytes (64-bit signed), so servers.ram_limit already holds the full byte range
-- without any schema change. This migration only reserves version 012 to keep the
-- numbering aligned with the MySQL and PostgreSQL migration sets.

-- +goose Down
-- No-op: see the Up section.
