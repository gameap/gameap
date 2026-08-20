package inmemory

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestUserRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(_ *testing.T) repositories.UserRepository {
			return NewUserRepository()
		},
	))
}
