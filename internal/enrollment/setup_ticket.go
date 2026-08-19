package enrollment

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const (
	setupTicketCachePrefix = "daemon:enroll_ticket:"

	// ticketSecretLength is the random half of a ticket key; the other half is
	// the xid that identifies the cache entry.
	ticketSecretLength = 32
	ticketIDLength     = 20

	// DefaultTicketTTL matches the admin setup key, MaxTicketTTL bounds what a
	// plugin may ask for.
	DefaultTicketTTL = setupKeyTTL
	MinTicketTTL     = time.Minute
	MaxTicketTTL     = 24 * time.Hour

	// consumedTicketRetention keeps a used ticket readable so the plugin that
	// issued it can still learn which node it produced.
	consumedTicketRetention = 24 * time.Hour
)

var (
	ErrTicketNotFound   = errors.New("enrollment ticket not found")
	ErrInvalidTicketTTL = errors.New("enrollment ticket ttl out of range")

	// These wrap ErrInvalidSetupKey so the gRPC gateway keeps mapping a bad
	// enrollment attempt to codes.Unauthenticated without knowing about tickets.
	ErrTicketConsumed = errors.WithMessage(ErrInvalidSetupKey, "enrollment ticket already used")
	ErrTicketExpired  = errors.WithMessage(ErrInvalidSetupKey, "enrollment ticket expired")
)

type TicketStatus string

const (
	TicketStatusPending  TicketStatus = "pending"
	TicketStatusConsumed TicketStatus = "consumed"
)

// NodePresets are written onto the node the panel creates when a daemon
// enrolls with the ticket. Unset fields keep the enrollment defaults.
type NodePresets struct {
	Enabled      *bool           `json:"enabled,omitempty"`
	Name         *string         `json:"name,omitempty"`
	Location     *string         `json:"location,omitempty"`
	Provider     *string         `json:"provider,omitempty"`
	WorkPath     *string         `json:"work_path,omitempty"`
	SteamcmdPath *string         `json:"steamcmd_path,omitempty"`
	Metadata     domain.Metadata `json:"metadata,omitempty"`
}

// ApplyTo overwrites the enrollment defaults with the preset values.
func (p NodePresets) ApplyTo(node *domain.Node) {
	patch := domain.NodePatch{
		Enabled:      p.Enabled,
		Name:         p.Name,
		Location:     p.Location,
		Provider:     p.Provider,
		WorkPath:     p.WorkPath,
		SteamcmdPath: p.SteamcmdPath,
		Metadata:     p.Metadata,
	}

	patch.ApplyTo(node)
}

// Validate rejects presets the node model would not accept.
func (p NodePresets) Validate() error {
	patch := domain.NodePatch{
		Enabled:      p.Enabled,
		Name:         p.Name,
		Location:     p.Location,
		Provider:     p.Provider,
		WorkPath:     p.WorkPath,
		SteamcmdPath: p.SteamcmdPath,
		Metadata:     p.Metadata,
	}

	return patch.Validate()
}

