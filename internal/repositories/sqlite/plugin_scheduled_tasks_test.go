package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"

	"github.com/gameap/gameap/internal/repositories/sqlite"
)

func TestPluginScheduledTaskRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPluginScheduledTaskRepositorySuite(
		func(t *testing.T) repositories.PluginScheduledTaskRepository {
			t.Helper()

			return sqlite.NewPluginScheduledTaskRepository(SetupTestDB(t))
		},
	))
}
