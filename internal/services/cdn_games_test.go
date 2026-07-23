package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCDNGamesService(urls ...string) *CDNGamesService {
	cfg := &config.Config{}
	cfg.GamesCDN.URLs = urls

	return NewCDNGamesService(cfg)
}

func gamesResponseHandler(games []domain.GlobalAPIGame) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domain.GlobalAPIResponse[[]domain.GlobalAPIGame]{
			Data:    games,
			Message: "Games retrieved successfully",
			Success: true,
		})
	}
}

func TestCDNGamesService_Games(t *testing.T) {
	t.Run("fetches_from_primary_mirror", func(t *testing.T) {
		var primaryHits, secondaryHits atomic.Int32

		primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			primaryHits.Add(1)

			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Accept"))

			gamesResponseHandler([]domain.GlobalAPIGame{
				{Code: "cstrike", Name: "Counter-Strike 1.6", Engine: "GoldSource"},
			})(w, r)
		}))
		defer primary.Close()

		secondary := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			secondaryHits.Add(1)
		}))
		defer secondary.Close()

		service := newCDNGamesService(primary.URL, secondary.URL)

		games, err := service.Games(context.Background())

		require.NoError(t, err)
		require.Len(t, games, 1)
		assert.Equal(t, "cstrike", games[0].Code)
		assert.Equal(t, int32(1), primaryHits.Load())
		assert.Equal(t, int32(0), secondaryHits.Load(), "secondary mirror must not be hit when primary succeeds")
	})

	t.Run("falls_back_to_secondary_mirror_on_error_status", func(t *testing.T) {
		primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer primary.Close()

		secondary := httptest.NewServer(gamesResponseHandler([]domain.GlobalAPIGame{
			{Code: "css", Name: "Counter-Strike Source", Engine: "Source"},
		}))
		defer secondary.Close()

		service := newCDNGamesService(primary.URL, secondary.URL)

		games, err := service.Games(context.Background())

		require.NoError(t, err)
		require.Len(t, games, 1)
		assert.Equal(t, "css", games[0].Code)
	})

	t.Run("falls_back_to_secondary_mirror_on_invalid_json", func(t *testing.T) {
		primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer primary.Close()

		secondary := httptest.NewServer(gamesResponseHandler([]domain.GlobalAPIGame{
			{Code: "rust", Name: "Rust", Engine: "Source"},
		}))
		defer secondary.Close()

		service := newCDNGamesService(primary.URL, secondary.URL)

		games, err := service.Games(context.Background())

		require.NoError(t, err)
		require.Len(t, games, 1)
		assert.Equal(t, "rust", games[0].Code)
	})

	t.Run("returns_error_when_all_mirrors_fail", func(t *testing.T) {
		primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer primary.Close()

		secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer secondary.Close()

		service := newCDNGamesService(primary.URL, secondary.URL)

		_, err := service.Games(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch games from all CDN mirrors")
	})

	t.Run("returns_error_when_no_urls_configured", func(t *testing.T) {
		service := newCDNGamesService()

		_, err := service.Games(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no games CDN URLs configured")
	})

	t.Run("empty_catalog_is_a_success", func(t *testing.T) {
		primary := httptest.NewServer(gamesResponseHandler([]domain.GlobalAPIGame{}))
		defer primary.Close()

		service := newCDNGamesService(primary.URL)

		games, err := service.Games(context.Background())

		require.NoError(t, err)
		assert.Empty(t, games)
	})
}

func TestCDNGamesService_Games_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	service := newCDNGamesService(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Games(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
