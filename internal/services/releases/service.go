// Package releases resolves the latest available GameAP releases (the panel
// itself and gameap-daemon) from the GameAP CDN, falling back to the GitHub
// releases API. Results are small and kept in the shared cache, so a
// multi-instance panel performs one upstream request per TTL.
package releases

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/version"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

type Component string

const (
	ComponentPanel  Component = "gameap"
	ComponentDaemon Component = "gameap-daemon"
)

// githubRepo maps a component to its GitHub repository name: the daemon lives
// in gameap/daemon while its CDN path is gameap-daemon.
var githubRepo = map[Component]string{
	ComponentPanel:  "gameap",
	ComponentDaemon: "daemon",
}

const (
	httpTimeout    = 15 * time.Second
	cacheKeyPrefix = "releases"
	// negativeTTL keeps a failed lookup cached briefly so that an unreachable
	// CDN is not re-dialed on every dashboard request.
	negativeTTL = 15 * time.Minute
	// maxResponseSize caps the release feed read. The real documents are about
	// 1 MB; the cap only guards against an endless response.
	maxResponseSize = 16 * 1024 * 1024
)

// Info is the resolved state of a component's releases. LatestBeta is filled
// only when the newest pre-release is newer than the newest stable release.
type Info struct {
	LatestStable    string    `json:"latest_stable"`
	LatestStableURL string    `json:"latest_stable_url"`
	LatestBeta      string    `json:"latest_beta"`
	LatestBetaURL   string    `json:"latest_beta_url"`
	CheckedAt       time.Time `json:"checked_at"`
}

// release is the subset of the GitHub release object both the CDN mirrors and
// the GitHub API expose.
type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type Config struct {
	Enabled bool
	URLs    []string
	TTL     time.Duration
}

type Service struct {
	config     Config
	httpClient *http.Client
	cache      cache.Cache

	sf singleflight.Group
}

func NewService(cfg Config, c cache.Cache) *Service {
	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		cache: c,
	}
}

// Enabled reports whether the panel is allowed to contact the release sources.
func (s *Service) Enabled() bool {
	return s.config.Enabled
}

// Latest returns the newest releases of a component. With the update check
// disabled it returns an empty Info without touching the network.
func (s *Service) Latest(ctx context.Context, component Component) (Info, error) {
	if !s.config.Enabled {
		return Info{}, nil
	}

	key := cacheKey(component)

	if cached, err := cache.GetTyped[Info](ctx, s.cache, key); err == nil {
		return cached, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		slog.WarnContext(ctx, "failed to read releases from cache", "component", component, "error", err)
	}

	result, err, _ := s.sf.Do(string(component), func() (any, error) {
		return s.fetchAndCache(ctx, component, key)
	})
	if err != nil {
		return Info{}, err
	}

	info, ok := result.(Info)
	if !ok {
		return Info{}, errors.New("unexpected singleflight result type")
	}

	return info, nil
}

func (s *Service) fetchAndCache(ctx context.Context, component Component, key string) (Info, error) {
	info, err := s.fetch(ctx, component)
	if err != nil {
		s.store(ctx, key, Info{CheckedAt: time.Now()}, negativeTTL)

		return Info{}, err
	}

	s.store(ctx, key, info, s.config.TTL)

	return info, nil
}

func (s *Service) fetch(ctx context.Context, component Component) (Info, error) {
	var lastErr error

	for _, template := range s.config.URLs {
		url := buildURL(template, component)
		if url == "" {
			continue
		}

		releaseList, err := s.fetchReleases(ctx, url)
		if err != nil {
			slog.DebugContext(ctx, "failed to fetch releases",
				"component", component, "url", url, "error", err,
			)
			lastErr = err

			continue
		}

		return newInfo(releaseList), nil
	}

	if lastErr == nil {
		return Info{}, errors.New("no release sources configured")
	}

	return Info{}, errors.WithMessage(lastErr, "all release sources failed")
}

func (s *Service) fetchReleases(ctx context.Context, url string) ([]release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.WarnContext(ctx, "failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
	}

	var releaseList []release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&releaseList); err != nil {
		return nil, errors.Wrap(err, "failed to decode releases")
	}

	return releaseList, nil
}

func (s *Service) store(ctx context.Context, key string, info Info, ttl time.Duration) {
	if err := s.cache.Set(ctx, key, info, cache.WithExpiration(ttl)); err != nil {
		slog.WarnContext(ctx, "failed to write releases to cache", "key", key, "error", err)
	}
}

func newInfo(releaseList []release) Info {
	info := Info{CheckedAt: time.Now()}

	for _, rel := range releaseList {
		if rel.Draft || version.Normalize(rel.TagName) == "" {
			continue
		}

		if rel.Prerelease || !version.IsRelease(rel.TagName) {
			if info.LatestBeta == "" || version.IsNewer(info.LatestBeta, rel.TagName) {
				info.LatestBeta = displayVersion(rel.TagName)
				info.LatestBetaURL = rel.HTMLURL
			}

			continue
		}

		if info.LatestStable == "" || version.IsNewer(info.LatestStable, rel.TagName) {
			info.LatestStable = displayVersion(rel.TagName)
			info.LatestStableURL = rel.HTMLURL
		}
	}

	// A pre-release older than the newest stable release is not worth showing.
	if info.LatestStable != "" && !version.IsNewer(info.LatestStable, info.LatestBeta) {
		info.LatestBeta = ""
		info.LatestBetaURL = ""
	}

	return info
}

// displayVersion drops the "v" tag prefix so that a release reads the same way
// as the versions the panel and the daemons report about themselves.
func displayVersion(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// buildURL expands a source template. {component} is the CDN path segment
// (gameap, gameap-daemon) and {repo} the GitHub repository name.
func buildURL(template string, component Component) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}

	replaced := strings.ReplaceAll(template, "{component}", string(component))

	return strings.ReplaceAll(replaced, "{repo}", githubRepo[component])
}

func cacheKey(component Component) string {
	return cacheKeyPrefix + ":" + string(component)
}
