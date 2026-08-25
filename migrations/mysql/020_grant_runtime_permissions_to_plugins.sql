-- +goose Up

-- Server control, daemon-task creation, server and server-setting writes are
-- now gated on "manage_servers", node commands on "node_commands" and event
-- subscriptions on "listen_events". Those modules used to be open to every
-- plugin, so grandfather every existing installation (as 015 did for "files");
-- new installs receive only what they declare, and an operator can narrow
-- the grants in the admin UI.

UPDATE plugins
SET allowed_permissions = '["manage_servers", "node_commands", "listen_events"]'
WHERE allowed_permissions IS NULL
   OR allowed_permissions = ''
   OR allowed_permissions = 'null';

UPDATE plugins
SET allowed_permissions = JSON_ARRAY_APPEND(allowed_permissions, '$', 'manage_servers')
WHERE NOT JSON_CONTAINS(allowed_permissions, JSON_QUOTE('manage_servers'));

UPDATE plugins
SET allowed_permissions = JSON_ARRAY_APPEND(allowed_permissions, '$', 'node_commands')
WHERE NOT JSON_CONTAINS(allowed_permissions, JSON_QUOTE('node_commands'));

UPDATE plugins
SET allowed_permissions = JSON_ARRAY_APPEND(allowed_permissions, '$', 'listen_events')
WHERE NOT JSON_CONTAINS(allowed_permissions, JSON_QUOTE('listen_events'));

-- +goose Down

-- The grants become part of the operator-managed state; removing them blindly
-- could revoke a deliberately granted permission, so Down is a no-op.
SELECT 1;
