package inmemory

import (
	"cmp"
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type ClientCertificateRepository struct {
	mu           sync.RWMutex
	certificates map[uint]*domain.ClientCertificate
	nextID       uint32
	idIndex      map[uint]map[uint]struct{} // id -> certificateIDs
}

func NewClientCertificateRepository() *ClientCertificateRepository {
	return &ClientCertificateRepository{
		certificates: make(map[uint]*domain.ClientCertificate),
		idIndex:      make(map[uint]map[uint]struct{}),
	}
}

func (r *ClientCertificateRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ClientCertificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	certificates := make([]domain.ClientCertificate, 0, len(r.certificates))
	for _, cert := range r.certificates {
		certificates = append(certificates, *cert)
	}

	r.sortCertificates(certificates, order)

	return r.applyPagination(certificates, pagination), nil
}

func (r *ClientCertificateRepository) Find(
	_ context.Context,
	filter *filters.FindClientCertificate,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ClientCertificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindClientCertificate{}
	}

	// Use hash indexes for efficient filtering
	candidateIDs := r.getFilteredCertificateIDs(filter)

	certificates := make([]domain.ClientCertificate, 0, len(candidateIDs))
	for certID := range candidateIDs {
		if cert, exists := r.certificates[certID]; exists {
			certificates = append(certificates, *cert)
		}
	}

	r.sortCertificates(certificates, order)

	return r.applyPagination(certificates, pagination), nil
}

func (r *ClientCertificateRepository) Save(_ context.Context, cert *domain.ClientCertificate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old indexes if updating existing certificate
	if cert.ID != 0 {
		if oldCert, exists := r.certificates[cert.ID]; exists {
			r.removeFromIndexes(oldCert)
		}
	} else {
		cert.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	// Save certificate (deep copy to prevent external modifications)
	r.certificates[cert.ID] = &domain.ClientCertificate{
		ID:          cert.ID,
		Fingerprint: cert.Fingerprint,
		Expires:     cert.Expires,
		Certificate: cert.Certificate,
		PrivateKey:  cert.PrivateKey,
	}

	// Add to indexes
	r.addToIndexes(r.certificates[cert.ID])

	return nil
}

func (r *ClientCertificateRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cert, exists := r.certificates[id]; exists {
		// Remove from indexes
		r.removeFromIndexes(cert)
	}

	delete(r.certificates, id)

	return nil
}

func (r *ClientCertificateRepository) addToIndexes(cert *domain.ClientCertificate) {
	// ID index
	if r.idIndex[cert.ID] == nil {
		r.idIndex[cert.ID] = make(map[uint]struct{})
	}
	r.idIndex[cert.ID][cert.ID] = struct{}{}
}

func (r *ClientCertificateRepository) removeFromIndexes(cert *domain.ClientCertificate) {
	// ID index
	if certSet, exists := r.idIndex[cert.ID]; exists {
		delete(certSet, cert.ID)
		if len(certSet) == 0 {
			delete(r.idIndex, cert.ID)
		}
	}
}

func (r *ClientCertificateRepository) getFilteredCertificateIDs(
	filter *filters.FindClientCertificate,
) map[uint]struct{} {
	resultIDs := make(map[uint]struct{}, len(r.certificates))

	if filter == nil {
		// No filter, return all certificate IDs
		for certID := range r.certificates {
			resultIDs[certID] = struct{}{}
		}

		return resultIDs
	}

	// Filter by IDs
	if len(filter.IDs) > 0 {
		for _, id := range filter.IDs {
			if _, exists := r.certificates[id]; exists {
				resultIDs[id] = struct{}{}
			}
		}
	} else {
		// No ID filter, return all certificates
		for certID := range r.certificates {
			resultIDs[certID] = struct{}{}
		}
	}

	return resultIDs
}

func (r *ClientCertificateRepository) sortCertificates(
	certificates []domain.ClientCertificate, order []filters.Sorting,
) {
	if len(order) == 0 {
		sort.Slice(certificates, func(i, j int) bool {
			return certificates[i].ID < certificates[j].ID
		})

		return
	}

	sort.Slice(certificates, func(i, j int) bool {
		for _, o := range order {
			cmpRes := r.compareCertificates(&certificates[i], &certificates[j], o.Field)
			if cmpRes != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cmpRes > 0
				}

				return cmpRes < 0
			}
		}

		return false
	})
}

func (r *ClientCertificateRepository) compareCertificates(a, b *domain.ClientCertificate, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "expires":
		if a.Expires.Before(b.Expires) {
			return -1
		}
		if a.Expires.After(b.Expires) {
			return 1
		}

		return 0
	default:
		return 0
	}
}

func (r *ClientCertificateRepository) applyPagination(
	certificates []domain.ClientCertificate,
	pagination *filters.Pagination,
) []domain.ClientCertificate {
	if pagination == nil {
		return certificates
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(certificates) {
		return []domain.ClientCertificate{}
	}

	end := min(offset+limit, len(certificates))

	return certificates[offset:end]
}
