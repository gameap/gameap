package testing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	normalizeIdentifiersVersion       = 22
	versionBeforeNormalizeIdentifiers = normalizeIdentifiersVersion - 1
)

// userIdentifierSeed is a users row as an installation from before the lowercase
// invariant could have stored it, together with what 022 has to leave behind.
type userIdentifierSeed struct {
	id           int64
	login        string
	email        string
	wantLogin    string
	wantEmail    string
	caseOnlyTwin bool
	why          string
}

// RunNormalizeUserIdentifiersMigrationTest checks migration 022 on a real
// database.
//
// The migration folds with strings.ToLower, the same function
// services.UserService applies to what is typed into the login form, so the
// stored spelling and the looked-up spelling have to agree on every alphabet
// rather than on ASCII alone. The assertions are therefore about reachability by
// exact comparison, which is what the repositories do, not merely about what
// each column ends up holding.
//
// db must be empty; driver is the DATABASE_DRIVER value. caseOnlyDuplicatesStorable
// says whether this backend can hold two rows whose identifiers differ only by
// case: Postgres and SQLite compare their unique indexes case-sensitively and
// can, MySQL under a case-insensitive collation rejects the second INSERT, so
// those rows are left out of the seed there. That same collation also makes the
// reachability checks weaker on MySQL, since its comparison ignores case
// anyway; Postgres and SQLite are where they carry the signal.
func RunNormalizeUserIdentifiersMigrationTest(
	t *testing.T,
	db *sql.DB,
	driver string,
	caseOnlyDuplicatesStorable bool,
) {
	t.Helper()

	ctx := context.Background()

	provider, err := migrations.NewProvider(ctx, migrationContainer{
		db:  db,
		cfg: &config.Config{DatabaseDriver: driver},
	})
	require.NoError(t, err)

	_, err = provider.UpTo(ctx, versionBeforeNormalizeIdentifiers)
	require.NoError(t, err)

	seeds := []userIdentifierSeed{
		{
			id: 9001, login: "MixedLogin", email: "Mixed@Example.COM",
			wantLogin: "mixedlogin", wantEmail: "mixed@example.com",
			why: "plain ASCII mixed case folds",
		},
		{
			id: 9002, login: "plainlogin", email: "plain@example.com",
			wantLogin: "plainlogin", wantEmail: "plain@example.com",
			why: "already canonical, must not be rewritten",
		},
		{
			// SQL LOWER() folds ASCII only on SQLite, so this row alone -- no
			// duplicate involved -- is what the SQL version of 022 silently left
			// unreachable.
			id: 9003, login: "Пётр", email: "Пётр@Example.com",
			wantLogin: "пётр", wantEmail: "пётр@example.com",
			why: "non-Latin capitals fold like the runtime folds them",
		},
		{
			id: 9004, login: "dupone", email: "Dup@Example.com",
			wantLogin: "dupone", wantEmail: "Dup@Example.com",
			caseOnlyTwin: true,
			why:          "loses the address to the row that already holds it lowercase",
		},
		{
			id: 9005, login: "duptwo", email: "dup@example.com",
			wantLogin: "duptwo", wantEmail: "dup@example.com",
			caseOnlyTwin: true,
			why:          "already canonical, so it keeps the address",
		},
		{
			id: 9006, login: "bobone", email: "Bob@x.com",
			wantLogin: "bobone", wantEmail: "bob@x.com",
			caseOnlyTwin: true,
			why:          "neither twin is canonical, so the lowest id takes the address",
		},
		{
			// Its login has no twin, so it folds even though its email cannot:
			// the two columns are decided independently.
			id: 9007, login: "BobTwo", email: "BOB@x.com",
			wantLogin: "bobtwo", wantEmail: "BOB@x.com",
			caseOnlyTwin: true,
			why:          "loses the address but still gets its login folded",
		},
		{
			id: 9008, login: "Twin", email: "twin-a@example.com",
			wantLogin: "Twin", wantEmail: "twin-a@example.com",
			caseOnlyTwin: true,
			why:          "logins collide the same way emails do",
		},
		{
			id: 9009, login: "twin", email: "twin-b@example.com",
			wantLogin: "twin", wantEmail: "twin-b@example.com",
			caseOnlyTwin: true,
			why:          "already canonical, so it keeps the login",
		},
	}

	for _, seed := range seeds {
		if seed.caseOnlyTwin && !caseOnlyDuplicatesStorable {
			continue
		}

		seedUserIdentifierRow(ctx, t, db, driver, seed)
	}

	logs := captureSlogWarnings(t)

	_, err = provider.UpTo(ctx, normalizeIdentifiersVersion)
	require.NoError(t, err, "a case-only duplicate must not abort the migration")

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	for _, seed := range seeds {
		if seed.caseOnlyTwin && !caseOnlyDuplicatesStorable {
			continue
		}

		login, email := readUserIdentifierRow(ctx, t, db, driver, seed.id)
		assert.Equalf(t, seed.wantLogin, login, "login of %d: %s", seed.id, seed.why)
		assert.Equalf(t, seed.wantEmail, email, "email of %d: %s", seed.id, seed.why)
	}

	// The invariant that actually matters: every canonical identifier resolves
	// through the exact comparison the repositories use, and resolves to exactly
	// one account. "bob@x.com" is the case the SQL version of 022 left resolving
	// to nobody at all.
	assertResolvesTo(ctx, t, db, driver, "email", "mixed@example.com", 9001)
	assertResolvesTo(ctx, t, db, driver, "email", "plain@example.com", 9002)
	assertResolvesTo(ctx, t, db, driver, "email", "пётр@example.com", 9003)
	assertResolvesTo(ctx, t, db, driver, "login", "пётр", 9003)

	wantWarnings := []identifierWarning{}

	if caseOnlyDuplicatesStorable {
		assertResolvesTo(ctx, t, db, driver, "email", "dup@example.com", 9005)
		assertResolvesTo(ctx, t, db, driver, "email", "bob@x.com", 9006)
		assertResolvesTo(ctx, t, db, driver, "login", "twin", 9009)

		wantWarnings = []identifierWarning{
			{Column: "login", UserID: 9008, KeptBy: 9009},
			{Column: "email", UserID: 9004, KeptBy: 9005},
			{Column: "email", UserID: 9007, KeptBy: 9006},
		}
	}

	// Exactly these, in this order: a row that quietly loses its identifier is
	// the failure this migration exists to make visible, and a warning about a
	// row that kept its identifier would train operators to ignore them.
	assert.Equal(t, wantWarnings, collectIdentifierWarnings(t, logs))

	assertNoIdentifiersLogged(t, logs, seeds)
}

