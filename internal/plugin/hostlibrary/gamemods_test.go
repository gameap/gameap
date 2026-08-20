package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameModsService_FindGameMods(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.GameModRepository)
		request   *gamemods.FindGameModsRequest
		wantTotal int
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.GameModRepository) {
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Classic"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Zombie"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "tf2", Name: "Standard"})
			},
			request:   &gamemods.FindGameModsRequest{},
			wantTotal: 3,
		},
		{
			name: "filter_by_ids",
			setupRepo: func(r *inmemory.GameModRepository) {
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Classic"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Zombie"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "tf2", Name: "Standard"})
			},
			request: &gamemods.FindGameModsRequest{
				Filter: &gamemods.GameModFilter{Ids: []uint64{1, 3}},
			},
			wantTotal: 2,
		},
		{
			name: "filter_by_game_code",
			setupRepo: func(r *inmemory.GameModRepository) {
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Classic"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Zombie"})
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "tf2", Name: "Standard"})
			},
			request: &gamemods.FindGameModsRequest{
				Filter: &gamemods.GameModFilter{GameCode: new("cs")},
			},
			wantTotal: 2,
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.GameModRepository) {
				for i := 1; i <= 10; i++ {
					_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Mod" + string(rune('0'+i))})
				}
			},
			request: &gamemods.FindGameModsRequest{
				Pagination: &common.Pagination{Limit: 3, Offset: 2},
			},
			wantTotal: 3,
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.GameModRepository) {},
			request:   &gamemods.FindGameModsRequest{},
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameModRepository()
			tt.setupRepo(repo)

			svc := NewGameModsService(repo)
			resp, err := svc.FindGameMods(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.GameMods, tt.wantTotal)
		})
	}
}

func TestGameModsService_GetGameMod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.GameModRepository)
		gameModID uint64
		wantFound bool
		wantName  string
	}{
		{
			name: "existing_returns_found",
			setupRepo: func(r *inmemory.GameModRepository) {
				_ = r.Save(context.Background(), &domain.GameMod{
					GameCode: "cs",
					Name:     "Classic",
				})
			},
			gameModID: 1,
			wantFound: true,
			wantName:  "Classic",
		},
		{
			name:      "missing_returns_not_found",
			setupRepo: func(_ *inmemory.GameModRepository) {},
			gameModID: 999,
			wantFound: false,
		},
		{
			name: "wrong_id_returns_not_found",
			setupRepo: func(r *inmemory.GameModRepository) {
				_ = r.Save(context.Background(), &domain.GameMod{GameCode: "cs", Name: "Classic"})
			},
			gameModID: 999,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameModRepository()
			tt.setupRepo(repo)

			svc := NewGameModsService(repo)
			resp, err := svc.GetGameMod(context.Background(), &gamemods.GetGameModRequest{Id: tt.gameModID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				require.NotNil(t, resp.GameMod)
				assert.Equal(t, tt.wantName, resp.GameMod.Name)
				assert.Equal(t, tt.gameModID, resp.GameMod.Id)
			} else {
				assert.Nil(t, resp.GameMod)
			}
		})
	}
}

func TestConvertGameModToProto(t *testing.T) {
	t.Parallel()
	gameMod := &domain.GameMod{
		ID:              42,
		GameCode:        "cs",
		Name:            "Classic",
		StartCmdLinux:   new("./hlds_run -game cstrike"),
		StartCmdWindows: new("hlds.exe -game cstrike"),
		KickCmd:         new("kick %s"),
		BanCmd:          new("banid 0 %s"),
	}

	result := convertGameModToProto(gameMod)

	assert.Equal(t, uint64(42), result.Id)
	assert.Equal(t, "cs", result.GameCode)
	assert.Equal(t, "Classic", result.Name)
	require.NotNil(t, result.StartCmdLinux)
	assert.Equal(t, "./hlds_run -game cstrike", *result.StartCmdLinux)
	require.NotNil(t, result.StartCmdWindows)
	assert.Equal(t, "hlds.exe -game cstrike", *result.StartCmdWindows)
	require.NotNil(t, result.KickCmd)
	assert.Equal(t, "kick %s", *result.KickCmd)
	require.NotNil(t, result.BanCmd)
	assert.Equal(t, "banid 0 %s", *result.BanCmd)
}

func TestConvertGameModToProto_EmptyFields(t *testing.T) {
	t.Parallel()
	gameMod := &domain.GameMod{
		ID:       1,
		GameCode: "mc",
		Name:     "Vanilla",
	}

	result := convertGameModToProto(gameMod)

	assert.Equal(t, uint64(1), result.Id)
	assert.Equal(t, "mc", result.GameCode)
	assert.Equal(t, "Vanilla", result.Name)
	assert.Nil(t, result.StartCmdLinux)
	assert.Nil(t, result.StartCmdWindows)
	assert.Nil(t, result.KickCmd)
	assert.Nil(t, result.BanCmd)
}

func TestNewGameModsHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewGameModRepository()
	lib := NewGameModsHostLibrary(repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
