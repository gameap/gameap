package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"

	"github.com/gameap/gameap/internal/repositories/sqlite"
)

func TestPluginSecretRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPluginSecretRepositorySuite(
		func(t *testing.T) repositories.PluginSecretRepository {
			t.Helper()

			return sqlite.NewPluginSecretRepository(SetupTestDB(t))
		},
	))
}