func seedUserIdentifierRow(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
	seed userIdentifierSeed,
) {
	t.Helper()

	query, args, err := sq.Insert("users").
		Columns("id", "login", "email", "password").
		Values(seed.id, seed.login, seed.email, "hash").
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.NoErrorf(t, err, "seeding %d", seed.id)
}

func readUserIdentifierRow(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
	id int64,
) (string, string) {
	t.Helper()

	query, args, err := sq.Select("login", "email").From("users").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	var login, email string
	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&login, &email))

	return login, email
}

// assertResolvesTo repeats what a repository does -- an exact comparison, no
// LOWER() -- and insists the canonical identifier reaches one account.
func assertResolvesTo(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
	column, value string,
	wantID int64,
) {
	t.Helper()

	query, args, err := sq.Select("id").From("users").
		Where(sq.Eq{column: value}).
		OrderBy("id").
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	rows, err := db.QueryContext(ctx, query, args...)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var matched []int64

	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))

		matched = append(matched, id)
	}

	require.NoError(t, rows.Err())
	assert.Equalf(t, []int64{wantID}, matched, "%s %q must reach exactly one account", column, value)
}

func driverPlaceholders(driver string) sq.PlaceholderFormat {
	if driver == "postgres" || driver == "pgx" {
		return sq.Dollar
	}

	return sq.Question
}

// identifierWarning is the payload migration 022 logs for a row it could not
// fold: the column, and the pair of ids an operator needs to act on. Deliberately
// no login or email — see assertNoIdentifiersLogged.
type identifierWarning struct {
	Column string `json:"column"`
	UserID int64  `json:"user_id"`
	KeptBy int64  `json:"kept_by_user_id"`
}

// assertNoIdentifiersLogged is the regression guard for the fields this warning
// deliberately omits. A login or an email address is personal data; the journal
// is shipped, retained and read far more widely than the users table, so no
// seeded identifier may appear in it — not even the one that could not be
// folded, which is the tempting one to include.
func assertNoIdentifiersLogged(t *testing.T, logs *bytes.Buffer, seeds []userIdentifierSeed) {
	t.Helper()

	journal := logs.String()

	for _, seed := range seeds {
		assert.NotContainsf(t, journal, seed.login, "login of %d leaked into the journal", seed.id)
		assert.NotContainsf(t, journal, seed.email, "email of %d leaked into the journal", seed.id)
	}
}

// captureSlogWarnings redirects the default logger, which is where a migration
// can reach a logger from at all, into a buffer for the rest of the test. The
// repository packages run their tests sequentially (paralleltest is disabled for
// them), so swapping the process-wide default here races with nothing.
func captureSlogWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return buf
}

func collectIdentifierWarnings(t *testing.T, logs *bytes.Buffer) []identifierWarning {
	t.Helper()

	warnings := []identifierWarning{}

	for line := range bytes.SplitSeq(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		var record struct {
			identifierWarning

			Level string `json:"level"`
		}

		require.NoErrorf(t, json.Unmarshal(line, &record), "log line %q", line)

		if record.Level != slog.LevelWarn.String() {
			continue
		}

		warnings = append(warnings, record.identifierWarning)
	}

	return warnings
}
