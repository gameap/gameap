-- +goose Up

-- SHA-256 of the wasm file recorded at install time. It lets an instance tell
-- a re-upload of the same version number apart from the version it already
-- runs, and lets it verify a file it re-downloads without trusting the store's
-- own metadata. NULL on rows installed before this column existed.
ALTER TABLE plugins ADD COLUMN checksum TEXT DEFAULT NULL;

-- +goose Down

ALTER TABLE plugins DROP COLUMN checksum;
