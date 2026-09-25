package testing

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IdempotencyKeyRepositorySuite struct {
	suite.Suite

	repo repositories.IdempotencyKeyRepository
	fn   func(t *testing.T) repositories.IdempotencyKeyRepository
}

func NewIdempotencyKeyRepositorySuite(
	fn func(t *testing.T) repositories.IdempotencyKeyRepository,
) *IdempotencyKeyRepositorySuite {
	return &IdempotencyKeyRepositorySuite{
		fn: fn,
	}
}

func (s *IdempotencyKeyRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

// idempotencyBaseTime is millisecond-aligned: MySQL keeps DATETIME(3) and
// SQLite keeps expires_at as unix milliseconds.
var idempotencyBaseTime = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) //nolint:gochecknoglobals

func newIdempotencyKey(userID uint, keyHash string, createdAt time.Time, ttl time.Duration) *domain.IdempotencyKey {
	return &domain.IdempotencyKey{
		UserID:             userID,
		KeyHash:            keyHash,
		RequestMethod:      "POST",
		RequestPath:        "/api/servers",
		RequestFingerprint: "fingerprint-" + keyHash,
		ResponseStatus:     201,
		ResponseHeaders: map[string][]string{
			"Content-Type": {"application/json"},
			"Location":     {"/api/servers/7"},
		},
		ResponseBody: []byte(`{"message":"success","result":{"serverId":7}}`),
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt.Add(ttl),
	}
}

func (s *IdempotencyKeyRepositorySuite) TestIdempotencyKeyRepositorySave() {
	ctx := context.Background()

	s.T().Run("saves_new_record_and_assigns_id", func(t *testing.T) {
		record := newIdempotencyKey(1, "save-new", idempotencyBaseTime, time.Hour)

		saved, err := s.repo.Save(ctx, record)
		require.NoError(t, err)
		assert.True(t, saved)
		assert.NotZero(t, record.ID)

		found, err := s.repo.Find(ctx, 1, "save-new", idempotencyBaseTime)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, record.ID, found.ID)
		assert.Equal(t, uint(1), found.UserID)
		assert.Equal(t, "save-new", found.KeyHash)
		assert.Equal(t, "POST", found.RequestMethod)
		assert.Equal(t, "/api/servers", found.RequestPath)
		assert.Equal(t, "fingerprint-save-new", found.RequestFingerprint)
		assert.Equal(t, 201, found.ResponseStatus)
		assert.Equal(t, map[string][]string{
			"Content-Type": {"application/json"},
			"Location":     {"/api/servers/7"},
		}, found.ResponseHeaders)
		assert.Equal(t, `{"message":"success","result":{"serverId":7}}`, string(found.ResponseBody))
		assert.WithinDuration(t, idempotencyBaseTime, found.CreatedAt, time.Millisecond)
		assert.WithinDuration(t, idempotencyBaseTime.Add(time.Hour), found.ExpiresAt, time.Millisecond)
	})

	s.T().Run("keeps_live_record_and_reports_false", func(t *testing.T) {
		first := newIdempotencyKey(2, "save-live", idempotencyBaseTime, time.Hour)
		saved, err := s.repo.Save(ctx, first)
		require.NoError(t, err)
		require.True(t, saved)

		second := newIdempotencyKey(2, "save-live", idempotencyBaseTime.Add(time.Minute), time.Hour)
		second.ResponseStatus = 500
		second.ResponseBody = []byte(`{"status":"error"}`)

		saved, err = s.repo.Save(ctx, second)
		require.NoError(t, err)
		assert.False(t, saved)

		found, err := s.repo.Find(ctx, 2, "save-live", idempotencyBaseTime.Add(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, first.ID, found.ID)
		assert.Equal(t, 201, found.ResponseStatus)
		assert.Equal(t, `{"message":"success","result":{"serverId":7}}`, string(found.ResponseBody))
	})

	s.T().Run("replaces_expired_record", func(t *testing.T) {
		first := newIdempotencyKey(3, "save-expired", idempotencyBaseTime, time.Hour)
		saved, err := s.repo.Save(ctx, first)
		require.NoError(t, err)
		require.True(t, saved)

		secondCreatedAt := idempotencyBaseTime.Add(time.Hour)
		second := newIdempotencyKey(3, "save-expired", secondCreatedAt, time.Hour)
		second.RequestFingerprint = "fingerprint-replacement"
		second.ResponseStatus = 200
		second.ResponseBody = []byte(`{"status":"ok"}`)

		saved, err = s.repo.Save(ctx, second)
		require.NoError(t, err)
		assert.True(t, saved)

		found, err := s.repo.Find(ctx, 3, "save-expired", secondCreatedAt)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "fingerprint-replacement", found.RequestFingerprint)
		assert.Equal(t, 200, found.ResponseStatus)
		assert.Equal(t, `{"status":"ok"}`, string(found.ResponseBody))
		assert.WithinDuration(t, secondCreatedAt.Add(time.Hour), found.ExpiresAt, time.Millisecond)
	})

	s.T().Run("stores_empty_body_and_headers", func(t *testing.T) {
		record := newIdempotencyKey(4, "save-empty", idempotencyBaseTime, time.Hour)
		record.ResponseStatus = 204
		record.ResponseHeaders = nil
		record.ResponseBody = nil

		saved, err := s.repo.Save(ctx, record)
		require.NoError(t, err)
		require.True(t, saved)

		found, err := s.repo.Find(ctx, 4, "save-empty", idempotencyBaseTime)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, 204, found.ResponseStatus)
		assert.Empty(t, found.ResponseHeaders)
		assert.NotNil(t, found.ResponseHeaders)
		assert.Empty(t, found.ResponseBody)
	})

	s.T().Run("scopes_keys_by_user", func(t *testing.T) {
		first := newIdempotencyKey(5, "save-shared", idempotencyBaseTime, time.Hour)
		second := newIdempotencyKey(6, "save-shared", idempotencyBaseTime, time.Hour)
		second.ResponseBody = []byte(`{"message":"success","result":{"serverId":8}}`)

		saved, err := s.repo.Save(ctx, first)
		require.NoError(t, err)
		require.True(t, saved)

		saved, err = s.repo.Save(ctx, second)
		require.NoError(t, err)
		require.True(t, saved)

		foundFirst, err := s.repo.Find(ctx, 5, "save-shared", idempotencyBaseTime)
		require.NoError(t, err)
		require.NotNil(t, foundFirst)
		assert.JSONEq(t, `{"message":"success","result":{"serverId":7}}`, string(foundFirst.ResponseBody))

		foundSecond, err := s.repo.Find(ctx, 6, "save-shared", idempotencyBaseTime)
		require.NoError(t, err)
		require.NotNil(t, foundSecond)
		assert.JSONEq(t, `{"message":"success","result":{"serverId":8}}`, string(foundSecond.ResponseBody))
	})
}

