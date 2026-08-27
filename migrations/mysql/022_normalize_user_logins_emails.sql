-- +goose Up

-- services.UserService stores logins and emails lowercased and the repositories
-- compare them exactly, so rows written before that invariant existed would stop
-- matching. Fold them here.
--
-- Unconditional, unlike the postgres and sqlite copies: the default utf8mb4
-- collation is case-insensitive, so users_email_unique and users_login_unique
-- already forbid pairs differing only by case and there is nothing to skip. That
-- same collation makes "WHERE email <> LOWER(email)" always false, and spelling
-- a case-sensitive comparison takes BINARY or CAST, which differ across MySQL
-- versions. The table is small, so rewriting every row is the simpler choice.
-- users.updated_at has no ON UPDATE CURRENT_TIMESTAMP, so timestamps stay put.
UPDATE users SET email = LOWER(email);

UPDATE users SET login = LOWER(login);

-- +goose Down

-- Case folding is not reversible: the original spelling is gone.
SELECT 1;
