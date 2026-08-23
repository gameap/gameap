-- +goose Up

-- config_schema: the JSON Schema subset the manifest declares for the plugin
-- configuration, copied from PluginInfo on every successful load so the admin
-- UI can render the form while the plugin is not loaded on the answering
-- instance. checksum: sha256 of the wasm recorded at install, verified when a
-- peer instance re-downloads the file and telling a re-upload of the same
-- version apart. generation: bumped by an operator reload so every other
-- instance restarts the module on its next reconcile pass.
ALTER TABLE plugins
    ADD COLUMN config_schema TEXT DEFAULT NULL,
    ADD COLUMN checksum VARCHAR(64) DEFAULT NULL,
    ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE plugins
    DROP COLUMN IF EXISTS config_schema,
    DROP COLUMN IF EXISTS checksum,
    DROP COLUMN IF EXISTS generation;
