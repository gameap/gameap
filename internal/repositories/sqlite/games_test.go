package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestGameRepository(t *testing.T) {
	suite.Run(t, repotesting.NewGameRepositorySuite(
		func(t *testing.T) repositories.GameRepository {
			t.Helper()

			return sqlite.NewGameRepository(SetupTestDB(t))
		},
	))
}
