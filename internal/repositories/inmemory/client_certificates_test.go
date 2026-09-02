package inmemory_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestClientCertificateRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewClientCertificatesRepositorySuite(
		func(_ *testing.T) repositories.ClientCertificateRepository {
			return inmemory.NewClientCertificateRepository()
		},
	))
}

func setupClientCertificateRepo(
	t *testing.T,
	certificates []domain.ClientCertificate,
) repositories.ClientCertificateRepository {
	t.Helper()

	repo := inmemory.NewClientCertificateRepository()

	for i := range certificates {
		require.NoError(t, repo.Save(t.Context(), &certificates[i]))
	}

	return repo
}

func clientCertificateIDs(certificates []domain.ClientCertificate) []uint {
	ids := make([]uint, 0, len(certificates))
	for _, cert := range certificates {
		ids = append(ids, cert.ID)
	}

	return ids
}

func TestClientCertificateRepository_SortByScalarField(t *testing.T) {
	t.Parallel()

	fixture := func() []domain.ClientCertificate {
		return []domain.ClientCertificate{
			{ID: 1, Fingerprint: "c", Expires: time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 2, Fingerprint: "a", Expires: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 3, Fingerprint: "d", Expires: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 4, Fingerprint: "b", Expires: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)},
		}
	}

	tests := []struct {
		name     string
		field    string
		wantAsc  []uint
		wantDesc []uint
	}{
		{
			name:     "id",
			field:    "id",
			wantAsc:  []uint{1, 2, 3, 4},
			wantDesc: []uint{4, 3, 2, 1},
		},
		{
			name:     "expires",
			field:    "expires",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupClientCertificateRepo(t, fixture())

			// ACT
			asc, ascErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionAsc}}, nil,
			)
			desc, descErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionDesc}}, nil,
			)

			// ASSERT
			require.NoError(t, ascErr)
			require.NoError(t, descErr)
			require.Len(t, asc, len(tt.wantAsc))
			require.Len(t, desc, len(tt.wantDesc))
			assert.Equal(t, tt.wantAsc, clientCertificateIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, clientCertificateIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestClientCertificateRepository_SortByEqualExpiresKeepsNextTermDeciding(t *testing.T) {
	t.Parallel()

	sharedExpiry := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	laterExpiry := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	// ARRANGE
	repo := setupClientCertificateRepo(t, []domain.ClientCertificate{
		{ID: 1, Fingerprint: "a", Expires: sharedExpiry},
		{ID: 2, Fingerprint: "b", Expires: laterExpiry},
		{ID: 3, Fingerprint: "c", Expires: sharedExpiry},
	})

	// ACT
	certificates, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "expires", Direction: filters.SortDirectionAsc},
		{Field: "id", Direction: filters.SortDirectionDesc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, certificates, 3)
	assert.Equal(t, []uint{3, 1, 2}, clientCertificateIDs(certificates),
		"certificates with the same expiry must be ordered by the next sort term")
}

func TestClientCertificateRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupClientCertificateRepo(t, []domain.ClientCertificate{
		{ID: 1, Fingerprint: "a", Expires: time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Fingerprint: "b", Expires: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 3, Fingerprint: "c", Expires: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)},
	})

	// ACT
	certificates, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "fingerprint", Direction: filters.SortDirectionAsc},
		{Field: "expires", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, certificates, 3)
	assert.Equal(t, []uint{2, 3, 1}, clientCertificateIDs(certificates),
		"fingerprint is not a sortable column, so expires must decide the order")
}