func (s *IdempotencyKeyRepositorySuite) TestIdempotencyKeyRepositoryFind() {
	ctx := context.Background()

	s.T().Run("returns_nil_for_unknown_key", func(t *testing.T) {
		found, err := s.repo.Find(ctx, 10, "find-unknown", idempotencyBaseTime)
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	s.T().Run("returns_record_until_expiry", func(t *testing.T) {
		record := newIdempotencyKey(11, "find-expiry", idempotencyBaseTime, time.Hour)
		saved, err := s.repo.Save(ctx, record)
		require.NoError(t, err)
		require.True(t, saved)

		found, err := s.repo.Find(ctx, 11, "find-expiry", record.ExpiresAt.Add(-time.Millisecond))
		require.NoError(t, err)
		assert.NotNil(t, found)

		found, err = s.repo.Find(ctx, 11, "find-expiry", record.ExpiresAt)
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	s.T().Run("returns_nil_for_other_user", func(t *testing.T) {
		record := newIdempotencyKey(12, "find-owner", idempotencyBaseTime, time.Hour)
		saved, err := s.repo.Save(ctx, record)
		require.NoError(t, err)
		require.True(t, saved)

		found, err := s.repo.Find(ctx, 13, "find-owner", idempotencyBaseTime)
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func (s *IdempotencyKeyRepositorySuite) TestIdempotencyKeyRepositoryConcurrentSave() {
	for _, expired := range []bool{false, true} {
		name := "new_key"
		if expired {
			name = "expired_key"
		}

		s.T().Run(name, func(t *testing.T) {
			ctx := t.Context()
			if expired {
				previous := newIdempotencyKey(30, name, idempotencyBaseTime.Add(-time.Hour), time.Hour)
				saved, err := s.repo.Save(ctx, previous)
				require.NoError(t, err)
				require.True(t, saved)
			}

			type result struct {
				record *domain.IdempotencyKey
				saved  bool
				err    error
			}

			const contenders = 8
			start := make(chan struct{})
			results := make(chan result, contenders)
			for i := range contenders {
				go func() {
					record := newIdempotencyKey(30, name, idempotencyBaseTime, time.Hour)
					record.ResponseBody = []byte(strconv.Itoa(i))
					<-start
					saved, err := s.repo.Save(ctx, record)
					results <- result{record: record, saved: saved, err: err}
				}()
			}
			close(start)

			var winners []*domain.IdempotencyKey
			for range contenders {
				result := <-results
				require.NoError(t, result.err)
				if result.saved {
					winners = append(winners, result.record)
				}
			}
			require.Len(t, winners, 1)

			found, err := s.repo.Find(ctx, 30, name, idempotencyBaseTime)
			require.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, winners[0].ResponseBody, found.ResponseBody)
		})
	}
}

func (s *IdempotencyKeyRepositorySuite) TestIdempotencyKeyRepositoryDeleteExpired() {
	ctx := context.Background()

	s.T().Run("deletes_only_expired_records", func(t *testing.T) {
		// A separate time range keeps rows of the other subtests out of the count.
		base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

		for _, record := range []*domain.IdempotencyKey{
			newIdempotencyKey(20, "delete-expired-1", base, time.Minute),
			newIdempotencyKey(20, "delete-expired-2", base, 2*time.Minute),
			newIdempotencyKey(20, "delete-live", base, time.Hour),
		} {
			saved, err := s.repo.Save(ctx, record)
			require.NoError(t, err)
			require.True(t, saved)
		}

		now := base.Add(2 * time.Minute)

		deleted, err := s.repo.DeleteExpired(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 2, deleted)

		live, err := s.repo.Find(ctx, 20, "delete-live", now)
		require.NoError(t, err)
		assert.NotNil(t, live)

		// The row is gone, not merely hidden: a lookup from before its expiry
		// no longer finds it.
		expired, err := s.repo.Find(ctx, 20, "delete-expired-1", base)
		require.NoError(t, err)
		assert.Nil(t, expired)
	})
}
