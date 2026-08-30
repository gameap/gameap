package getversion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/releases"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResponder struct {
	writeCalled      bool
	writeErrorCalled bool
	lastResult       any
	lastError        error
}

func (m *mockResponder) WriteError(_ context.Context, _ http.ResponseWriter, err error) {
	m.writeErrorCalled = true
	m.lastError = err
}

func (m *mockResponder) Write(_ context.Context, _ http.ResponseWriter, result any) {
	m.writeCalled = true
	m.lastResult = result
}

type stubReleases struct {
	enabled bool
	infos   map[releases.Component]releases.Info
	err     error
}

func (s *stubReleases) Latest(_ context.Context, component releases.Component) (releases.Info, error) {
	if s.err != nil {
		return releases.Info{}, s.err
	}

	return s.infos[component], nil
}

func (s *stubReleases) Enabled() bool {
	return s.enabled
}

func authenticatedRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)

	return req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1, Login: "admin"},
	}))
}

func serve(t *testing.T, service releasesService) (*mockResponder, versionResponse) {
	t.Helper()

	responder := &mockResponder{}
	NewHandler(service, responder).ServeHTTP(httptest.NewRecorder(), authenticatedRequest())

	if !responder.writeCalled {
		return responder, versionResponse{}
	}

	resp, ok := responder.lastResult.(versionResponse)
	require.True(t, ok, "unexpected response type %T", responder.lastResult)

	return responder, resp
}

func TestHandler_ServeHTTPRequiresAuthentication(t *testing.T) {
	t.Parallel()

	responder := &mockResponder{}
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)

	NewHandler(&stubReleases{enabled: true}, responder).ServeHTTP(httptest.NewRecorder(), req)

	assert.False(t, responder.writeCalled)
	require.True(t, responder.writeErrorCalled)
	assert.Contains(t, responder.lastError.Error(), "user not authenticated")
}

func TestHandler_ServeHTTPReturnsPanelAndDaemonVersions(t *testing.T) {
	t.Parallel()

	service := &stubReleases{
		enabled: true,
		infos: map[releases.Component]releases.Info{
			releases.ComponentPanel: {
				LatestStable:    "v4.4.2",
				LatestStableURL: "https://example.com/panel/v4.4.2",
				LatestBeta:      "v4.5.0-beta.1",
				LatestBetaURL:   "https://example.com/panel/v4.5.0-beta.1",
			},
			releases.ComponentDaemon: {
				LatestStable:    "v4.1.2",
				LatestStableURL: "https://example.com/daemon/v4.1.2",
			},
		},
	}

	_, resp := serve(t, service)

	assert.Equal(t, defaults.Version, resp.Panel.Current)
	assert.Equal(t, defaults.BuildDate, resp.Panel.BuildDate)
	assert.Equal(t, "v4.4.2", resp.Panel.LatestStable)
	assert.Equal(t, "https://example.com/panel/v4.4.2", resp.Panel.LatestStableURL)
	assert.Equal(t, "v4.5.0-beta.1", resp.Panel.LatestBeta)
	assert.Equal(t, "v4.1.2", resp.Daemon.LatestStable)
	assert.Equal(t, "https://example.com/daemon/v4.1.2", resp.Daemon.LatestStableURL)
	assert.True(t, resp.UpdateCheckEnabled)
}

func TestHandler_ServeHTTPUpdateAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		current             string
		latestStable        string
		wantUpdateAvailable bool
		wantIsRelease       bool
	}{
		{
			name:                "older_panel_gets_update_notice",
			current:             "4.4.1",
			latestStable:        "v4.4.2",
			wantUpdateAvailable: true,
			wantIsRelease:       true,
		},
		{
			name:          "up_to_date_panel",
			current:       "v4.4.2",
			latestStable:  "v4.4.2",
			wantIsRelease: true,
		},
		{
			name:          "beta_newer_than_latest_stable",
			current:       "4.5.0-beta.1",
			latestStable:  "v4.4.2",
			wantIsRelease: false,
		},
		{
			name:          "development_build_is_never_outdated",
			current:       "development",
			latestStable:  "v4.4.2",
			wantIsRelease: false,
		},
		{
			name:          "no_latest_version_resolved",
			current:       "4.4.1",
			latestStable:  "",
			wantIsRelease: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := newVersionResponse(
				tt.current,
				"2026-08-06T14:42:13Z",
				releases.Info{LatestStable: tt.latestStable},
				releases.Info{},
				true,
			)

			assert.Equal(t, tt.wantUpdateAvailable, resp.Panel.UpdateAvailable)
			assert.Equal(t, tt.wantIsRelease, resp.Panel.IsRelease)
		})
	}
}

func TestHandler_ServeHTTPDegradesWhenReleaseLookupIsUnavailable(t *testing.T) {
	t.Parallel()

	responder, resp := serve(t, &stubReleases{
		enabled: true,
		err:     errors.New("all release sources failed"),
	})

	assert.False(t, responder.writeErrorCalled)
	assert.True(t, responder.writeCalled)
	assert.Equal(t, defaults.Version, resp.Panel.Current)
	assert.Empty(t, resp.Panel.LatestStable)
	assert.Empty(t, resp.Daemon.LatestStable)
	assert.False(t, resp.Panel.UpdateAvailable)
}

func TestHandler_ServeHTTPWithUpdateCheckDisabled(t *testing.T) {
	t.Parallel()

	_, resp := serve(t, &stubReleases{enabled: false})

	assert.False(t, resp.UpdateCheckEnabled)
	assert.Empty(t, resp.Panel.LatestStable)
	assert.Empty(t, resp.Daemon.LatestStable)
	assert.False(t, resp.Panel.UpdateAvailable)
}
