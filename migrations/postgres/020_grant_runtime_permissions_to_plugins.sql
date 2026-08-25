-- +goose Up

-- Server control, daemon-task creation, server and server-setting writes are
-- now gated on "manage_servers", node commands on "node_commands" and event
-- subscriptions on "listen_events". Those modules used to be open to every
-- plugin, so grandfather every existing installation (as 015 did for "files");
-- new installs receive only what they declare, and an operator can narrow
-- the grants in the admin UI.

UPDATE plugins
SET allowed_permissions = ARRAY['manage_servers', 'node_commands', 'listen_events']
WHERE allowed_permissions IS NULL;

-- array_position instead of NOT ('x' = ANY(...)): with a NULL element in the
-- array the ANY comparison yields NULL, NOT NULL stays NULL and the row would
-- silently miss the grant.
UPDATE plugins
SET allowed_permissions = array_append(allowed_permissions, 'manage_servers')
WHERE array_position(allowed_permissions, 'manage_servers') IS NULL;

UPDATE plugins
SET allowed_permissions = array_append(allowed_permissions, 'node_commands')
WHERE array_position(allowed_permissions, 'node_commands') IS NULL;

UPDATE plugins
SET allowed_permissions = array_append(allowed_permissions, 'listen_events')
WHERE array_position(allowed_permissions, 'listen_events') IS NULL;

-- +goose Down

-- The grants become part of the operator-managed state; removing them blindly
-- could revoke a deliberately granted permission, so Down is a no-op.
SELECT 1;
