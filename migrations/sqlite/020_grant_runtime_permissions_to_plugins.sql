-- +goose Up

-- Server control, daemon-task creation, server and server-setting writes are
-- now gated on "manage_servers", node commands on "node_commands" and event
-- subscriptions on "listen_events". Those modules used to be open to every
-- plugin, so grandfather every existing installation (as 015 did for "files");
-- new installs receive only what they declare, and an operator can narrow
-- the grants in the admin UI.

-- The repository stores the column as a BLOB, so it is compared as text:
-- a plain "= 'null'" never matches a BLOB (015 missed such rows, which is
-- why "files" is granted here as well).
UPDATE plugins
SET allowed_permissions = '["files","manage_servers","node_commands","listen_events"]'
WHERE allowed_permissions IS NULL
   OR CAST(allowed_permissions AS TEXT) IN ('', 'null', '[]');

UPDATE plugins
SET allowed_permissions = json_insert(allowed_permissions, '$[#]', 'manage_servers')
WHERE NOT EXISTS (
    SELECT 1 FROM json_each(plugins.allowed_permissions)
    WHERE json_each.value = 'manage_servers'
);

UPDATE plugins
SET allowed_permissions = json_insert(allowed_permissions, '$[#]', 'node_commands')
WHERE NOT EXISTS (
    SELECT 1 FROM json_each(plugins.allowed_permissions)
    WHERE json_each.value = 'node_commands'
);

UPDATE plugins
SET allowed_permissions = json_insert(allowed_permissions, '$[#]', 'listen_events')
WHERE NOT EXISTS (
    SELECT 1 FROM json_each(plugins.allowed_permissions)
    WHERE json_each.value = 'listen_events'
);

-- +goose Down

-- The grants become part of the operator-managed state; removing them blindly
-- could revoke a deliberately granted permission, so Down is a no-op.
SELECT 1;
