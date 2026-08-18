-- +goose Up

-- Global plugin storage entries (no entity) sit outside the reach of the
-- unique index: NULLs never conflict, so every save of such a key used to add
-- a row and reads kept answering with the oldest one. Saves are scope-aware
-- now; this keeps the newest row of every scope and drops the rest.

DELETE stale FROM plugin_storage AS stale
JOIN (
    SELECT MAX(id) AS keep_id, plugin_id, `key`, entity_type, entity_id
    FROM plugin_storage
    GROUP BY plugin_id, `key`, entity_type, entity_id
) AS newest
    ON newest.plugin_id = stale.plugin_id
   AND newest.`key` = stale.`key`
   AND newest.entity_type <=> stale.entity_type
   AND newest.entity_id <=> stale.entity_id
WHERE stale.id <> newest.keep_id;

-- +goose Down

-- The dropped rows were stale duplicates; there is nothing to restore.
SELECT 1;
