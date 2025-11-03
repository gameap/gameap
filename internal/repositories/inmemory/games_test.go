package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestGameRepository(t *testing.T) {
	suite.Run(t, repotesting.NewGameRepositorySuite(
		func(_ *testing.T) repositories.GameRepository {
			return inmemory.NewGameRepository()
		},
	))
}
