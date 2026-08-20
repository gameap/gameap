package domain_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUser_PasswordChangedAt_NilWhenAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user domain.User
	}{
		{
			name: "nil_metadata",
			user: domain.User{},
		},
		{
			name: "empty_metadata",
			user: domain.User{Metadata: domain.Metadata{}},
		},
		{
			name: "non_string_value",
			user: domain.User{Metadata: domain.Metadata{"password_changed_at": 12345}},
		},
		{
			name: "unparseable_value",
			user: domain.User{Metadata: domain.Metadata{"password_changed_at": "not-a-time"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ACT & ASSERT
			assert.Nil(t, tt.user.PasswordChangedAt(),
				"a missing or unreadable password_changed_at must read as nil (accounts predating tracking)")
		})
	}
}

func TestUser_SetPasswordChangedAt_RoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	user := domain.User{}

	// ACT
	user.SetPasswordChangedAt(&now)

	// ASSERT
	require.NotNil(t, user.Metadata)
	assert.Equal(t, "2026-03-04T05:06:07Z", user.Metadata["password_changed_at"],
		"password-change time must be stored as an RFC3339 string in the metadata bag")

	got := user.PasswordChangedAt()
	require.NotNil(t, got)
	assert.True(t, now.Equal(*got), "accessor must return the same instant that was set")
}

func TestUser_SetPasswordChangedAt_NormalizesToUTC(t *testing.T) {
	t.Parallel()

	// ARRANGE
	loc := time.FixedZone("UTC+3", 3*60*60)
	local := time.Date(2026, 5, 30, 12, 0, 0, 0, loc)
	user := domain.User{}

	// ACT
	user.SetPasswordChangedAt(&local)

	// ASSERT
	assert.Equal(t, "2026-05-30T09:00:00Z", user.Metadata["password_changed_at"],
		"stored value must be normalised to UTC")
	assert.True(t, local.Equal(*user.PasswordChangedAt()))
}

func TestUser_SetPasswordChangedAt_NilClears(t *testing.T) {
	t.Parallel()

	// ARRANGE
	now := time.Now()
	user := domain.User{}
	user.SetPasswordChangedAt(&now)

	// ACT
	user.SetPasswordChangedAt(nil)

	// ASSERT
	_, present := user.Metadata["password_changed_at"]
	assert.False(t, present, "passing nil must remove the key")
	assert.Nil(t, user.PasswordChangedAt())
}

func TestUser_SetPasswordChangedAt_NilOnNilMetadataIsNoOp(t *testing.T) {
	t.Parallel()

	// ARRANGE
	user := domain.User{}

	// ACT — clearing an absent value must not panic nor allocate the bag.
	user.SetPasswordChangedAt(nil)

	// ASSERT
	assert.Nil(t, user.PasswordChangedAt())
	assert.Nil(t, user.Metadata, "clearing an absent key must not allocate the metadata bag")
}

func TestUser_SetPasswordChangedAt_RepeatedSetOverwrites(t *testing.T) {
	t.Parallel()

	// ARRANGE
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	user := domain.User{}

	// ACT
	user.SetPasswordChangedAt(&first)
	user.SetPasswordChangedAt(&second)

	// ASSERT
	assert.Equal(t, "2026-02-02T00:00:00Z", user.Metadata["password_changed_at"],
		"a second Set must overwrite the previously stored timestamp")
	got := user.PasswordChangedAt()
	require.NotNil(t, got)
	assert.True(t, second.Equal(*got))
}

func TestUser_SetPasswordChangedAt_PreservesUnrelatedMetadata(t *testing.T) {
	t.Parallel()

	// ARRANGE
	shown := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	changed := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	user := domain.User{Metadata: domain.Metadata{"theme": "dark"}}
	user.SetMFAFirstShownAt(&shown)

	// ACT
	user.SetPasswordChangedAt(&changed)

	// ASSERT — writing the password key must not clobber unrelated entries.
	assert.Equal(t, "dark", user.Metadata["theme"])
	require.NotNil(t, user.MFAFirstShownAt())
	assert.True(t, shown.Equal(*user.MFAFirstShownAt()),
		"the MFA first-shown timestamp must survive a password-change write")
	require.NotNil(t, user.PasswordChangedAt())
	assert.True(t, changed.Equal(*user.PasswordChangedAt()))
}
