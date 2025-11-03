package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerTaskRepositorySuite(
		func(t *testing.T) repositories.ServerTaskRepository {
			t.Helper()

			return sqlite.NewServerTaskRepository(SetupTestDB(t))
		},
	))
}
