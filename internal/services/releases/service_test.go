package releases_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/services/releases"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const panelReleasesJSON = `[
  {"tag_name": "v4.6.0", "html_url": "https://example.com/v4.6.0", "draft": true, "prerelease": false},
  {"tag_name": "v4.5.0-beta.1", "html_url": "https://example.com/v4.5.0-beta.1", "draft": false, "prerelease": true},
  {"tag_name": "v4.4.2", "html_url": "https://example.com/v4.4.2", "draft": false, "prerelease": false},
  {"tag_name": "v4.4.1", "html_url": "https://example.com/v4.4.1", "draft": false, "prerelease": false},
  {"tag_name": "v4.4.0-beta.1", "html_url": "https://example.com/v4.4.0-beta.1", "draft": false, "prerelease": true},
  {"tag_name": "nightly", "html_url": "https://example.com/nightly", "draft": false, "prerelease": false}
]`

func newService(t *testing.T, urls []string) *releases.Service {
	t.Helper()

	return releases.NewService(releases.Config{
		Enabled: true,
		URLs:    urls,
		TTL:     time.Hour,
	}, cache.NewInMemory())
}

func TestService_Latest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		status    int
		want      releases.Info
		wantError string
	}{
		{
			name:   "picks_newest_stable_and_newer_beta",
			body:   panelReleasesJSON,
			status: http.StatusOK,
			want: releases.Info{
				LatestStable:    "4.4.2",
				LatestStableURL: "https://example.com/v4.4.2",
				LatestBeta:      "4.5.0-beta.1",
				LatestBetaURL:   "https://example.com/v4.5.0-beta.1",
			},
		},
		{
			name: "beta_older_than_stable_is_dropped",
			body: `[
			  {"tag_name": "v4.1.2", "html_url": "https://example.com/v4.1.2"},
			  {"tag_name": "v4.1.0-rc.2", "html_url": "https://example.com/v4.1.0-rc.2", "prerelease": true}
			]`,
			status: http.StatusOK,
			want: releases.Info{
				LatestStable:    "4.1.2",
				LatestStableURL: "https://example.com/v4.1.2",
			},
		},
		{
			name: "prerelease_flag_missing_is_detected_by_tag",
			body: `[
			  {"tag_name": "v4.5.0-beta.1", "html_url": "https://example.com/v4.5.0-beta.1"},
			  {"tag_name": "v4.4.2", "html_url": "https://example.com/v4.4.2"}
			]`,
			status: http.StatusOK,
			want: releases.Info{
				LatestStable:    "4.4.2",
				LatestStableURL: "https://example.com/v4.4.2",
				LatestBeta:      "4.5.0-beta.1",
				LatestBetaURL:   "https://example.com/v4.5.0-beta.1",
			},
		},
		{
			name:   "only_prereleases_leave_stable_empty",
			body:   `[{"tag_name": "v5.0.0-alpha.1", "html_url": "https://example.com/a", "prerelease": true}]`,
			status: http.StatusOK,
			want: releases.Info{
				LatestBeta:    "5.0.0-alpha.1",
				LatestBetaURL: "https://example.com/a",
			},
		},
		{
			name:   "empty_release_list",
			body:   `[]`,
			status: http.StatusOK,
			want:   releases.Info{},
		},
		{
			name:      "malformed_json",
			body:      `{"not": "a list"}`,
			status:    http.StatusOK,
			wantError: "failed to decode releases",
		},
		{
			name:      "server_error",
			body:      "",
			status:    http.StatusInternalServerError,
			wantError: "unexpected HTTP status code: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			service := newService(t, []string{server.URL + "/{component}/releases.json"})

			info, err := service.Latest(context.Background(), releases.ComponentPanel)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.LatestStable, info.LatestStable)
			assert.Equal(t, tt.want.LatestStableURL, info.LatestStableURL)
			assert.Equal(t, tt.want.LatestBeta, info.LatestBeta)
			assert.Equal(t, tt.want.LatestBetaURL, info.LatestBetaURL)
		})
	}
}

// TestService_LatestStripsTagPrefix pins the display form: releases are
// reported without the "v" tag prefix so that they read the same way as the
// versions the panel and the daemons report about themselves.
func TestService_LatestStripsTagPrefix(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(panelReleasesJSON))
	}))
	defer server.Close()

	info, err := newService(t, []string{server.URL + "/{component}/releases.json"}).
		Latest(context.Background(), releases.ComponentPanel)

	require.NoError(t, err)
	assert.Equal(t, "4.4.2", info.LatestStable)
	assert.Equal(t, "4.5.0-beta.1", info.LatestBeta)
	assert.Equal(t, "https://example.com/v4.4.2", info.LatestStableURL, "the release URL keeps its original tag")
}

func TestService_LatestFallsBackToNextSource(t *testing.T) {
	t.Parallel()

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer broken.Close()

	var workingHits atomic.Int32
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workingHits.Add(1)
		assert.Equal(t, "/repos/gameap/daemon/releases", r.URL.Path)
		_, _ = w.Write([]byte(`[{"tag_name": "v4.1.2", "html_url": "https://example.com/v4.1.2"}]`))
	}))
	defer working.Close()

	service := newService(t, []string{
		broken.URL + "/{component}/releases.json",
		working.URL + "/repos/gameap/{repo}/releases",
	})

	info, err := service.Latest(context.Background(), releases.ComponentDaemon)

	require.NoError(t, err)
	assert.Equal(t, "4.1.2", info.LatestStable)
	assert.Equal(t, int32(1), workingHits.Load())
}

func TestService_LatestUsesCacheOnSecondCall(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "/gameap/releases.json", r.URL.Path)
		_, _ = w.Write([]byte(panelReleasesJSON))
	}))
	defer server.Close()

	service := newService(t, []string{server.URL + "/{component}/releases.json"})

	first, err := service.Latest(context.Background(), releases.ComponentPanel)
	require.NoError(t, err)

	second, err := service.Latest(context.Background(), releases.ComponentPanel)
	require.NoError(t, err)

	assert.Equal(t, first.LatestStable, second.LatestStable)
	assert.Equal(t, int32(1), hits.Load(), "second call must be served from cache")
}

func TestService_LatestCachesFailureToAvoidHammering(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	service := newService(t, []string{server.URL + "/{component}/releases.json"})

	_, err := service.Latest(context.Background(), releases.ComponentPanel)
	require.Error(t, err)

	info, err := service.Latest(context.Background(), releases.ComponentPanel)

	require.NoError(t, err)
	assert.Empty(t, info.LatestStable)
	assert.Equal(t, int32(1), hits.Load(), "the negative result must be cached")
}

func TestService_LatestWhenDisabled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be made when the update check is disabled")
	}))
	defer server.Close()

	service := releases.NewService(releases.Config{
		Enabled: false,
		URLs:    []string{server.URL + "/{component}/releases.json"},
		TTL:     time.Hour,
	}, cache.NewInMemory())

	info, err := service.Latest(context.Background(), releases.ComponentPanel)

	require.NoError(t, err)
	assert.False(t, service.Enabled())
	assert.Empty(t, info.LatestStable)
}

func TestService_LatestWithoutSources(t *testing.T) {
	t.Parallel()

	service := newService(t, nil)

	_, err := service.Latest(context.Background(), releases.ComponentPanel)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release sources configured")
}
