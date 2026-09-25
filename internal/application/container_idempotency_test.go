package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/idempotency"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableRedisAddr is a port nothing listens on: a container that dials it
// panics, one that must not dial it stays quiet.
const unreachableRedisAddr = "127.0.0.1:1"

// sendKeyedTwice runs the same keyed request through the middleware twice and
// returns how often the handler ran and whether the second answer was replayed.
func sendKeyedTwice(t *testing.T, m *idempotency.Middleware) (int32, bool) {
	t.Helper()

	var calls atomic.Int32

	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))

	var last *httptest.ResponseRecorder

	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/api/servers", strings.NewReader(`{}`))
		r.Header.Set(idempotency.HeaderKey, "order-1")
		r = r.WithContext(auth.ContextWithSession(r.Context(), &auth.Session{User: &domain.User{ID: 1}}))

		last = httptest.NewRecorder()
		h.ServeHTTP(last, r)
	}

	return calls.Load(), last.Header().Get(idempotency.HeaderReplayed) == "true"
}

func TestIdempotencyMiddleware_database_driver_replays(t *testing.T) {
	t.Parallel()

	c := newWiredContainer(t, func(cfg *config.Config) {
		cfg.Idempotency.Driver = idempotencyDriverDatabase
	})

	calls, replayed := sendKeyedTwice(t, c.IdempotencyMiddleware())

	assert.Equal(t, int32(1), calls)
	assert.True(t, replayed)
	assert.NotNil(t, c.IdempotencyJanitor())
}

func TestIdempotencyMiddleware_database_driver_coordinates_without_redis_cache(t *testing.T) {
	t.Parallel()

	first := newWiredContainer(t, func(cfg *config.Config) {
		cfg.Idempotency.Driver = idempotencyDriverDatabase
		cfg.Cache.Driver = cacheDriverRedis
		cfg.Cache.Redis.Addr = unreachableRedisAddr
	})
	second := newMinimalContainer(first.config)
	second.db = first.db

	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan *httptest.ResponseRecorder, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusCreated)
	})
	firstHandler := first.IdempotencyMiddleware().Middleware(handler)
	secondHandler := second.IdempotencyMiddleware().Middleware(handler)
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/servers", strings.NewReader(`{}`))
		r.Header.Set(idempotency.HeaderKey, "shared-database-lock")

		return r.WithContext(auth.ContextWithSession(r.Context(), &auth.Session{User: &domain.User{ID: 1}}))
	}

	t.Cleanup(func() {
		close(release)
		assert.Equal(t, http.StatusCreated, (<-completed).Code)
	})
	go func() {
		response := httptest.NewRecorder()
		firstHandler.ServeHTTP(response, request())
		completed <- response
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not reach the handler")
	}

	concurrent := httptest.NewRecorder()
	secondHandler.ServeHTTP(concurrent, request())

	assert.Equal(t, http.StatusConflict, concurrent.Code)
	assert.Equal(t, int32(1), calls.Load())
}

func TestIdempotencyMiddleware_none_driver_ignores_the_header(t *testing.T) {
	t.Parallel()

	c := newMinimalContainer(&config.Config{})
	c.config.Idempotency.Driver = idempotencyDriverNone
	c.config.Idempotency.Redis.Addr = unreachableRedisAddr

	calls, replayed := sendKeyedTwice(t, c.IdempotencyMiddleware())

	assert.Equal(t, int32(2), calls)
	assert.False(t, replayed)
	assert.Nil(t, c.IdempotencyJanitor())
}

