package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
)

// CDNGamesService fetches the games and mods catalog from the GameAP CDN.
// The configured mirror URLs are tried in order; the catalog from the first
// mirror that returns a valid document is used.
type CDNGamesService struct {
	urls       []string
	httpClient *http.Client
}

// NewCDNGamesService creates a new CDN games service.
func NewCDNGamesService(cfg *config.Config) *CDNGamesService {
	return &CDNGamesService{
		urls: cfg.GamesCDN.URLs,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Games fetches the games catalog from the CDN, trying each configured mirror
// in order until one responds with a valid document.
func (s *CDNGamesService) Games(ctx context.Context) ([]domain.GlobalAPIGame, error) {
	if len(s.urls) == 0 {
		return nil, errors.New("no games CDN URLs configured")
	}

	var lastErr error

	for _, url := range s.urls {
		games, err := s.fetch(ctx, url)
		if err != nil {
			lastErr = err

			slog.WarnContext(ctx, "failed to fetch games from CDN mirror",
				slog.String("url", url),
				slog.String("error", err.Error()),
			)

			continue
		}

		return games, nil
	}

	return nil, errors.Wrap(lastErr, "failed to fetch games from all CDN mirrors")
}

func (s *CDNGamesService) fetch(ctx context.Context, url string) ([]domain.GlobalAPIGame, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Warn("failed to close response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
	}

	var apiResp domain.GlobalAPIResponse[[]domain.GlobalAPIGame]
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return apiResp.Data, nil
}
