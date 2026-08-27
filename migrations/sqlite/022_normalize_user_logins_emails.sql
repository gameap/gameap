-- +goose Up

-- services.UserService stores logins and emails lowercased and the repositories
-- compare them exactly, so rows written before that invariant existed would stop
-- matching. Fold them here.
--
-- users.email and users.login are declared UNIQUE and SQLite compares TEXT
-- case-sensitively by default, so a pair differing only by case can already be
-- stored. Folding one of those would violate the constraint, so such rows are
-- left as they are: they stay exactly as ambiguous as they are today, and the
-- migration never blocks an upgrade.
UPDATE users SET email = LOWER(email)
WHERE email <> LOWER(email)
  AND NOT EXISTS (
    SELECT 1 FROM users u2
    WHERE u2.id <> users.id AND LOWER(u2.email) = LOWER(users.email)
  );

UPDATE users SET login = LOWER(login)
WHERE login <> LOWER(login)
  AND NOT EXISTS (
    SELECT 1 FROM users u2
    WHERE u2.id <> users.id AND LOWER(u2.login) = LOWER(users.login)
  );

-- +goose Down

-- Case folding is not reversible: the original spelling is gone.
SELECT 1;
