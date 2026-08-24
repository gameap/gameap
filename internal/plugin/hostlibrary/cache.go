package hostlibrary

import (
	"context"
	"errors"
	"strconv"
	"time"

	intcache "github.com/gameap/gameap/internal/cache"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/cache"
	"github.com/tetratelabs/wazero"
)

// CacheServiceImpl serves one plugin: keyPrefix is the plugin's own
// namespace in the panel cache, so plugins never see each other's entries.
type CacheServiceImpl struct {
	cache     intcache.Cache
	keyPrefix string
	// maxValueBytes caps a single value; 0 = unlimited.
	maxValueBytes int
}

// CacheOption tunes a CacheServiceImpl.
type CacheOption func(*CacheServiceImpl)

// WithCacheMaxValueBytes caps the size of a value a plugin may cache; 0
// removes the cap.
func WithCacheMaxValueBytes(maxBytes int) CacheOption {
	return func(s *CacheServiceImpl) {
		s.maxValueBytes = maxBytes
	}
}

func NewCacheService(c intcache.Cache, keyPrefix string, opts ...CacheOption) *CacheServiceImpl {
	service := &CacheServiceImpl{
		cache:     c,
		keyPrefix: keyPrefix,
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

func (s *CacheServiceImpl) prefixedKey(key string) string {
	return s.keyPrefix + key
}

func (s *CacheServiceImpl) Get(
	ctx context.Context,
	req *cache.CacheGetRequest,
) (*cache.CacheGetResponse, error) {
	value, err := s.cache.Get(ctx, s.prefixedKey(req.Key))
	if err != nil {
		if errors.Is(err, intcache.ErrNotFound) {
			return &cache.CacheGetResponse{
				Found: false,
			}, nil
		}

		return nil, err
	}

	bytes, ok := value.([]byte)
	if !ok {
		return &cache.CacheGetResponse{
			Found: false,
		}, nil
	}

	return &cache.CacheGetResponse{
		Value: bytes,
		Found: true,
	}, nil
}

func (s *CacheServiceImpl) Set(
	ctx context.Context,
	req *cache.CacheSetRequest,
) (*cache.CacheSetResponse, error) {
	if s.maxValueBytes > 0 && len(req.Value) > s.maxValueBytes {
		return &cache.CacheSetResponse{
			Success: false,
			Error: new("value too large: " + strconv.Itoa(len(req.Value)) +
				" bytes exceeds the cache value limit of " + strconv.Itoa(s.maxValueBytes) + " bytes"),
		}, nil
	}

	var opts []intcache.Option
	if req.TtlSeconds > 0 {
		opts = append(opts, intcache.WithExpiration(time.Duration(req.TtlSeconds)*time.Second))
	}

	err := s.cache.Set(ctx, s.prefixedKey(req.Key), req.Value, opts...)
	if err != nil {
		return &cache.CacheSetResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &cache.CacheSetResponse{
		Success: true,
	}, nil
}

func (s *CacheServiceImpl) Delete(
	ctx context.Context,
	req *cache.CacheDeleteRequest,
) (*cache.CacheDeleteResponse, error) {
	err := s.cache.Delete(ctx, s.prefixedKey(req.Key))
	if err != nil && !errors.Is(err, intcache.ErrNotFound) {
		return &cache.CacheDeleteResponse{
			Success: false,
		}, nil
	}

	return &cache.CacheDeleteResponse{
		Success: true,
	}, nil
}

type CacheHostLibrary struct {
	impl *CacheServiceImpl
}

func NewCacheHostLibrary(c intcache.Cache, keyPrefix string, opts ...CacheOption) *CacheHostLibrary {
	return &CacheHostLibrary{
		impl: NewCacheService(c, keyPrefix, opts...),
	}
}

func (l *CacheHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return cache.Instantiate(ctx, r, l.impl)
}

// CacheHostLibraryFactory builds a per-plugin cache module whose keys live
// under "<prefix><plugin db id>:", so a plugin cannot read or overwrite
// another plugin's entries (or the panel's own) through the shared cache.
type CacheHostLibraryFactory struct {
	cache     intcache.Cache
	keyPrefix string
	opts      []CacheOption
}

func NewCacheHostLibraryFactory(c intcache.Cache, keyPrefix string, opts ...CacheOption) *CacheHostLibraryFactory {
	return &CacheHostLibraryFactory{
		cache:     c,
		keyPrefix: keyPrefix,
		opts:      opts,
	}
}

// PluginCacheKeyPrefix is the namespace of one plugin's cache entries.
func PluginCacheKeyPrefix(keyPrefix string, pluginID uint64) string {
	return keyPrefix + strconv.FormatUint(pluginID, 10) + ":"
}

func (f *CacheHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewCacheHostLibrary(f.cache, PluginCacheKeyPrefix(f.keyPrefix, pluginID), f.opts...)
}
