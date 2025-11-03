package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ClientCertificatesRepositorySuite struct {
	suite.Suite

	repo repositories.ClientCertificateRepository

	fn func(t *testing.T) repositories.ClientCertificateRepository
}

// Define a function type for repository setup.
type clientCertificateRepoSetupFunc func(t *testing.T) repositories.ClientCertificateRepository

func NewClientCertificatesRepositorySuite(fn clientCertificateRepoSetupFunc) *ClientCertificatesRepositorySuite {
	return &ClientCertificatesRepositorySuite{
		fn: fn,
	}
}

func (s *ClientCertificatesRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *ClientCertificatesRepositorySuite) TestClientCertificateRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_certificate", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			Expires:     time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			Certificate: "-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAKHHCgVZU7t2MA0GCSqGSIb3DQEBCwUAMBEx\n-----END CERTIFICATE-----",
			PrivateKey:  "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0\n-----END PRIVATE KEY-----",
		}

		err := s.repo.Save(ctx, cert)
		require.NoError(t, err)
		assert.NotZero(t, cert.ID)
	})

	s.T().Run("update_existing_certificate", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			Fingerprint: "11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00",
			Expires:     time.Date(2025, 6, 30, 23, 59, 59, 0, time.UTC),
			Certificate: "-----BEGIN CERTIFICATE-----\nOriginalCert\n-----END CERTIFICATE-----",
			PrivateKey:  "-----BEGIN PRIVATE KEY-----\nOriginalKey\n-----END PRIVATE KEY-----",
		}

		err := s.repo.Save(ctx, cert)
		require.NoError(t, err)
		originalID := cert.ID

		cert.Fingerprint = "FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00"
		cert.Expires = time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
		cert.Certificate = "-----BEGIN CERTIFICATE-----\nUpdatedCert\n-----END CERTIFICATE-----"
		cert.PrivateKey = "-----BEGIN PRIVATE KEY-----\nUpdatedKey\n-----END PRIVATE KEY-----"

		err = s.repo.Save(ctx, cert)
		require.NoError(t, err)
		assert.Equal(t, originalID, cert.ID)
		assert.Equal(t, "FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00", cert.Fingerprint)
		assert.Equal(t, time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), cert.Expires)
		assert.Contains(t, cert.Certificate, "UpdatedCert")
		assert.Contains(t, cert.PrivateKey, "UpdatedKey")
	})

	s.T().Run("save_with_explicit_id", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			ID:          999,
			Fingerprint: "99:88:77:66:55:44:33:22:11:00:FF:EE:DD:CC:BB:AA",
			Expires:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			Certificate: "-----BEGIN CERTIFICATE-----\nExplicitIDCert\n-----END CERTIFICATE-----",
			PrivateKey:  "-----BEGIN PRIVATE KEY-----\nExplicitIDKey\n-----END PRIVATE KEY-----",
		}

		err := s.repo.Save(ctx, cert)
		require.NoError(t, err)
		assert.Equal(t, uint(999), cert.ID)
	})
}

