package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestClientCertificateRepository(t *testing.T) {
	suite.Run(t, repotesting.NewClientCertificatesRepositorySuite(
		func(t *testing.T) repositories.ClientCertificateRepository {
			t.Helper()

			return sqlite.NewClientCertificateRepository(SetupTestDB(t))
		},
	))
}
