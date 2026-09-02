package base

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullGameMod is a game mod with every column populated, so a field the mapper
// forgets shows up as a zero value instead of hiding behind another test's nil.
func fullGameMod() domain.GameMod {
	return domain.GameMod{
		ID:       7,
		GameCode: "cstrike",
		Name:     "Classic",
		FastRcon: domain.GameModFastRconList{
			{Info: "Restart map", Command: "restart"},
			{
				Info:    "Kick",
				Command: "kick {player}",
				I18n:    domain.GameModFastRconI18n{"ru": {Info: "Кик"}},
			},
		},
		Vars: domain.GameModVarList{
			{
				Var:         "maxplayers",
				Default:     "32",
				Info:        "Max players",
				AdminVar:    true,
				Type:        domain.GameModVarTypeInt,
				Description: "Player slots",
				Rules:       &domain.GameModVarRules{Min: new(1.0), Max: new(64.0)},
				I18n:        domain.GameModVarI18n{"ru": {Info: "Максимум игроков"}},
			},
			{
				Var:         "difficulty",
				Default:     "easy",
				Info:        "Difficulty",
				Type:        domain.GameModVarTypeSelect,
				Options:     domain.GameModVarOptions{{Value: "easy"}},
				AllowCustom: true,
			},
			{
				Var:        "pvp",
				Default:    "on",
				Info:       "PvP enabled",
				Type:       domain.GameModVarTypeBool,
				TrueValue:  new("on"),
				FalseValue: new("off"),
			},
		},
		RemoteRepositoryLinux:   new("https://cdn.example.com/linux"),
		RemoteRepositoryWindows: new("https://cdn.example.com/windows"),
		LocalRepositoryLinux:    new("/srv/repo/linux"),
		LocalRepositoryWindows:  new(`C:\repo\windows`),
		StartCmdLinux:           new("./hlds_run -game cstrike"),
		StartCmdWindows:         new("hlds.exe -game cstrike"),
		KickCmd:                 new("kick {player}"),
		BanCmd:                  new("ban {player}"),
		ChnameCmd:               new("chname {player} {name}"),
		SrestartCmd:             new("restart"),
		ChmapCmd:                new("changelevel {map}"),
		SendmsgCmd:              new("say {msg}"),
		PasswdCmd:               new("sv_password {password}"),
		Metadata:                domain.Metadata{"source": "catalog", "version": 3.0},
	}
}

func TestNewGameModResponseFromGameMod(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gameMod := fullGameMod()

	// ACT
	got := NewGameModResponseFromGameMod(&gameMod)

	// ASSERT
	assert.Equal(t, uint(7), got.ID)
	assert.Equal(t, "cstrike", got.GameCode)
	assert.Equal(t, "Classic", got.Name)
	assert.Equal(t, gameMod.RemoteRepositoryLinux, got.RemoteRepositoryLinux)
	assert.Equal(t, gameMod.RemoteRepositoryWindows, got.RemoteRepositoryWindows)
	assert.Equal(t, gameMod.LocalRepositoryLinux, got.LocalRepositoryLinux)
	assert.Equal(t, gameMod.LocalRepositoryWindows, got.LocalRepositoryWindows)
	assert.Equal(t, gameMod.StartCmdLinux, got.StartCmdLinux)
	assert.Equal(t, gameMod.StartCmdWindows, got.StartCmdWindows)
	assert.Equal(t, gameMod.KickCmd, got.KickCmd)
	assert.Equal(t, gameMod.BanCmd, got.BanCmd)
	assert.Equal(t, gameMod.ChnameCmd, got.ChnameCmd)
	assert.Equal(t, gameMod.SrestartCmd, got.SrestartCmd)
	assert.Equal(t, gameMod.ChmapCmd, got.ChmapCmd)
	assert.Equal(t, gameMod.SendmsgCmd, got.SendmsgCmd)
	assert.Equal(t, gameMod.PasswdCmd, got.PasswdCmd)
	assert.Equal(t, map[string]any{"source": "catalog", "version": 3.0}, got.Metadata)
}

func TestNewGameModResponseFromGameMod_FastRcon(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gameMod := fullGameMod()

	// ACT
	got := NewGameModResponseFromGameMod(&gameMod)

	// ASSERT
	require.Len(t, got.FastRcon, 2)
	assert.Equal(t, "Restart map", got.FastRcon[0].Info)
	assert.Equal(t, "restart", got.FastRcon[0].Command)
	assert.Nil(t, got.FastRcon[0].I18n, "an entry without translations must omit the i18n key")
	assert.Equal(t, "Kick", got.FastRcon[1].Info, "the stored order is preserved")
	assert.Equal(t, "kick {player}", got.FastRcon[1].Command)
	assert.Equal(t, domain.GameModFastRconI18n{"ru": {Info: "Кик"}}, got.FastRcon[1].I18n)
}