func (s *ClientCertificatesRepositorySuite) TestClientCertificateRepositoryFindAll() {
	ctx := context.Background()

	s.T().Run("find_all_certificates", func(t *testing.T) {
		certs := []*domain.ClientCertificate{
			{
				Fingerprint: "AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA",
				Expires:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Certificate: "cert1",
				PrivateKey:  "key1",
			},
			{
				Fingerprint: "BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB",
				Expires:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Certificate: "cert2",
				PrivateKey:  "key2",
			},
			{
				Fingerprint: "CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC",
				Expires:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
				Certificate: "cert3",
				PrivateKey:  "key3",
			},
		}

		for _, cert := range certs {
			require.NoError(t, s.repo.Save(ctx, cert))
		}

		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		foundFingerprints := make(map[string]bool)
		for _, result := range results {
			foundFingerprints[result.Fingerprint] = true
		}

		assert.True(t, foundFingerprints["AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA:AA"])
		assert.True(t, foundFingerprints["BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB:BB"])
		assert.True(t, foundFingerprints["CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC:CC"])
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *ClientCertificatesRepositorySuite) TestClientCertificateRepositoryFind() {
	ctx := context.Background()

	cert1 := &domain.ClientCertificate{
		Fingerprint: "DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD:DD",
		Expires:     time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC),
		Certificate: "find_cert1",
		PrivateKey:  "find_key1",
	}
	cert2 := &domain.ClientCertificate{
		Fingerprint: "EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE:EE",
		Expires:     time.Date(2025, 6, 20, 18, 30, 0, 0, time.UTC),
		Certificate: "find_cert2",
		PrivateKey:  "find_key2",
	}
	cert3 := &domain.ClientCertificate{
		Fingerprint: "FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF",
		Expires:     time.Date(2025, 9, 25, 9, 45, 0, 0, time.UTC),
		Certificate: "find_cert3",
		PrivateKey:  "find_key3",
	}

	require.NoError(s.T(), s.repo.Save(ctx, cert1))
	require.NoError(s.T(), s.repo.Save(ctx, cert2))
	require.NoError(s.T(), s.repo.Save(ctx, cert3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindClientCertificate{
			IDs: []uint{cert1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, cert1.ID, results[0].ID)
		assert.Equal(t, cert1.Fingerprint, results[0].Fingerprint)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindClientCertificate{
			IDs: []uint{cert1.ID, cert3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, cert1.ID)
		assert.Contains(t, ids, cert3.ID)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_with_empty_filter", func(t *testing.T) {
		filter := &filters.FindClientCertificate{}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent_id", func(t *testing.T) {
		filter := &filters.FindClientCertificate{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindClientCertificate{
			IDs: []uint{cert1.ID, cert2.ID, cert3.ID},
		}
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		filter := &filters.FindClientCertificate{
			IDs: []uint{cert1.ID, cert2.ID, cert3.ID},
		}
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, filter, order, nil)
		require.NoError(t, err)
		require.Len(t, results, 3)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *ClientCertificatesRepositorySuite) TestClientCertificateRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_certificate", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			Fingerprint: "00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF",
			Expires:     time.Date(2025, 5, 10, 14, 20, 30, 0, time.UTC),
			Certificate: "delete_cert",
			PrivateKey:  "delete_key",
		}

		require.NoError(t, s.repo.Save(ctx, cert))
		certID := cert.ID

		err := s.repo.Delete(ctx, certID)
		require.NoError(t, err)

		filter := &filters.FindClientCertificate{
			IDs: []uint{certID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_certificate", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_certificate", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			Fingerprint: "AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89",
			Expires:     time.Date(2025, 8, 15, 10, 30, 0, 0, time.UTC),
			Certificate: "double_delete_cert",
			PrivateKey:  "double_delete_key",
		}

		require.NoError(t, s.repo.Save(ctx, cert))
		certID := cert.ID

		err := s.repo.Delete(ctx, certID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, certID)
		require.NoError(t, err)
	})
}

func (s *ClientCertificatesRepositorySuite) TestClientCertificateRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		cert := &domain.ClientCertificate{
			Fingerprint: "12:34:56:78:9A:BC:DE:F0:12:34:56:78:9A:BC:DE:F0",
			Expires:     time.Date(2025, 11, 11, 11, 11, 11, 0, time.UTC),
			Certificate: "lifecycle_cert",
			PrivateKey:  "lifecycle_key",
		}

		err := s.repo.Save(ctx, cert)
		require.NoError(t, err)
		assert.NotZero(t, cert.ID)

		filter := &filters.FindClientCertificate{
			IDs: []uint{cert.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, cert.Fingerprint, results[0].Fingerprint)

		cert.Fingerprint = "FE:DC:BA:98:76:54:32:10:FE:DC:BA:98:76:54:32:10"
		err = s.repo.Save(ctx, cert)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "FE:DC:BA:98:76:54:32:10:FE:DC:BA:98:76:54:32:10", results[0].Fingerprint)

		err = s.repo.Delete(ctx, cert.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_certificates_operations", func(t *testing.T) {
		var certIDs []uint
		for i := range 5 {
			cert := &domain.ClientCertificate{
				Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:" + string(rune('A'+i)) + string(rune('0'+i)),
				Expires:     time.Date(2025+i, 1, 1, 0, 0, 0, 0, time.UTC),
				Certificate: "multi_cert_" + string(rune('A'+i)),
				PrivateKey:  "multi_key_" + string(rune('A'+i)),
			}
			require.NoError(t, s.repo.Save(ctx, cert))
			certIDs = append(certIDs, cert.ID)
		}

		filter := &filters.FindClientCertificate{
			IDs: certIDs,
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 5)

		for i := range 3 {
			require.NoError(t, s.repo.Delete(ctx, certIDs[i]))
		}

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}
