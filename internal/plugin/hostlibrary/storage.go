package hostlibrary

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/tetratelabs/wazero"
)

const (
	defaultStorageMaxKeysPerPlugin = 10000
	defaultStorageMaxValueBytes    = 1 << 20
	defaultStorageMaxTotalBytes    = 64 << 20
)

// Storage failures are reported to the guest as fixed messages, as in
// gameap-secrets: a raw driver error would hand the plugin details about the
// panel's database (and the generated host glue panics on a returned error,
// putting that text in the guest's trace). The cause is logged instead.
const (
	storageReadFailureMessage  = "failed to read storage entry"
	storageWriteFailureMessage = "failed to store entry"
)

// StorageConfig caps what one plugin may keep in gameap-storage. Non-positive
// values fall back to the defaults above so a partially filled config cannot
// silently remove a quota. The checks are advisory between concurrent writes
// of one plugin (count-then-save), which is enough to keep a runaway plugin
// from filling the panel database.
type StorageConfig struct {
	MaxKeysPerPlugin int
	MaxValueBytes    int
	MaxTotalBytes    uint64
}

func (c StorageConfig) withDefaults() StorageConfig {
	if c.MaxKeysPerPlugin <= 0 {
		c.MaxKeysPerPlugin = defaultStorageMaxKeysPerPlugin
	}

	if c.MaxValueBytes <= 0 {
		c.MaxValueBytes = defaultStorageMaxValueBytes
	}

	if c.MaxTotalBytes == 0 {
		c.MaxTotalBytes = defaultStorageMaxTotalBytes
	}

	return c
}

// StorageOption tunes a StorageServiceImpl.
type StorageOption func(*StorageServiceImpl)

// WithStorageQuotas installs the per-plugin quotas.
func WithStorageQuotas(cfg StorageConfig) StorageOption {
	return func(s *StorageServiceImpl) {
		s.cfg = cfg.withDefaults()
	}
}

type StorageServiceImpl struct {
	pluginID uint64
	repo     repositories.PluginStorageRepository
	cfg      StorageConfig
}

