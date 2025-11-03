package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestDaemonTaskRepository(t *testing.T) {
	suite.Run(t, repotesting.NewDaemonTaskRepositorySuite(
		func(_ *testing.T) repositories.DaemonTaskRepository {
			return inmemory.NewDaemonTaskRepository()
		},
	))
}
