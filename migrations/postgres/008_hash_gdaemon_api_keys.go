package postgres

import (
	"context"
	"database/sql"

	pkgstrings "github.com/gameap/gameap/pkg/strings"
)

// Up008 hashes any existing plaintext gdaemon_api_key values with SHA-256 so a
// database leak no longer exposes a usable daemon credential (the same threat
// model already applied to gdaemon_api_token in migration 007). Daemons keep
// authenticating with the plaintext key they were issued: the gRPC interceptor,
// gateway register check and /gdaemon_api/get_token now hash the presented key
// before comparison/lookup.
//
// Rows whose stored value already looks like a SHA-256 hex digest are left
// untouched, making the migration safe to re-run.
func Up008(ctx context.Context, tx *sql.Tx) error {
	return hashGDaemonAPIKeys(ctx, tx)
}

// Down008 is intentionally a no-op. SHA-256 is one-way so the original keys
// cannot be restored; blanking every gdaemon_api_key on rollback would also
// destroy rows that pre-dated this migration and were never touched by Up008,
// turning a quick same-day rollback into a fleet-wide re-enroll. The hashed
// values are left in place: Up008 is idempotent (already-hashed rows are
// skipped), so re-applying after a rollback is safe, and any node that still
// needs its original plaintext key must re-enroll via
// /gdaemon_api/get_token regardless of what this function does.
func Down008(_ context.Context, _ *sql.Tx) error {
	return nil
}

func hashGDaemonAPIKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, gdaemon_api_key FROM dedicated_servers WHERE gdaemon_api_key IS NOT NULL AND gdaemon_api_key != ''`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type pending struct {
		id  int64
		key string
	}

	var toUpdate []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.key); err != nil {
			return err
		}

		if pkgstrings.IsSHA256Hex(p.key) {
			continue
		}

		p.key = pkgstrings.SHA256(p.key)
		toUpdate = append(toUpdate, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range toUpdate {
		if _, err := tx.ExecContext(ctx,
			`UPDATE dedicated_servers SET gdaemon_api_key = $1 WHERE id = $2`, p.key, p.id); err != nil {
			return err
		}
	}

	return nil
}
