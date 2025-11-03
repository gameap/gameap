package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPersonalAccessTokenRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPersonalAccessTokenRepositorySuite(
		func(_ *testing.T) repositories.PersonalAccessTokenRepository {
			return inmemory.NewPersonalAccessTokenRepository()
		},
	))
}
