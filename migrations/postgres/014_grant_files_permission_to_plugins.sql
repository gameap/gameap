-- +goose Up

-- The nodefs host module used to be open to every plugin; it is now gated on
-- the "files" grant. Grandfather every existing installation so shipped
-- plugins keep working; new installs receive the grant only when they
-- declare the permission.

UPDATE plugins
SET allowed_permissions = ARRAY['files']
WHERE allowed_permissions IS NULL;

UPDATE plugins
SET allowed_permissions = array_append(allowed_permissions, 'files')
WHERE NOT ('files' = ANY(allowed_permissions));

-- +goose Down

-- The grant becomes part of the operator-managed state; removing it blindly
-- could revoke a deliberately granted permission, so Down is a no-op.
SELECT 1;
