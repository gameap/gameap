-- +goose Up

-- Global plugin storage entries (no entity) sit outside the reach of the
-- unique index: NULLs never conflict, so every save of such a key used to add
-- a row and reads kept answering with the oldest one. Saves are scope-aware
-- now; this keeps the newest row of every scope and drops the rest.

DELETE FROM plugin_storage
WHERE id NOT IN (
    SELECT MAX(id) FROM plugin_storage
    GROUP BY plugin_id, key, entity_type, entity_id
);

-- +goose Down

-- The dropped rows were stale duplicates; there is nothing to restore.
SELECT 1;
