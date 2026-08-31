-- +goose Up

-- server_user carried no uniqueness, so two concurrent attaches of the same
-- pair could each pass the delete-then-insert dance and leave duplicate rows.
-- Collapse whatever duplicates an installation already holds, then let a
-- unique key carry idempotency: AttachUserServer now inserts with
-- ON DUPLICATE KEY UPDATE against it. The table has no primary key, so rows
-- of a duplicated pair are indistinguishable; a temporary auto-increment id
-- tells them apart for the delete and is dropped right after.

ALTER TABLE server_user ADD COLUMN dedup_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY;

DELETE stale FROM server_user AS stale
JOIN (
    SELECT MIN(dedup_id) AS keep_id, user_id, server_id
    FROM server_user
    GROUP BY user_id, server_id
) AS keep
    ON keep.user_id = stale.user_id
   AND keep.server_id = stale.server_id
WHERE stale.dedup_id <> keep.keep_id;

ALTER TABLE server_user DROP COLUMN dedup_id;

ALTER TABLE server_user ADD UNIQUE KEY server_user_user_id_server_id_unique (user_id, server_id);

-- +goose Down

ALTER TABLE server_user DROP KEY server_user_user_id_server_id_unique;
