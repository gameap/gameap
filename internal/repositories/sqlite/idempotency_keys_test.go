package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestIdempotencyKeyRepository(t *testing.T) {
	suite.Run(t, repotesting.NewIdempotencyKeyRepositorySuite(
		func(t *testing.T) repositories.IdempotencyKeyRepository {
			t.Helper()

			return sqlite.NewIdempotencyKeyRepository(SetupTestDB(t))
		},
	))
}
