package idempotency

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

// RedisStore keeps outcomes as Redis keys that expire with them. The Redis
// instance must not evict keys before they expire (maxmemory-policy
// noeviction): an evicted outcome lets a retry run the request again. The
// policy is instance-wide, so the store needs an instance of its own rather
// than the cache's, which usually evicts.
type RedisStore struct {
	client *redis.Client
	prefix string
}

func NewRedisStore(client *redis.Client, prefix string) *RedisStore {
	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

// redisRecord pins the stored JSON shape independently of the domain type.
type redisRecord struct {
	RequestMethod      string              `json:"request_method"`
	RequestPath        string              `json:"request_path"`
	RequestFingerprint string              `json:"request_fingerprint"`
	ResponseStatus     int                 `json:"response_status"`
	ResponseHeaders    map[string][]string `json:"response_headers"`
	ResponseBody       []byte              `json:"response_body"`
	CreatedAt          time.Time           `json:"created_at"`
	ExpiresAt          time.Time           `json:"expires_at"`
}

func (s *RedisStore) Find(
	ctx context.Context,
	userID uint,
	keyHash string,
	now time.Time,
) (*domain.IdempotencyKey, error) {
	raw, err := s.client.Get(ctx, s.key(userID, keyHash)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, errors.Wrap(err, "redis get failed")
	}

	var stored redisRecord
	if err = json.Unmarshal(raw, &stored); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal idempotency record")
	}

	record := &domain.IdempotencyKey{
		UserID:             userID,
		KeyHash:            keyHash,
		RequestMethod:      stored.RequestMethod,
		RequestPath:        stored.RequestPath,
		RequestFingerprint: stored.RequestFingerprint,
		ResponseStatus:     stored.ResponseStatus,
		ResponseHeaders:    stored.ResponseHeaders,
		ResponseBody:       stored.ResponseBody,
		CreatedAt:          stored.CreatedAt,
		ExpiresAt:          stored.ExpiresAt,
	}

	// Redis expires the key itself; the check covers the milliseconds it
	// may lag behind.
	if record.IsExpired(now) {
		return nil, nil
	}

	return record, nil
}

// PEXPIREAT preserves millisecond precision and works before Redis 6.2's
// SET PXAT. Both commands must be atomic so a crash cannot leave an immortal key.
const saveRedisRecordScript = `
if not redis.call("SET", KEYS[1], ARGV[1], "NX") then
	return 0
end
redis.call("PEXPIREAT", KEYS[1], ARGV[2])
return 1
`

func (s *RedisStore) Save(ctx context.Context, record *domain.IdempotencyKey) (bool, error) {
	if !record.ExpiresAt.After(record.CreatedAt) {
		return false, errors.New("idempotency record expires before it is created")
	}

	encoded, err := json.Marshal(redisRecord{
		RequestMethod:      record.RequestMethod,
		RequestPath:        record.RequestPath,
		RequestFingerprint: record.RequestFingerprint,
		ResponseStatus:     record.ResponseStatus,
		ResponseHeaders:    record.ResponseHeaders,
		ResponseBody:       record.ResponseBody,
		CreatedAt:          record.CreatedAt,
		ExpiresAt:          record.ExpiresAt,
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to marshal idempotency record")
	}

	res, err := s.client.Eval(ctx, saveRedisRecordScript,
		[]string{s.key(record.UserID, record.KeyHash)}, encoded, record.ExpiresAt.UnixMilli(),
	).Int()
	if err != nil {
		return false, errors.Wrap(err, "redis set failed")
	}

	return res == 1, nil
}

func (s *RedisStore) key(userID uint, keyHash string) string {
	return s.prefix + strconv.FormatUint(uint64(userID), 10) + ":" + keyHash
}

// WarnIfEvictable logs a warning when Redis may evict stored outcomes before
// they expire. It is best effort: managed Redis often disables CONFIG, and
// then nothing is logged.
func WarnIfEvictable(ctx context.Context, client *redis.Client, logger *slog.Logger) {
	policy, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return
	}

	limit, err := client.ConfigGet(ctx, "maxmemory").Result()
	if err != nil {
		return
	}

	if limit["maxmemory"] == "0" || policy["maxmemory-policy"] == "" || policy["maxmemory-policy"] == "noeviction" {
		return
	}

	logger.WarnContext(ctx,
		"idempotency: Redis may evict stored outcomes before they expire, and a retried request would run twice;"+
			" maxmemory-policy covers the whole instance, so point IDEMPOTENCY_REDIS_ADDR at a Redis of its own"+
			" with maxmemory-policy noeviction, or use IDEMPOTENCY_DRIVER=database",
		slog.String("maxmemory_policy", policy["maxmemory-policy"]),
		slog.String("maxmemory", limit["maxmemory"]),
	)
}
