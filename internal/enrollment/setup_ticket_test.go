package enrollment

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTicketStore(t *testing.T) *TicketStore {
	t.Helper()

	return NewTicketStore(cache.NewInMemory(), nil)
}

func TestTicketStore_CreateAndResolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	ticket, setupKey, err := store.Create(ctx, CreateTicketInput{
		Owner: "plugin:7",
		Presets: NodePresets{
			Name:     new("hz-fsn1-1"),
			Location: new("fsn1"),
			Metadata: domain.Metadata{"hetzner.server_id": "42"},
		},
		TTL: time.Hour,
	})

	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Len(t, setupKey, ticketIDLength+ticketSecretLength)
	assert.Equal(t, ticket.ID, setupKey[:ticketIDLength], "the key must carry its own ticket id")
	assert.NotContains(t, ticket.SetupKeyHash, setupKey, "the key itself must never be stored")
	assert.Equal(t, TicketStatusPending, ticket.Status)

	resolved, err := store.Resolve(ctx, setupKey)
	require.NoError(t, err)
	assert.Equal(t, ticket.ID, resolved.ID)
	assert.Equal(t, "plugin:7", resolved.Owner)
	require.NotNil(t, resolved.Presets.Name)
	assert.Equal(t, "hz-fsn1-1", *resolved.Presets.Name)
	assert.Equal(t, "42", resolved.Presets.Metadata["hetzner.server_id"])
}

func TestTicketStore_Resolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		key       func(validKey string, ticket *Ticket) string
		wantError string
	}{
		{
			name:      "unknown_id_is_not_found",
			key:       func(_ string, _ *Ticket) string { return "cvhkq1234567890abcdef" },
			wantError: ErrTicketNotFound.Error(),
		},
		{
			name:      "wrong_secret_of_a_known_ticket_is_rejected",
			key:       func(_ string, ticket *Ticket) string { return ticket.ID + "0000000000000000000000000000000x" },
			wantError: ErrInvalidSetupKey.Error(),
		},
		{
			name:      "malformed_key_never_reaches_the_cache",
			key:       func(_ string, _ *Ticket) string { return "short" },
			wantError: ErrTicketNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newTicketStore(t)
			ticket, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
			require.NoError(t, err)

			_, err = store.Resolve(ctx, tt.key(key, ticket))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestTicketStore_TTLBounds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	tests := []struct {
		name      string
		ttl       time.Duration
		wantError string
	}{
		{name: "zero_uses_the_default"},
		{name: "below_minimum_is_rejected", ttl: time.Second, wantError: ErrInvalidTicketTTL.Error()},
		{name: "above_maximum_is_rejected", ttl: 48 * time.Hour, wantError: ErrInvalidTicketTTL.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ticket, _, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: tt.ttl})

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.WithinDuration(t, ticket.CreatedAt.Add(DefaultTicketTTL), ticket.ExpiresAt, time.Second)
		})
	}
}

func TestTicketStore_ExpiredTicketIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	_, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	// The cache entry outlives the ticket only if clocks disagree; the store
	// re-checks ExpiresAt so a stale entry cannot be replayed.
	store.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	_, err = store.Resolve(ctx, key)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSetupKey)
	assert.Contains(t, err.Error(), "expired")
}

func TestTicketStore_ConsumeAndRevoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	ticket, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	require.NoError(t, store.Consume(ctx, ticket, 17))

	stored, err := store.Get(ctx, ticket.ID)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusConsumed, stored.Status)
	assert.Equal(t, uint(17), stored.NodeID)
	require.NotNil(t, stored.ConsumedAt)

	_, err = store.Resolve(ctx, key)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSetupKey, "a consumed ticket must not enroll a second daemon")

	require.NoError(t, store.Revoke(ctx, ticket.ID))

	_, err = store.Get(ctx, ticket.ID)
	assert.ErrorIs(t, err, ErrTicketNotFound)
}

func TestTicketStore_ClaimIsSingleUse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	ticket, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	require.NoError(t, store.Claim(ctx, ticket))

	stored, err := store.Get(ctx, ticket.ID)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusConsumed, stored.Status)
	require.NotNil(t, stored.ConsumedAt)
	assert.Zero(t, stored.NodeID, "the node does not exist yet when the ticket is claimed")

	assert.ErrorIs(t, store.Claim(ctx, ticket), ErrTicketConsumed)

	_, err = store.Resolve(ctx, key)
	assert.ErrorIs(t, err, ErrInvalidSetupKey, "a claimed ticket must not enroll a second daemon")
}

// TestTicketStore_ClaimRejectsAStaleTicket covers the reason Claim re-reads the
// record: the caller holds a *Ticket that Resolve handed out before another
// enrollment consumed it.
func TestTicketStore_ClaimRejectsAStaleTicket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	ticket, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	first, err := store.Resolve(ctx, key)
	require.NoError(t, err)
	second, err := store.Resolve(ctx, key)
	require.NoError(t, err)

	require.NoError(t, store.Claim(ctx, first))
	require.NoError(t, store.Consume(ctx, first, 17))

	assert.ErrorIs(t, store.Claim(ctx, second), ErrTicketConsumed)

	stored, err := store.Get(ctx, ticket.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(17), stored.NodeID, "the losing claim must not overwrite the winner")
}

func TestTicketStore_ClaimRejectsAnExpiredTicket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	ticket, _, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	store.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	err = store.Claim(ctx, ticket)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSetupKey)
	assert.Contains(t, err.Error(), "expired")
}

// TestTicketStore_ConcurrentClaims is the race the lock exists for: many
// daemons presenting the same setup key at once must produce exactly one claim.
func TestTicketStore_ConcurrentClaims(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTicketStore(t)

	_, key, err := store.Create(ctx, CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	const racers = 16

	var (
		start   sync.WaitGroup
		done    sync.WaitGroup
		claimed atomic.Int32
	)

	start.Add(1)
	done.Add(racers)

	for range racers {
		go func() {
			defer done.Done()

			start.Wait()

			ticket, resolveErr := store.Resolve(ctx, key)
			if resolveErr != nil {
				return
			}

			if store.Claim(ctx, ticket) == nil {
				claimed.Add(1)
			}
		}()
	}

	start.Done()
	done.Wait()

	assert.Equal(t, int32(1), claimed.Load(), "exactly one daemon may enroll with a ticket")
}

func TestIsTicketKey(t *testing.T) {
	t.Parallel()

	store := newTicketStore(t)
	ticket, key, err := store.Create(context.Background(), CreateTicketInput{Owner: "plugin:1", TTL: time.Hour})
	require.NoError(t, err)

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "issued_key", key: key, want: true},
		{name: "admin_setup_key_is_not_a_ticket", key: "test-setup-key-32-chars-long1234", want: false},
		{name: "empty", key: "", want: false},
		{name: "id_only", key: ticket.ID, want: false},
		{name: "non_alphanumeric_secret", key: ticket.ID + "0000000000000000000000000000000-", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, IsTicketKey(tt.key))
		})
	}
}
