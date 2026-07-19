package pluginstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/pkg/errors"
)

const (
	defaultBaseURL  = "https://plugins.gameap.dev/api"
	httpTimeout     = 30 * time.Second
	cacheTTL        = 5 * time.Minute
	cacheKeyPrefix  = "pluginstore"
	maxDownloadSize = 100 * 1024 * 1024 // 100MB
	maxIconSize     = 5 * 1024 * 1024   // 5MB
)

type Service struct {
	baseURL    string
	licenseKey string
	httpClient *http.Client
	cache      cache.Cache
}

func NewService(baseURL string, licenseKey string, c cache.Cache) *Service {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &Service{
		baseURL:    baseURL,
		licenseKey: licenseKey,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		cache: c,
	}
}

func (s *Service) BaseURL() string {
	return s.baseURL
}

func (s *Service) HasLicenseKey() bool {
	return s.licenseKey != ""
}

func (s *Service) ValidateLicense(ctx context.Context) (*LicenseValidation, error) {
	if s.licenseKey == "" {
		return nil, nil
	}

	cacheKey := s.buildCacheKey("license", "", "")

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if validation, ok := cached.(*LicenseValidation); ok {
			return validation, nil
		}
	}

	validation, err := fetchJSONWithLicenseKey[LicenseValidation](ctx, s, "/licenses/validate", "", nil)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, &validation)

	return &validation, nil
}

func (s *Service) GetCategories(ctx context.Context, lang string) ([]Category, error) {
	cacheKey := s.buildCacheKey("categories", lang, "")

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if categories, ok := cached.([]Category); ok {
			return categories, nil
		}
	}

	categories, err := fetchJSON[[]Category](ctx, s, "/categories", lang, nil)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, categories)

	return categories, nil
}

func (s *Service) GetLabels(ctx context.Context, lang string) ([]Label, error) {
	cacheKey := s.buildCacheKey("labels", lang, "")

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if labels, ok := cached.([]Label); ok {
			return labels, nil
		}
	}

	labels, err := fetchJSON[[]Label](ctx, s, "/labels", lang, nil)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, labels)

	return labels, nil
}

type GetPluginsParams struct {
	Page      int
	PerPage   int
	SortBy    string
	SortOrder string
	Category  string
	Label     string
}

func (s *Service) GetPlugins(
	ctx context.Context,
	lang string,
	params GetPluginsParams,
) (*PaginatedResponse[Plugin], error) {
	query := url.Values{}
	if params.Page > 0 {
		query.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		query.Set("per_page", strconv.Itoa(params.PerPage))
	}
	if params.SortBy != "" {
		query.Set("sort_by", params.SortBy)
	}
	if params.SortOrder != "" {
		query.Set("sort_order", params.SortOrder)
	}
	if params.Category != "" {
		query.Set("category", params.Category)
	}
	if params.Label != "" {
		query.Set("label", params.Label)
	}

	cacheKey := s.buildCacheKey("plugins", lang, query.Encode())

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if plugins, ok := cached.(*PaginatedResponse[Plugin]); ok {
			return plugins, nil
		}
	}

	plugins, err := fetchJSON[PaginatedResponse[Plugin]](ctx, s, "/plugins", lang, query)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, &plugins)

	return &plugins, nil
}

func (s *Service) GetPlugin(ctx context.Context, pluginID string, lang string) (*PluginDetails, error) {
	cacheKey := s.buildCacheKey("plugin:"+pluginID, lang, "")

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if plugin, ok := cached.(*PluginDetails); ok {
			return plugin, nil
		}
	}

	plugin, err := fetchJSON[PluginDetails](ctx, s, "/plugins/"+pluginID, lang, nil)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, &plugin)

	return &plugin, nil
}

type GetPluginVersionsParams struct {
	Page    int
	PerPage int
}

