package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPluginScheduledTaskRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPluginScheduledTaskRepositorySuite(
		func(_ *testing.T) repositories.PluginScheduledTaskRepository {
			return inmemory.NewPluginScheduledTaskRepository()
		},
	))
}
