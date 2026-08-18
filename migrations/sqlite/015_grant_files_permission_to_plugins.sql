-- +goose Up

-- The nodefs host module used to be open to every plugin; it is now gated on
-- the "files" grant. Grandfather every existing installation so shipped
-- plugins keep working; new installs receive the grant only when they
-- declare the permission.

UPDATE plugins
SET allowed_permissions = '["files"]'
WHERE allowed_permissions IS NULL
   OR allowed_permissions = ''
   OR allowed_permissions = 'null';

UPDATE plugins
SET allowed_permissions = json_insert(allowed_permissions, '$[#]', 'files')
WHERE NOT EXISTS (
    SELECT 1 FROM json_each(plugins.allowed_permissions)
    WHERE json_each.value = 'files'
);

-- +goose Down

-- The grant becomes part of the operator-managed state; removing it blindly
-- could revoke a deliberately granted permission, so Down is a no-op.
SELECT 1;
