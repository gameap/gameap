package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPluginRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewPluginRepositorySuite(
		func(_ *testing.T) repositories.PluginRepository {
			return inmemory.NewPluginRepository()
		},
	))
}
