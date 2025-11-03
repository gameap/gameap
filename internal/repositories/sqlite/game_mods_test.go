package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestGameModRepository(t *testing.T) {
	suite.Run(t, repotesting.NewGameModRepositorySuite(
		func(t *testing.T) repositories.GameModRepository {
			t.Helper()

			return sqlite.NewGameModRepository(SetupTestDB(t))
		},
	))
}
