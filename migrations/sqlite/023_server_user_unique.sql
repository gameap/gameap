-- +goose Up

-- server_user carried no uniqueness, so two concurrent attaches of the same
-- pair could each pass the delete-then-insert dance and leave duplicate rows.
-- Collapse whatever duplicates an installation already holds, then let a
-- unique index carry idempotency: AttachUserServer now inserts with
-- ON CONFLICT DO NOTHING against it.

DELETE FROM server_user
WHERE rowid NOT IN (
    SELECT MIN(rowid)
    FROM server_user
    GROUP BY user_id, server_id
);

CREATE UNIQUE INDEX server_user_user_id_server_id_unique
    ON server_user (user_id, server_id);

-- +goose Down

DROP INDEX server_user_user_id_server_id_unique;