func (s *Service) GetPluginVersions(
	ctx context.Context,
	pluginID string,
	params GetPluginVersionsParams,
) (*PaginatedResponse[PluginVersion], error) {
	query := url.Values{}
	if params.Page > 0 {
		query.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		query.Set("per_page", strconv.Itoa(params.PerPage))
	}

	cacheKey := s.buildCacheKey("plugin:"+pluginID+":versions", "", query.Encode())

	if cached, err := s.getFromCache(ctx, cacheKey); err == nil {
		if versions, ok := cached.(*PaginatedResponse[PluginVersion]); ok {
			return versions, nil
		}
	}

	versions, err := fetchJSON[PaginatedResponse[PluginVersion]](
		ctx, s, "/plugins/"+pluginID+"/versions", "", query,
	)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, &versions)

	return &versions, nil
}

func (s *Service) DownloadPlugin(
	ctx context.Context,
	pluginID string,
	version string,
) ([]byte, error) {
	downloadURL := fmt.Sprintf("%s/plugins/%s/versions/%s/download", s.baseURL, pluginID, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create download request")
	}

	if s.licenseKey != "" {
		req.Header.Set("X-License-Key", s.licenseKey)
	}

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute download request")
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.Body, resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxDownloadSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read download response")
	}

	return data, nil
}

func (s *Service) GetPluginIcon(ctx context.Context, pluginID string) (*PluginIcon, error) {
	cacheKey := s.buildCacheKey("plugin:"+pluginID+":icon", "", "")

	if s.cache != nil {
		if icon, err := cache.GetTyped[PluginIcon](ctx, s.cache, cacheKey); err == nil {
			return &icon, nil
		}
	}

	iconURL := s.baseURL + "/plugins/" + url.PathEscape(pluginID) + "/icon"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create icon request")
	}

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute icon request")
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.Body, resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxIconSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read icon response")
	}

	icon := PluginIcon{
		Data:        data,
		ContentType: resp.Header.Get("Content-Type"),
	}

	s.setCache(ctx, cacheKey, icon)

	return &icon, nil
}

func VerifyHash(data []byte, expectedHash string) bool {
	hash := sha256.Sum256(data)
	actualHash := hex.EncodeToString(hash[:])

	return actualHash == expectedHash
}

func ExtractLanguage(r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}

	return r.Header.Get("Accept-Language")
}

func (s *Service) buildCacheKey(operation, lang, params string) string {
	key := cacheKeyPrefix + ":" + operation
	if lang != "" {
		key += ":" + lang
	}
	if params != "" {
		key += ":" + params
	}

	return key
}

func (s *Service) getFromCache(ctx context.Context, key string) (any, error) {
	if s.cache == nil {
		return nil, cache.ErrNotFound
	}

	return s.cache.Get(ctx, key)
}

func (s *Service) setCache(ctx context.Context, key string, value any) {
	if s.cache == nil {
		return
	}

	if err := s.cache.Set(ctx, key, value, cache.WithExpiration(cacheTTL)); err != nil {
		slog.WarnContext(ctx, "failed to set cache", slog.String("key", key), slog.Any("error", err))
	}
}

func fetchJSON[T any](
	ctx context.Context,
	s *Service,
	path string,
	lang string,
	query url.Values,
) (T, error) {
	return doFetchJSON[T](ctx, s, path, lang, query, false)
}

func fetchJSONWithLicenseKey[T any](
	ctx context.Context,
	s *Service,
	path string,
	lang string,
	query url.Values,
) (T, error) {
	return doFetchJSON[T](ctx, s, path, lang, query, true)
}

func doFetchJSON[T any](
	ctx context.Context,
	s *Service,
	path string,
	lang string,
	query url.Values,
	includeLicenseKey bool,
) (T, error) {
	var zero T

	requestURL := s.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return zero, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Accept", "application/json")
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	if includeLicenseKey && s.licenseKey != "" {
		req.Header.Set("X-License-Key", s.licenseKey)
	}

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return zero, errors.Wrap(err, "failed to execute request")
	}
	defer closeBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return zero, parseAPIError(resp.Body, resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, errors.Wrap(err, "failed to decode response")
	}

	return result, nil
}

func closeBody(body io.ReadCloser) {
	if err := body.Close(); err != nil {
		slog.Warn("failed to close response body", slog.Any("error", err))
	}
}
