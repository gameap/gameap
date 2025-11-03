package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerSettingRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerSettingRepositorySuite(
		func(t *testing.T) repositories.ServerSettingRepository {
			t.Helper()

			return sqlite.NewServerSettingRepository(SetupTestDB(t))
		},
	))
}
