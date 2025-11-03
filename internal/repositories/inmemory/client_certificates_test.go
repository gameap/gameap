package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestClientCertificateRepository(t *testing.T) {
	suite.Run(t, repotesting.NewClientCertificatesRepositorySuite(
		func(_ *testing.T) repositories.ClientCertificateRepository {
			return inmemory.NewClientCertificateRepository()
		},
	))
}
