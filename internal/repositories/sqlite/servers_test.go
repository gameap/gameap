package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/suite"
)

func TestServerRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerRepositorySuite(
		func(t *testing.T) repositories.ServerRepository {
			t.Helper()

			db := SetupTestDB(t)
			tm := services.NewNilTransactionManager()

			return sqlite.NewServerRepository(db, tm)
		},
	))
}
