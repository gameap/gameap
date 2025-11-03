package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskFailRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerTaskFailRepositorySuite(
		func(_ *testing.T) repositories.ServerTaskFailRepository {
			return inmemory.NewServerTaskFailRepository()
		},
	))
}
