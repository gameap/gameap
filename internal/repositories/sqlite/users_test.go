package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestUserRepository(t *testing.T) {
	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(t *testing.T) repositories.UserRepository {
			t.Helper()

			return sqlite.NewUserRepository(SetupTestDB(t))
		},
	))
}
