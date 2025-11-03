package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestNodeRepository(t *testing.T) {
	suite.Run(t, repotesting.NewNodeRepositorySuite(
		func(t *testing.T) repositories.NodeRepository {
			t.Helper()

			return sqlite.NewNodeRepository(SetupTestDB(t))
		},
	))
}
