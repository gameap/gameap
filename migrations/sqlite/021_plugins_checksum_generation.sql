-- +goose Up

-- checksum: sha256 of the wasm recorded at install, verified when a peer
-- instance re-downloads the file and telling a re-upload of the same version
-- apart. generation: bumped by an operator reload so every other instance
-- restarts the module on its next reconcile pass.
ALTER TABLE plugins ADD COLUMN checksum TEXT DEFAULT NULL;
ALTER TABLE plugins ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE plugins DROP COLUMN checksum;
ALTER TABLE plugins DROP COLUMN generation;
