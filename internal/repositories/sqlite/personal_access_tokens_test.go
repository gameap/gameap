package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPersonalAccessTokenRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPersonalAccessTokenRepositorySuite(
		func(t *testing.T) repositories.PersonalAccessTokenRepository {
			t.Helper()

			return sqlite.NewPersonalAccessTokenRepository(SetupTestDB(t))
		},
	))
}
