package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
)

// GlobalAPIService provides methods to interact with the GameAP Global API.
type GlobalAPIService struct {
	config     *config.Config
	httpClient *http.Client
}

// NewGlobalAPIService creates a new GlobalAPI service.
func NewGlobalAPIService(cfg *config.Config) *GlobalAPIService {
	return &GlobalAPIService{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Games fetches the list of games from the Global API.
func (s *GlobalAPIService) Games(ctx context.Context) ([]domain.GlobalAPIGame, error) {
	url := s.config.GlobalAPI.URL + "/games"

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

	if !apiResp.Success {
		return nil, errors.New("API error: " + apiResp.Message)
	}

	return apiResp.Data, nil
}

// BugReport represents the data to send when reporting a bug.
type BugReport struct {
	Version     string
	Summary     string
	Description string
	Environment string
}

// SendBug sends a bug report to the Global API.
func (s *GlobalAPIService) SendBug(ctx context.Context, report BugReport) error {
	url := s.config.GlobalAPI.URL + "/bugs"

	// Enhance environment information
	environment := report.Environment
	environment += fmt.Sprintf("Go version: %s\n", runtime.Version())
	environment += fmt.Sprintf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	payload := map[string]string{
		"version":     report.Version,
		"summary":     report.Summary,
		"description": report.Description,
		"environment": environment,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return errors.Wrap(err, "failed to execute request")
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Warn("failed to close response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return errors.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
	}

	return nil
}
