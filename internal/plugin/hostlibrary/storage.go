package hostlibrary

import (
	"context"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/tetratelabs/wazero"
)

type StorageServiceImpl struct {
	pluginID uint64
	repo     repositories.PluginStorageRepository
}

func NewStorageService(pluginID uint64, repo repositories.PluginStorageRepository) *StorageServiceImpl {
	return &StorageServiceImpl{
		pluginID: pluginID,
		repo:     repo,
	}
}

func (s *StorageServiceImpl) Get(
	ctx context.Context,
	req *storage.StorageGetRequest,
) (*storage.StorageGetResponse, error) {
	filter := &filters.FindPluginStorage{
		PluginIDs: []uint64{s.pluginID},
		Keys:      []string{req.Key},
		EntityPairs: []domain.PluginStorageEntityPair{
			{
				EntityType: entityTypeFromProto(req.EntityType),
				EntityID:   uintPtrFromUint64Ptr(req.EntityId),
			},
		},
	}

	// Newest first: a scope that was duplicated before saves became
	// scope-aware reads back its latest write, not its first.
	newestFirst := []filters.Sorting{{Field: "id", Direction: filters.SortDirectionDesc}}

	entries, err := s.repo.Find(ctx, filter, newestFirst, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return &storage.StorageGetResponse{
			Found: false,
		}, nil
	}

	return &storage.StorageGetResponse{
		Payload: entries[0].Payload,
		Found:   true,
	}, nil
}

func (s *StorageServiceImpl) Set(
	ctx context.Context,
	req *storage.StorageSetRequest,
) (*storage.StorageSetResponse, error) {
	entry := &domain.PluginStorageEntry{
		PluginID:   s.pluginID,
		Key:        req.Key,
		EntityType: entityTypeFromProto(req.EntityType),
		EntityID:   uintPtrFromUint64Ptr(req.EntityId),
		Payload:    req.Payload,
	}

	err := s.repo.Save(ctx, entry)
	if err != nil {
		return &storage.StorageSetResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &storage.StorageSetResponse{
		Success: true,
	}, nil
}

func (s *StorageServiceImpl) Delete(
	ctx context.Context,
	req *storage.StorageDeleteRequest,
) (*storage.StorageDeleteResponse, error) {
	filter := &filters.FindPluginStorage{
		PluginIDs: []uint64{s.pluginID},
		Keys:      []string{req.Key},
		EntityPairs: []domain.PluginStorageEntityPair{
			{
				EntityType: entityTypeFromProto(req.EntityType),
				EntityID:   uintPtrFromUint64Ptr(req.EntityId),
			},
		},
	}

	err := s.repo.DeleteByFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &storage.StorageDeleteResponse{
		Success: true,
	}, nil
}

func (s *StorageServiceImpl) List(
	ctx context.Context,
	req *storage.StorageListRequest,
) (*storage.StorageListResponse, error) {
	filter := &filters.FindPluginStorage{
		PluginIDs: []uint64{s.pluginID},
	}

	if req.EntityType != nil || req.EntityId != nil {
		filter.EntityPairs = []domain.PluginStorageEntityPair{
			{
				EntityType: entityTypeFromProto(req.EntityType),
				EntityID:   uintPtrFromUint64Ptr(req.EntityId),
			},
		}
	}

	entries, err := s.repo.Find(ctx, filter, nil, nil)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.StorageEntry, 0, len(entries))
	for _, entry := range entries {
		if req.KeyPrefix != nil && !strings.HasPrefix(entry.Key, *req.KeyPrefix) {
			continue
		}

		result = append(result, &storage.StorageEntry{
			Key:        entry.Key,
			EntityType: entityTypeToProtoPtr(entry.EntityType),
			EntityId:   uint64PtrFromUintPtr(entry.EntityID),
			Payload:    entry.Payload,
		})
	}

	return &storage.StorageListResponse{
		Entries: result,
	}, nil
}

type StorageHostLibrary struct {
	impl *StorageServiceImpl
}

func NewStorageHostLibrary(pluginID uint64, repo repositories.PluginStorageRepository) *StorageHostLibrary {
	return &StorageHostLibrary{
		impl: NewStorageService(pluginID, repo),
	}
}

func (l *StorageHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return storage.Instantiate(ctx, r, l.impl)
}

type StorageHostLibraryFactory struct {
	repo repositories.PluginStorageRepository
}

func NewStorageHostLibraryFactory(repo repositories.PluginStorageRepository) *StorageHostLibraryFactory {
	return &StorageHostLibraryFactory{repo: repo}
}

func (f *StorageHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewStorageHostLibrary(pluginID, f.repo)
}