func TestNewGameModResponseFromGameMod_Vars(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gameMod := fullGameMod()

	// ACT
	got := NewGameModResponseFromGameMod(&gameMod)

	// ASSERT
	require.Len(t, got.Vars, 3)

	intVar := got.Vars[0]
	assert.Equal(t, "maxplayers", intVar.Var)
	assert.Equal(t, "32", intVar.Default, "the default is rendered as the literal text it is stored as")
	assert.Equal(t, "Max players", intVar.Info)
	assert.True(t, intVar.AdminVar)
	assert.Equal(t, domain.GameModVarTypeInt, intVar.Type)
	assert.Equal(t, "Player slots", intVar.Description)
	assert.Equal(t, gameMod.Vars[0].Rules, intVar.Rules)
	assert.Equal(t, domain.GameModVarI18n{"ru": {Info: "Максимум игроков"}}, intVar.I18n)
	assert.False(t, intVar.AllowCustom)
	assert.Nil(t, intVar.TrueValue)
	assert.Nil(t, intVar.FalseValue)

	selectVar := got.Vars[1]
	assert.Equal(t, "difficulty", selectVar.Var, "the stored order is preserved")
	assert.Equal(t, domain.GameModVarTypeSelect, selectVar.Type)
	assert.True(t, selectVar.AllowCustom)
	require.Len(t, selectVar.Options, 1)
	assert.Equal(t, "easy", selectVar.Options[0].Value)

	boolVar := got.Vars[2]
	assert.Equal(t, "pvp", boolVar.Var)
	assert.Equal(t, new("on"), boolVar.TrueValue)
	assert.Equal(t, new("off"), boolVar.FalseValue)
}

func TestNewGameModResponseFromGameMod_Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		options     domain.GameModVarOptions
		wantOptions []gameModVarOption
	}{
		{
			name:        "an_empty_option_list_is_omitted",
			options:     domain.GameModVarOptions{},
			wantOptions: nil,
		},
		{
			name:        "a_nil_option_list_is_omitted",
			options:     nil,
			wantOptions: nil,
		},
		{
			// The stored shorthand form carries no label; the editor still needs
			// something to show, so the value doubles as the label.
			name:        "a_shorthand_option_is_labelled_by_its_value",
			options:     domain.GameModVarOptions{{Value: "easy"}},
			wantOptions: []gameModVarOption{{Value: "easy", Label: "easy"}},
		},
		{
			name: "an_option_keeps_its_own_label_and_translations",
			options: domain.GameModVarOptions{
				{Value: "hard", Label: "Hard mode", I18n: domain.GameModVarOptionI18n{"ru": {Label: "Сложно"}}},
			},
			wantOptions: []gameModVarOption{
				{Value: "hard", Label: "Hard mode", I18n: domain.GameModVarOptionI18n{"ru": {Label: "Сложно"}}},
			},
		},
		{
			name: "every_option_is_mapped_in_order",
			options: domain.GameModVarOptions{
				{Value: "easy"},
				{Value: "hard", Label: "Hard mode"},
			},
			wantOptions: []gameModVarOption{
				{Value: "easy", Label: "easy"},
				{Value: "hard", Label: "Hard mode"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			gameMod := domain.GameMod{
				Vars: domain.GameModVarList{
					{
						Var:     "difficulty",
						Info:    "Difficulty",
						Type:    domain.GameModVarTypeSelect,
						Options: tt.options,
					},
				},
			}

			// ACT
			got := NewGameModResponseFromGameMod(&gameMod)

			// ASSERT
			require.Len(t, got.Vars, 1)
			assert.Equal(t, tt.wantOptions, got.Vars[0].Options)
		})
	}
}

func TestNewGameModResponseFromGameMod_EmptyCollections(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gameMod := domain.GameMod{ID: 1, GameCode: "cstrike", Name: "Classic"}

	// ACT
	got := NewGameModResponseFromGameMod(&gameMod)

	// ASSERT
	assert.NotNil(t, got.FastRcon, "fast_rcon has no omitempty and must marshal as [] rather than null")
	assert.Empty(t, got.FastRcon)
	assert.NotNil(t, got.Vars, "vars has no omitempty and must marshal as [] rather than null")
	assert.Empty(t, got.Vars)
	assert.Nil(t, got.Metadata, "an absent metadata bag stays absent")
}

func TestNewGameModsResponseFromGameMods(t *testing.T) {
	t.Parallel()

	t.Run("returns_an_empty_non_nil_list_for_no_game_mods", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		var gameMods []domain.GameMod

		// ACT
		got := NewGameModsResponseFromGameMods(gameMods)

		// ASSERT
		assert.NotNil(t, got, "an empty collection must marshal as [] rather than null")
		assert.Empty(t, got)
	})

	t.Run("maps_every_game_mod_preserving_order", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		gameMods := []domain.GameMod{
			{ID: 3, GameCode: "cstrike", Name: "Classic"},
			{ID: 1, GameCode: "csgo", Name: "Competitive"},
			{ID: 2, GameCode: "rust", Name: "Vanilla"},
		}

		// ACT
		got := NewGameModsResponseFromGameMods(gameMods)

		// ASSERT
		require.Len(t, got, 3)
		assert.Equal(t, []uint{3, 1, 2}, []uint{got[0].ID, got[1].ID, got[2].ID},
			"the order of the given collection is preserved")
		assert.Equal(t, "Classic", got[0].Name)
		assert.Equal(t, "csgo", got[1].GameCode)
		assert.Equal(t, "Vanilla", got[2].Name)
	})

	t.Run("maps_the_nested_collections_of_each_game_mod", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		gameMods := []domain.GameMod{
			fullGameMod(),
			{ID: 8, GameCode: "rust", Name: "Vanilla"},
		}

		// ACT
		got := NewGameModsResponseFromGameMods(gameMods)

		// ASSERT
		require.Len(t, got, 2)
		require.Len(t, got[0].FastRcon, 2)
		require.Len(t, got[0].Vars, 3)
		assert.Equal(t, "maxplayers", got[0].Vars[0].Var)
		assert.Empty(t, got[1].Vars, "a game mod without vars must not inherit its neighbour's")
	})
}
