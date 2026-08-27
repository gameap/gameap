package mysql

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
)

// Up022 folds every stored login and email to lower case.
//
// services.UserService writes both lowercased and lowercases the filter before a
// repository sees it, while the repositories compare exactly so the unique
// indexes stay usable. A row stored with any upper-case character therefore
// stops being reachable by that identifier until it is folded here.
//
// The folding goes through strings.ToLower rather than SQL LOWER() on purpose:
// that is the very function the service applies to the value typed into the
// login form, so the stored spelling and the looked-up spelling agree for every
// alphabet. LOWER() does not agree. SQLite folds ASCII only, so a single
// non-Latin capital is enough to strand a row; MySQL and Postgres follow the
// table collation and the server locale, neither of which this code controls.
//
// Rows already in canonical form are left alone, so the migration is safe to
// re-run.
func Up022(ctx context.Context, tx *sql.Tx) error {
	return normalizeUserIdentifiers(ctx, tx)
}

// Down022 cannot restore the original spelling: case folding is not reversible.
func Down022(_ context.Context, _ *sql.Tx) error {
	return nil
}

// userIdentifiers is one users row reduced to the two columns this migration folds.
type userIdentifiers struct {
	id    int64
	login string
	email string
}

// identifierUpdate is one folded value ready to be written back.
type identifierUpdate struct {
	id    int64
	value string
}

func normalizeUserIdentifiers(ctx context.Context, tx *sql.Tx) error {
	users, err := readUserIdentifiers(ctx, tx)
	if err != nil {
		return err
	}

	logins := planIdentifierFolding(ctx, users, "login", func(u userIdentifiers) string { return u.login })
	emails := planIdentifierFolding(ctx, users, "email", func(u userIdentifiers) string { return u.email })

	err = applyIdentifierFolding(ctx, tx, `UPDATE users SET login = ? WHERE id = ?`, logins)
	if err != nil {
		return err
	}

	return applyIdentifierFolding(ctx, tx, `UPDATE users SET email = ? WHERE id = ?`, emails)
}

func readUserIdentifiers(ctx context.Context, tx *sql.Tx) ([]userIdentifiers, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, login, email FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var users []userIdentifiers

	for rows.Next() {
		var user userIdentifiers

		if err := rows.Scan(&user.id, &user.login, &user.email); err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// planIdentifierFolding works out what one column needs. Rows whose value folds
// to the same string form a group, and only one member of a group can end up
// holding that string, or the unique index would be violated. The member that
// gets it is the one already in canonical form, or the lowest id when there is
// none, so the identifier always resolves to exactly one account instead of to
// none. Every other member keeps its stored spelling and is reported: it is no
// longer reachable by that identifier, and deciding what its value should become
// is an operator's call, not this migration's.
//
// Groups are walked in id order, so both the updates and the warnings are
// reproducible.
func planIdentifierFolding(
	ctx context.Context,
	users []userIdentifiers,
	column string,
	valueOf func(userIdentifiers) string,
) []identifierUpdate {
	order := make([]string, 0, len(users))
	groups := make(map[string][]userIdentifiers, len(users))

	for _, user := range users {
		lowered := strings.ToLower(valueOf(user))

		if _, ok := groups[lowered]; !ok {
			order = append(order, lowered)
		}

		groups[lowered] = append(groups[lowered], user)
	}

	updates := make([]identifierUpdate, 0, len(order))

	for _, lowered := range order {
		group := groups[lowered]
		keeper := pickIdentifierKeeper(group, lowered, valueOf)

		if valueOf(keeper) != lowered {
			updates = append(updates, identifierUpdate{id: keeper.id, value: lowered})
		}

		for _, user := range group {
			if user.id == keeper.id {
				continue
			}

			slog.WarnContext(ctx,
				"Migration 022 could not fold a user identifier: another account already claims "+
					"the lowercase spelling, so this one can no longer be used to sign in and an "+
					"operator has to give the account a different one",
				slog.String("column", column),
				slog.Int64("user_id", user.id),
				slog.String("stored", valueOf(user)),
				slog.String("canonical", lowered),
				slog.Int64("kept_by_user_id", keeper.id),
			)
		}
	}

	return updates
}

// pickIdentifierKeeper returns the row that ends up holding the canonical
// spelling: the one already holding it, or else the lowest id, groups being
// built in id order.
func pickIdentifierKeeper(
	group []userIdentifiers,
	lowered string,
	valueOf func(userIdentifiers) string,
) userIdentifiers {
	for _, user := range group {
		if valueOf(user) == lowered {
			return user
		}
	}

	return group[0]
}

func applyIdentifierFolding(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	updates []identifierUpdate,
) error {
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, query, update.value, update.id); err != nil {
			return err
		}
	}

	return nil
}
