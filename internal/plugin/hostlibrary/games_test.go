package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGamesService_FindGames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.GameRepository)
		request   *games.FindGamesRequest
		wantTotal int
		wantCodes []string
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.GameRepository) {
				_ = r.Save(context.Background(), &domain.Game{Code: "cs", Name: "Counter-Strike", Enabled: 1})
				_ = r.Save(context.Background(), &domain.Game{Code: "tf2", Name: "Team Fortress 2", Enabled: 1})
				_ = r.Save(context.Background(), &domain.Game{Code: "mc", Name: "Minecraft", Enabled: 1})
			},
			request:   &games.FindGamesRequest{},
			wantTotal: 3,
			wantCodes: []string{"cs", "tf2", "mc"},
		},
		{
			name: "filter_by_codes",
			setupRepo: func(r *inmemory.GameRepository) {
				_ = r.Save(context.Background(), &domain.Game{Code: "cs", Name: "Counter-Strike", Enabled: 1})
				_ = r.Save(context.Background(), &domain.Game{Code: "tf2", Name: "Team Fortress 2", Enabled: 1})
				_ = r.Save(context.Background(), &domain.Game{Code: "mc", Name: "Minecraft", Enabled: 1})
			},
			request: &games.FindGamesRequest{
				Filter: &games.GameFilter{Codes: []string{"cs", "mc"}},
			},
			wantTotal: 2,
			wantCodes: []string{"cs", "mc"},
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.GameRepository) {
				codes := []string{"a", "b", "c", "d", "e"}
				for _, code := range codes {
					_ = r.Save(context.Background(), &domain.Game{Code: code, Name: "Game " + code, Enabled: 1})
				}
			},
			request: &games.FindGamesRequest{
				Pagination: &common.Pagination{Limit: 2, Offset: 1},
			},
			wantTotal: 2,
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.GameRepository) {},
			request:   &games.FindGamesRequest{},
			wantTotal: 0,
			wantCodes: []string{},
		},
		{
			name: "filter_nonexistent_codes",
			setupRepo: func(r *inmemory.GameRepository) {
				_ = r.Save(context.Background(), &domain.Game{Code: "cs", Name: "Counter-Strike", Enabled: 1})
			},
			request: &games.FindGamesRequest{
				Filter: &games.GameFilter{Codes: []string{"nonexistent"}},
			},
			wantTotal: 0,
			wantCodes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameRepository()
			tt.setupRepo(repo)

			svc := NewGamesService(repo)
			resp, err := svc.FindGames(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.Games, tt.wantTotal)

			if len(tt.wantCodes) > 0 {
				actualCodes := make([]string, len(resp.Games))
				for i, game := range resp.Games {
					actualCodes[i] = game.Code
				}
				for _, wantCode := range tt.wantCodes {
					assert.Contains(t, actualCodes, wantCode)
				}
			}
		})
	}
}

func TestGamesService_GetGame(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.GameRepository)
		gameCode  string
		wantFound bool
		wantName  string
	}{
		{
			name: "existing_returns_found",
			setupRepo: func(r *inmemory.GameRepository) {
				_ = r.Save(context.Background(), &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsrc",
				})
			},
			gameCode:  "cs",
			wantFound: true,
			wantName:  "Counter-Strike",
		},
		{
			name:      "missing_returns_not_found",
			setupRepo: func(_ *inmemory.GameRepository) {},
			gameCode:  "nonexistent",
			wantFound: false,
		},
		{
			name: "wrong_code_returns_not_found",
			setupRepo: func(r *inmemory.GameRepository) {
				_ = r.Save(context.Background(), &domain.Game{Code: "cs", Name: "Counter-Strike"})
			},
			gameCode:  "tf2",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameRepository()
			tt.setupRepo(repo)

			svc := NewGamesService(repo)
			resp, err := svc.GetGame(context.Background(), &games.GetGameRequest{Code: tt.gameCode})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				require.NotNil(t, resp.Game)
				assert.Equal(t, tt.wantName, resp.Game.Name)
				assert.Equal(t, tt.gameCode, resp.Game.Code)
			} else {
				assert.Nil(t, resp.Game)
			}
		})
	}
}

func TestConvertGameToProto(t *testing.T) {
	t.Parallel()
	game := &domain.Game{
		Code:                    "cs",
		Name:                    "Counter-Strike",
		Engine:                  "goldsrc",
		EngineVersion:           "1.6",
		SteamAppIDLinux:         new(uint(10)),
		SteamAppIDWindows:       new(uint(10)),
		SteamAppSetConfig:       new("+game cstrike"),
		RemoteRepositoryLinux:   new("https://example.com/linux"),
		RemoteRepositoryWindows: new("https://example.com/windows"),
		Enabled:                 1,
	}

	result := convertGameToProto(game)

	assert.Equal(t, "cs", result.Code)
	assert.Equal(t, "Counter-Strike", result.Name)
	assert.Equal(t, "goldsrc", result.Engine)
	assert.Equal(t, "1.6", result.EngineVersion)
	require.NotNil(t, result.SteamAppIdLinux)
	assert.Equal(t, uint32(10), *result.SteamAppIdLinux)
	require.NotNil(t, result.SteamAppIdWindows)
	assert.Equal(t, uint32(10), *result.SteamAppIdWindows)
	require.NotNil(t, result.SteamAppSetConfig)
	assert.Equal(t, "+game cstrike", *result.SteamAppSetConfig)
	require.NotNil(t, result.RemoteRepositoryLinux)
	assert.Equal(t, "https://example.com/linux", *result.RemoteRepositoryLinux)
	require.NotNil(t, result.RemoteRepositoryWindows)
	assert.Equal(t, "https://example.com/windows", *result.RemoteRepositoryWindows)
	assert.True(t, result.Enabled)
}

func TestConvertGameToProto_NilOptionalFields(t *testing.T) {
	t.Parallel()
	game := &domain.Game{
		Code:    "mc",
		Name:    "Minecraft",
		Enabled: 0,
	}

	result := convertGameToProto(game)

	assert.Equal(t, "mc", result.Code)
	assert.Nil(t, result.SteamAppIdLinux)
	assert.Nil(t, result.SteamAppIdWindows)
	assert.False(t, result.Enabled)
}

func TestNewGamesHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewGameRepository()
	lib := NewGamesHostLibrary(repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