func TestIdempotencyMiddleware_rejects_invalid_configuration(t *testing.T) {
	t.Parallel()

	const sharedDB = 3

	tests := []struct {
		name      string
		driver    string
		configure func(cfg *config.Config)
		wantPanic string
	}{
		{
			name:      "unknown_driver",
			driver:    "bogus",
			wantPanic: "invalid idempotency driver: bogus",
		},
		{
			name:      "unreachable_redis",
			driver:    idempotencyDriverRedis,
			wantPanic: "failed to connect to idempotency Redis",
		},
		{
			name:   "redis_database_shared_with_cache",
			driver: idempotencyDriverRedis,
			configure: func(cfg *config.Config) {
				cfg.Cache.Driver = cacheDriverRedis
				cfg.Cache.Redis.Addr = unreachableRedisAddr
				cfg.Cache.Redis.DB = sharedDB
				cfg.Idempotency.Redis.DB = sharedDB
			},
			wantPanic: "IDEMPOTENCY_REDIS_DB must differ from CACHE_REDIS_DB",
		},
		{
			name:   "same_database_number_on_another_redis",
			driver: idempotencyDriverRedis,
			configure: func(cfg *config.Config) {
				cfg.Cache.Driver = cacheDriverRedis
				cfg.Cache.Redis.Addr = "cache.internal:6379"
				cfg.Cache.Redis.DB = sharedDB
				cfg.Idempotency.Redis.DB = sharedDB
			},
			wantPanic: "failed to connect to idempotency Redis",
		},
		{
			name:   "cache_not_on_redis",
			driver: idempotencyDriverRedis,
			configure: func(cfg *config.Config) {
				cfg.Cache.Driver = cacheDriverMemory
				cfg.Cache.Redis.Addr = unreachableRedisAddr
				cfg.Cache.Redis.DB = sharedDB
				cfg.Idempotency.Redis.DB = sharedDB
			},
			wantPanic: "failed to connect to idempotency Redis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newMinimalContainer(&config.Config{})
			c.config.Idempotency.Driver = tt.driver
			c.config.Idempotency.Redis.Addr = unreachableRedisAddr

			if tt.configure != nil {
				tt.configure(c.config)
			}

			panicValue := func() (value any) {
				defer func() {
					value = recover()
				}()

				c.IdempotencyMiddleware()

				return nil
			}()

			require.NotNil(t, panicValue)
			assert.Contains(t, panicMessage(panicValue), tt.wantPanic)
		})
	}
}

func TestIdempotencyMiddleware_redis_driver_uses_its_own_database(t *testing.T) {
	t.Parallel()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("Skipping Redis idempotency tests because TEST_REDIS_ADDR is not set")
	}

	const idempotencyDB = 2

	c := newMinimalContainer(&config.Config{})
	c.config.Idempotency.Driver = idempotencyDriverRedis
	c.config.Idempotency.Redis.DB = idempotencyDB
	// Left empty on purpose: the address falls back to the cache's.
	c.config.Cache.Redis.Addr = addr
	c.config.Cache.Redis.Password = os.Getenv("TEST_REDIS_PASSWORD")

	t.Cleanup(func() {
		for _, fn := range c.lateShutdownFuncs {
			_ = fn()
		}
	})

	calls, replayed := sendKeyedTwice(t, c.IdempotencyMiddleware())

	assert.Equal(t, int32(1), calls)
	assert.True(t, replayed)
	assert.Nil(t, c.IdempotencyJanitor(), "Redis expires its keys itself")

	idempotencyClient := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
		DB:       idempotencyDB,
	})
	cacheClient := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
	})

	t.Cleanup(func() {
		_ = idempotencyClient.Close()
		_ = cacheClient.Close()
	})

	keys, err := idempotencyClient.Keys(context.Background(), "gameap:idempotency:1:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)

	t.Cleanup(func() {
		_ = idempotencyClient.Del(context.Background(), keys...).Err()
	})

	cacheKeys, err := cacheClient.Keys(context.Background(), "gameap:idempotency:*").Result()
	require.NoError(t, err)
	assert.Empty(t, cacheKeys, "the cache database must not hold idempotency keys")
}

func panicMessage(value any) string {
	switch v := value.(type) {
	case error:
		return v.Error()
	case string:
		return v
	default:
		return ""
	}
}
