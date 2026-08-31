-- +goose Up

-- server_user carried no uniqueness, so two concurrent attaches of the same
-- pair could each pass the delete-then-insert dance and leave duplicate rows.
-- Collapse whatever duplicates an installation already holds, then let a
-- unique index carry idempotency: AttachUserServer now inserts with
-- ON CONFLICT DO NOTHING against it. ROW_NUMBER instead of ctid ordering
-- because tid comparison operators only exist since PostgreSQL 14.

-- The panel is multi-instance: while this runs, another instance still on
-- the previous version can be attaching pairs, and a row slipping in
-- between the cleanup and the index build would abort the index build and
-- the whole migration. Blocking writers until commit closes that window.
-- SHARE ROW EXCLUSIVE rather than SHARE because it is self-exclusive: two
-- instances migrating at once would both enter SHARE and then deadlock
-- upgrading to the ACCESS EXCLUSIVE that CREATE INDEX takes.
LOCK TABLE server_user IN SHARE ROW EXCLUSIVE MODE;

DELETE FROM server_user
WHERE ctid IN (
    SELECT ctid FROM (
        SELECT ctid,
               ROW_NUMBER() OVER (PARTITION BY user_id, server_id) AS rn
        FROM server_user
    ) numbered
    WHERE numbered.rn > 1
);

CREATE UNIQUE INDEX server_user_user_id_server_id_unique
    ON server_user (user_id, server_id);

-- +goose Down

DROP INDEX server_user_user_id_server_id_unique;
