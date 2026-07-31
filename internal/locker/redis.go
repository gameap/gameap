package locker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type RedisLocker struct {
	client *redis.Client
	prefix string
}

func NewRedisLocker(client *redis.Client, prefix string) *RedisLocker {
	return &RedisLocker{client: client, prefix: prefix}
}

func (l *RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	if ttl <= 0 {
		return nil, errors.New("ttl must be positive")
	}

	token, err := newToken()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate lock token")
	}

	res, err := l.client.SetArgs(ctx, l.prefix+key, token, redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrLocked
		}

		return nil, errors.Wrap(err, "redis set failed")
	}

	if res != "OK" {
		return nil, ErrLocked
	}

	return &redisLock{
		client: l.client,
		key:    l.prefix + key,
		token:  token,
	}, nil
}

type redisLock struct {
	mu       sync.Mutex
	client   *redis.Client
	key      string
	token    string
	released bool
}

const releaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`

func (l *redisLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}

	if err := l.client.Eval(ctx, releaseScript, []string{l.key}, l.token).Err(); err != nil {
		return errors.Wrap(err, "redis release failed")
	}

	l.released = true

	return nil
}

const refreshScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
	return 0
end
`

func (l *redisLock) Refresh(ctx context.Context, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return errors.New("lock already released")
	}

	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}

	res, err := l.client.Eval(ctx, refreshScript, []string{l.key}, l.token, ttl.Milliseconds()).Result()
	if err != nil {
		return errors.Wrap(err, "redis refresh failed")
	}

	if v, _ := res.(int64); v == 0 {
		return ErrLockLost
	}

	return nil
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