func NewStorageService(
	pluginID uint64,
	repo repositories.PluginStorageRepository,
	opts ...StorageOption,
) *StorageServiceImpl {
	service := &StorageServiceImpl{
		pluginID: pluginID,
		repo:     repo,
		cfg:      StorageConfig{}.withDefaults(),
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

// scopeFilter selects the one entry identified by key and entity scope.
func (s *StorageServiceImpl) scopeFilter(
	key string,
	entityType *proto.EntityType,
	entityID *uint64,
) *filters.FindPluginStorage {
	return &filters.FindPluginStorage{
		PluginIDs: []uint64{s.pluginID},
		Keys:      []string{key},
		EntityPairs: []domain.PluginStorageEntityPair{
			{
				EntityType: entityTypeFromProto(entityType),
				EntityID:   uintPtrFromUint64Ptr(entityID),
			},
		},
	}
}

// findScoped reads the newest entry of a scope; nil when absent.
func (s *StorageServiceImpl) findScoped(
	ctx context.Context,
	filter *filters.FindPluginStorage,
) (*domain.PluginStorageEntry, error) {
	// Newest first: a scope that was duplicated before saves became
	// scope-aware reads back its latest write, not its first.
	newestFirst := []filters.Sorting{{Field: "id", Direction: filters.SortDirectionDesc}}

	entries, err := s.repo.Find(ctx, filter, newestFirst, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, nil
	}

	return &entries[0], nil
}

func (s *StorageServiceImpl) logFailure(ctx context.Context, message string, err error) {
	slog.ErrorContext(ctx, message,
		slog.Uint64("plugin_id", s.pluginID),
		slog.String("error", err.Error()))
}

func (s *StorageServiceImpl) Get(
	ctx context.Context,
	req *storage.StorageGetRequest,
) (*storage.StorageGetResponse, error) {
	entry, err := s.findScoped(ctx, s.scopeFilter(req.Key, req.EntityType, req.EntityId))
	if err != nil {
		s.logFailure(ctx, "failed to read plugin storage entry", err)

		return &storage.StorageGetResponse{
			Found: false,
			Error: new(storageReadFailureMessage),
		}, nil
	}

	if entry == nil {
		return &storage.StorageGetResponse{
			Found: false,
		}, nil
	}

	return &storage.StorageGetResponse{
		Payload: entry.Payload,
		Found:   true,
	}, nil
}

func (s *StorageServiceImpl) Set(
	ctx context.Context,
	req *storage.StorageSetRequest,
) (*storage.StorageSetResponse, error) {
	if len(req.Payload) > s.cfg.MaxValueBytes {
		return storageSetFailure("payload exceeds " + strconv.Itoa(s.cfg.MaxValueBytes) + " bytes"), nil
	}

	existing, err := s.findScoped(ctx, s.scopeFilter(req.Key, req.EntityType, req.EntityId))
	if err != nil {
		s.logFailure(ctx, "failed to read plugin storage entry", err)

		return storageSetFailure(storageReadFailureMessage), nil
	}

	if msg := s.checkQuotas(ctx, existing, len(req.Payload)); msg != "" {
		return storageSetFailure(msg), nil
	}

	entry := &domain.PluginStorageEntry{
		PluginID:   s.pluginID,
		Key:        req.Key,
		EntityType: entityTypeFromProto(req.EntityType),
		EntityID:   uintPtrFromUint64Ptr(req.EntityId),
		Payload:    req.Payload,
	}

	if err := s.repo.Save(ctx, entry); err != nil {
		s.logFailure(ctx, "failed to store plugin storage entry", err)

		return storageSetFailure(storageWriteFailureMessage), nil
	}

	return &storage.StorageSetResponse{
		Success: true,
	}, nil
}

// checkQuotas answers the message refusing a write that would take the
// plugin over its key or byte quota, or "" when it fits. existing is the
// entry being replaced (its size is released), nil for a new key.
func (s *StorageServiceImpl) checkQuotas(
	ctx context.Context,
	existing *domain.PluginStorageEntry,
	payloadSize int,
) string {
	usage, err := s.repo.UsageByPlugin(ctx, s.pluginID)
	if err != nil {
		s.logFailure(ctx, "failed to read plugin storage usage", err)

		return storageReadFailureMessage
	}

	if existing == nil && usage.Keys >= s.cfg.MaxKeysPerPlugin {
		return "at most " + strconv.Itoa(s.cfg.MaxKeysPerPlugin) + " storage entries per plugin"
	}

	released := uint64(0)
	if existing != nil {
		released = uint64(len(existing.Payload))
	}

	// The entry may be deleted between reading it and reading the usage, so
	// its size is no longer part of usage.Bytes; releasing more than that
	// would wrap the unsigned subtraction into a spurious quota refusal.
	if released > usage.Bytes {
		released = usage.Bytes
	}

	if usage.Bytes-released+uint64(payloadSize) > s.cfg.MaxTotalBytes { //nolint:gosec // payloadSize is a length
		return "storage quota of " + strconv.FormatUint(s.cfg.MaxTotalBytes, 10) + " bytes exceeded"
	}

	return ""
}

func storageSetFailure(message string) *storage.StorageSetResponse {
	return &storage.StorageSetResponse{Success: false, Error: new(message)}
}

func (s *StorageServiceImpl) Delete(
	ctx context.Context,
	req *storage.StorageDeleteRequest,
) (*storage.StorageDeleteResponse, error) {
	err := s.repo.DeleteByFilter(ctx, s.scopeFilter(req.Key, req.EntityType, req.EntityId))
	if err != nil {
		s.logFailure(ctx, "failed to delete plugin storage entry", err)

		return &storage.StorageDeleteResponse{
			Success: false,
			Error:   new(storageWriteFailureMessage),
		}, nil
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
		KeyPrefix: req.KeyPrefix,
	}

	if req.EntityType != nil || req.EntityId != nil {
		filter.EntityPairs = []domain.PluginStorageEntityPair{
			{
				EntityType: entityTypeFromProto(req.EntityType),
				EntityID:   uintPtrFromUint64Ptr(req.EntityId),
			},
		}
	}

	// A limit fetches one extra row to learn whether more remain; no limit
	// (plugins built before the field existed) keeps returning everything.
	var pagination *filters.Pagination

	limit := uint64(req.GetLimit())
	if limit > 0 {
		pagination = &filters.Pagination{Limit: limit + 1, Offset: uint64(req.GetOffset())}
	}

	entries, err := s.repo.Find(ctx, filter, nil, pagination)
	if err != nil {
		s.logFailure(ctx, "failed to list plugin storage entries", err)

		return &storage.StorageListResponse{
			Error: new(storageReadFailureMessage),
		}, nil
	}

	hasMore := false
	if limit > 0 && uint64(len(entries)) > limit {
		hasMore = true
		entries = entries[:limit]
	}

	result := make([]*storage.StorageEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, &storage.StorageEntry{
			Key:        entry.Key,
			EntityType: entityTypeToProtoPtr(entry.EntityType),
			EntityId:   uint64PtrFromUintPtr(entry.EntityID),
			Payload:    entry.Payload,
		})
	}

	return &storage.StorageListResponse{
		Entries: result,
		HasMore: hasMore,
	}, nil
}

type StorageHostLibrary struct {
	impl *StorageServiceImpl
}

func NewStorageHostLibrary(
	pluginID uint64,
	repo repositories.PluginStorageRepository,
	opts ...StorageOption,
) *StorageHostLibrary {
	return &StorageHostLibrary{
		impl: NewStorageService(pluginID, repo, opts...),
	}
}

func (l *StorageHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return storage.Instantiate(ctx, r, l.impl)
}

type StorageHostLibraryFactory struct {
	repo repositories.PluginStorageRepository
	opts []StorageOption
}

func NewStorageHostLibraryFactory(
	repo repositories.PluginStorageRepository,
	opts ...StorageOption,
) *StorageHostLibraryFactory {
	return &StorageHostLibraryFactory{repo: repo, opts: opts}
}

func (f *StorageHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewStorageHostLibrary(pluginID, f.repo, f.opts...)
}
