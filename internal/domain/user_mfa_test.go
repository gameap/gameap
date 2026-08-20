package domain_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUser_MFAFirstShownAt_NilWhenAbsent(t *testing.T) {
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
			user: domain.User{Metadata: domain.Metadata{"mfa_first_shown_at": 12345}},
		},
		{
			name: "unparseable_value",
			user: domain.User{Metadata: domain.Metadata{"mfa_first_shown_at": "not-a-time"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, tt.user.MFAFirstShownAt())
		})
	}
}

func TestUser_SetMFAFirstShownAt_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	user := domain.User{}

	user.SetMFAFirstShownAt(&now)

	require.NotNil(t, user.Metadata)
	assert.Equal(t, "2026-01-02T03:04:05Z", user.Metadata["mfa_first_shown_at"],
		"first-shown time must be stored as an RFC3339 string in the metadata bag")

	got := user.MFAFirstShownAt()
	require.NotNil(t, got)
	assert.True(t, now.Equal(*got), "accessor must return the same instant that was set")
}

func TestUser_SetMFAFirstShownAt_NormalizesToUTC(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+3", 3*60*60)
	local := time.Date(2026, 5, 30, 12, 0, 0, 0, loc)
	user := domain.User{}

	user.SetMFAFirstShownAt(&local)

	assert.Equal(t, "2026-05-30T09:00:00Z", user.Metadata["mfa_first_shown_at"],
		"stored value must be normalised to UTC")
	assert.True(t, local.Equal(*user.MFAFirstShownAt()))
}

func TestUser_SetMFAFirstShownAt_NilClears(t *testing.T) {
	t.Parallel()

	now := time.Now()
	user := domain.User{}
	user.SetMFAFirstShownAt(&now)

	user.SetMFAFirstShownAt(nil)

	_, present := user.Metadata["mfa_first_shown_at"]
	assert.False(t, present, "passing nil must remove the key")
	assert.Nil(t, user.MFAFirstShownAt())
}

func TestUser_MFASnoozedUntil_RoundTrip(t *testing.T) {
	t.Parallel()

	until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	user := domain.User{}

	user.SetMFASnoozedUntil(&until)

	got := user.MFASnoozedUntil()
	require.NotNil(t, got)
	assert.True(t, until.Equal(*got))
}

func TestUser_SetMFAFirstShownAt_PreservesUnrelatedMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	user := domain.User{Metadata: domain.Metadata{"theme": "dark"}}

	user.SetMFAFirstShownAt(&now)

	assert.Equal(t, "dark", user.Metadata["theme"],
		"writing an MFA key must not clobber an unrelated pre-existing metadata entry")
	require.NotNil(t, user.MFAFirstShownAt())
	assert.True(t, now.Equal(*user.MFAFirstShownAt()))
}

func TestUser_MFAAccessors_AreIndependent(t *testing.T) {
	t.Parallel()

	shown := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snoozed := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	user := domain.User{}

	user.SetMFAFirstShownAt(&shown)
	user.SetMFASnoozedUntil(&snoozed)

	require.NotNil(t, user.MFAFirstShownAt())
	require.NotNil(t, user.MFASnoozedUntil())
	assert.True(t, shown.Equal(*user.MFAFirstShownAt()))
	assert.True(t, snoozed.Equal(*user.MFASnoozedUntil()))

	user.SetMFAFirstShownAt(nil)

	assert.Nil(t, user.MFAFirstShownAt(), "clearing first-shown must not affect snoozed-until")
	require.NotNil(t, user.MFASnoozedUntil())
}
