package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPluginSecretRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewPluginSecretRepositorySuite(
		func(_ *testing.T) repositories.PluginSecretRepository {
			return inmemory.NewPluginSecretRepository()
		},
	))
}
