package inmemory

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
)

func SetupTestRepo(t *testing.T) repositories.ClientCertificateRepository {
	t.Helper()

	repo := NewClientCertificateRepository()

	return repo
}
