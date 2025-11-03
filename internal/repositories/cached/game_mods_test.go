package cached_test

import (
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestGameModRepository(t *testing.T) {
	suite.Run(t, repotesting.NewGameModRepositorySuite(
		func(_ *testing.T) repositories.GameModRepository {
			return cached.NewGameModRepository(
				inmemory.NewGameModRepository(),
				cache.NewInMemory(),
				5*time.Minute,
			)
		},
	))
}

func TestGameModRepositoryWithRedisCache(t *testing.T) {
	testRedisAddr := os.Getenv("TEST_REDIS_ADDR")

	if testRedisAddr == "" {
		t.Skip("Skipping Redis tests because TEST_REDIS_ADDR is not set")
	}

	redisCache, err := cache.NewRedis(testRedisAddr, "", 0)
	if err != nil {
		t.Fatalf("failed to connect to Redis at %s: %v", testRedisAddr, err)
	}

	suite.Run(t, repotesting.NewGameModRepositorySuite(
		func(_ *testing.T) repositories.GameModRepository {
			return cached.NewGameModRepository(
				inmemory.NewGameModRepository(),
				redisCache,
				5*time.Minute,
			)
		},
	))
}