// Ticket is a single-use enrollment credential issued to one requester. The
// key itself is never stored: the record keeps only its digest, so a cache
// dump cannot be replayed against the enrollment endpoint.
type Ticket struct {
	ID           string       `json:"id"`
	SetupKeyHash string       `json:"setup_key_hash"`
	Owner        string       `json:"owner"`
	Presets      NodePresets  `json:"presets"`
	Status       TicketStatus `json:"status"`
	NodeID       uint         `json:"node_id,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	ExpiresAt    time.Time    `json:"expires_at"`
	ConsumedAt   *time.Time   `json:"consumed_at,omitempty"`
}

// CreateTicketInput describes a ticket to mint. Owner identifies the issuer
// ("plugin:<id>") so one plugin cannot inspect or revoke another's tickets.
type CreateTicketInput struct {
	Owner   string
	Presets NodePresets
	TTL     time.Duration
}

// TicketStore keeps enrollment tickets in the shared cache, so a daemon may
// enroll against any panel instance regardless of which one issued the key.
type TicketStore struct {
	cache cache.Cache
	now   func() time.Time
}

func NewTicketStore(c cache.Cache) *TicketStore {
	return &TicketStore{cache: c, now: time.Now}
}

// Create mints a ticket and returns it together with the setup key, which is
// the only time the key exists outside the caller.
func (s *TicketStore) Create(ctx context.Context, in CreateTicketInput) (*Ticket, string, error) {
	ttl := in.TTL
	if ttl == 0 {
		ttl = DefaultTicketTTL
	}

	if ttl < MinTicketTTL || ttl > MaxTicketTTL {
		return nil, "", ErrInvalidTicketTTL
	}

	if err := in.Presets.Validate(); err != nil {
		return nil, "", err
	}

	secret, err := pkgstrings.CryptoRandomString(ticketSecretLength)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to generate ticket secret")
	}

	id := xid.New().String()
	setupKey := id + secret

	now := s.now()
	ticket := &Ticket{
		ID:           id,
		SetupKeyHash: pkgstrings.SHA256(setupKey),
		Owner:        in.Owner,
		Presets:      in.Presets,
		Status:       TicketStatusPending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
	}

	if err := s.store(ctx, ticket, ttl); err != nil {
		return nil, "", err
	}

	return ticket, setupKey, nil
}

// Get returns a ticket by its non-secret id.
func (s *TicketStore) Get(ctx context.Context, id string) (*Ticket, error) {
	if !isTicketID(id) {
		return nil, ErrTicketNotFound
	}

	raw, err := s.cache.Get(ctx, setupTicketCachePrefix+id)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return nil, ErrTicketNotFound
		}

		return nil, errors.WithMessage(err, "failed to read enrollment ticket")
	}

	encoded, ok := raw.(string)
	if !ok {
		return nil, ErrTicketNotFound
	}

	var ticket Ticket
	if err := json.Unmarshal([]byte(encoded), &ticket); err != nil {
		return nil, errors.Wrap(err, "failed to decode enrollment ticket")
	}

	return &ticket, nil
}

// Resolve authenticates a setup key against its ticket. The id half of the key
// locates the record; the digest of the whole key authenticates it.
func (s *TicketStore) Resolve(ctx context.Context, setupKey string) (*Ticket, error) {
	if !IsTicketKey(setupKey) {
		return nil, ErrTicketNotFound
	}

	ticket, err := s.Get(ctx, setupKey[:ticketIDLength])
	if err != nil {
		return nil, err
	}

	given := pkgstrings.SHA256(setupKey)
	if subtle.ConstantTimeCompare([]byte(given), []byte(ticket.SetupKeyHash)) != 1 {
		return nil, ErrInvalidSetupKey
	}

	if ticket.Status == TicketStatusConsumed {
		return nil, ErrTicketConsumed
	}

	if s.now().After(ticket.ExpiresAt) {
		return nil, ErrTicketExpired
	}

	return ticket, nil
}

// Consume marks the ticket used and records the node the daemon became. The
// record survives for a while so the issuer can still read the result.
func (s *TicketStore) Consume(ctx context.Context, ticket *Ticket, nodeID uint) error {
	now := s.now()

	ticket.Status = TicketStatusConsumed
	ticket.NodeID = nodeID
	ticket.ConsumedAt = &now

	return s.store(ctx, ticket, consumedTicketRetention)
}

// Revoke drops a ticket; revoking an unknown ticket is not an error.
func (s *TicketStore) Revoke(ctx context.Context, id string) error {
	if !isTicketID(id) {
		return ErrTicketNotFound
	}

	if err := s.cache.Delete(ctx, setupTicketCachePrefix+id); err != nil {
		return errors.WithMessage(err, "failed to revoke enrollment ticket")
	}

	return nil
}

// store writes the ticket as a JSON string: the in-memory cache hands values
// back verbatim while the Redis and SQL caches round-trip them through
// json.Unmarshal into any, and a string survives both unchanged.
func (s *TicketStore) store(ctx context.Context, ticket *Ticket, ttl time.Duration) error {
	encoded, err := json.Marshal(ticket)
	if err != nil {
		return errors.Wrap(err, "failed to encode enrollment ticket")
	}

	err = s.cache.Set(ctx, setupTicketCachePrefix+ticket.ID, string(encoded), cache.WithExpiration(ttl))
	if err != nil {
		return errors.WithMessage(err, "failed to store enrollment ticket")
	}

	return nil
}

// IsTicketKey reports whether a setup key has the shape of a ticket key. It is
// checked before any cache lookup so a malformed key from a daemon never
// reaches the cache backend.
func IsTicketKey(key string) bool {
	if len(key) != ticketIDLength+ticketSecretLength {
		return false
	}

	if !isAlphanumeric(key[ticketIDLength:]) {
		return false
	}

	return isTicketID(key[:ticketIDLength])
}

func isTicketID(id string) bool {
	if len(id) != ticketIDLength {
		return false
	}

	if _, err := xid.FromString(id); err != nil {
		return false
	}

	return true
}

func isAlphanumeric(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
	}) == -1
}
