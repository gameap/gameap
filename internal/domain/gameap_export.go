package domain

import (
	"time"

	"github.com/gameap/gameap/pkg/strings"
	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

const CurrentSchemaVersion = "1.0"

const (
	maxGameCodeLength          = 16
	maxGameNameLength          = 128
	maxEngineLength            = 128
	maxEngineVersionLength     = 128
	maxRepositoryLength        = 128
	maxStartCmdLength          = 1000
	maxCommandLength           = 200
	maxModNameLength           = 255
	maxSteamAppSetConfigLength = 128
)

type GameExport struct {
	SchemaVersion string          `yaml:"schema_version"`
	ExportedAt    string          `yaml:"exported_at,omitempty"`
	ExportedBy    string          `yaml:"exported_by,omitempty"`
	Game          GameExportGame  `yaml:"game"`
	Mods          []GameExportMod `yaml:"mods,omitempty"`
}

type GameExportGame struct {
	Code                    string   `yaml:"code"`
	Name                    string   `yaml:"name"`
	Engine                  string   `yaml:"engine"`
	EngineVersion           string   `yaml:"engine_version,omitempty"`
	SteamAppIDLinux         *uint    `yaml:"steam_app_id_linux,omitempty"`
	SteamAppIDWindows       *uint    `yaml:"steam_app_id_windows,omitempty"`
	SteamAppSetConfig       *string  `yaml:"steam_app_set_config,omitempty"`
	RemoteRepositoryLinux   *string  `yaml:"remote_repository_linux,omitempty"`
	RemoteRepositoryWindows *string  `yaml:"remote_repository_windows,omitempty"`
	LocalRepositoryLinux    *string  `yaml:"local_repository_linux,omitempty"`
	LocalRepositoryWindows  *string  `yaml:"local_repository_windows,omitempty"`
	Metadata                Metadata `yaml:"metadata,omitempty"`
}

type GameExportMod struct {
	Name                    string                  `yaml:"name"`
	FastRcon                []GameExportModFastRcon `yaml:"fast_rcon,omitempty"`
	Vars                    []GameExportModVar      `yaml:"vars,omitempty"`
	RemoteRepositoryLinux   *string                 `yaml:"remote_repository_linux,omitempty"`
	RemoteRepositoryWindows *string                 `yaml:"remote_repository_windows,omitempty"`
	LocalRepositoryLinux    *string                 `yaml:"local_repository_linux,omitempty"`
	LocalRepositoryWindows  *string                 `yaml:"local_repository_windows,omitempty"`
	StartCmdLinux           *string                 `yaml:"start_cmd_linux,omitempty"`
	StartCmdWindows         *string                 `yaml:"start_cmd_windows,omitempty"`
	KickCmd                 *string                 `yaml:"kick_cmd,omitempty"`
	BanCmd                  *string                 `yaml:"ban_cmd,omitempty"`
	ChnameCmd               *string                 `yaml:"chname_cmd,omitempty"`
	SrestartCmd             *string                 `yaml:"srestart_cmd,omitempty"`
	ChmapCmd                *string                 `yaml:"chmap_cmd,omitempty"`
	SendmsgCmd              *string                 `yaml:"sendmsg_cmd,omitempty"`
	PasswdCmd               *string                 `yaml:"passwd_cmd,omitempty"`
	Metadata                Metadata                `yaml:"metadata,omitempty"`
}

// The export format carries the same variable and fast RCON shape as the
// catalog schema, so it reuses the domain types instead of a parallel copy that
// would have to be kept in sync by hand.
type (
	GameExportModFastRcon = GameModFastRcon
	GameExportModVar      = GameModVar
)

func ParseGameExport(data []byte) (*GameExport, error) {
	var export GameExport
	if err := yaml.Unmarshal(data, &export); err != nil {
		return nil, errors.Wrap(err, "failed to parse GameAP YAML")
	}

	return &export, nil
}

func (e *GameExport) ToYAML() ([]byte, error) {
	data, err := yaml.Marshal(e)
	// yaml.Marshal on this struct (no custom MarshalYAML hooks) does not fail in practice;
	// this branch is defensive.
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal GameAP YAML")
	}

	return data, nil
}

func (e *GameExport) Validate() error {
	if e.SchemaVersion == "" {
		return errors.New("schema_version is required")
	}

	if e.SchemaVersion != CurrentSchemaVersion {
		return errors.Errorf("unsupported schema version: %s, expected: %s", e.SchemaVersion, CurrentSchemaVersion)
	}

	if err := e.validateGame(); err != nil {
		return errors.WithMessage(err, "game validation failed")
	}

	if err := e.validateMods(); err != nil {
		return errors.WithMessage(err, "mods validation failed")
	}

	return nil
}

func (e *GameExport) validateGame() error {
	if e.Game.Code == "" {
		return errors.New("game.code is required")
	}

	if len(e.Game.Code) < 2 || len(e.Game.Code) > maxGameCodeLength {
		return errors.Errorf("game.code must be between 2 and %d characters", maxGameCodeLength)
	}

	if !strings.IsSlug(e.Game.Code) {
		return errors.New("game.code must match pattern: ^[a-z0-9_-]+$")
	}

	if e.Game.Name == "" {
		return errors.New("game.name is required")
	}

	if len(e.Game.Name) < 2 || len(e.Game.Name) > maxGameNameLength {
		return errors.Errorf("game.name must be between 2 and %d characters", maxGameNameLength)
	}

	if e.Game.Engine == "" {
		return errors.New("game.engine is required")
	}

	if len(e.Game.Engine) > maxEngineLength {
		return errors.Errorf("game.engine must be at most %d characters", maxEngineLength)
	}

	if len(e.Game.EngineVersion) > maxEngineVersionLength {
		return errors.Errorf("game.engine_version must be at most %d characters", maxEngineVersionLength)
	}

	if e.Game.RemoteRepositoryLinux != nil && len(*e.Game.RemoteRepositoryLinux) > maxRepositoryLength {
		return errors.Errorf("game.remote_repository_linux must be at most %d characters", maxRepositoryLength)
	}

	if e.Game.RemoteRepositoryWindows != nil && len(*e.Game.RemoteRepositoryWindows) > maxRepositoryLength {
		return errors.Errorf("game.remote_repository_windows must be at most %d characters", maxRepositoryLength)
	}

	if e.Game.LocalRepositoryLinux != nil && len(*e.Game.LocalRepositoryLinux) > maxRepositoryLength {
		return errors.Errorf("game.local_repository_linux must be at most %d characters", maxRepositoryLength)
	}

	if e.Game.LocalRepositoryWindows != nil && len(*e.Game.LocalRepositoryWindows) > maxRepositoryLength {
		return errors.Errorf("game.local_repository_windows must be at most %d characters", maxRepositoryLength)
	}

	if e.Game.SteamAppSetConfig != nil && len(*e.Game.SteamAppSetConfig) > maxSteamAppSetConfigLength {
		return errors.Errorf("game.steam_app_set_config must be at most %d characters", maxSteamAppSetConfigLength)
	}

	return nil
}

func (e *GameExport) validateMods() error {
	modNames := make(map[string]struct{})

	for i, mod := range e.Mods {
		if mod.Name == "" {
			return errors.Errorf("mods[%d].name is required", i)
		}

		if len(mod.Name) > maxModNameLength {
			return errors.Errorf("mods[%d].name must be at most %d characters", i, maxModNameLength)
		}

		if _, exists := modNames[mod.Name]; exists {
			return errors.Errorf("duplicate mod name: %s", mod.Name)
		}
		modNames[mod.Name] = struct{}{}

		if err := validateModCommands(&mod, i); err != nil {
			return err
		}

		if err := GameModVarList(mod.Vars).Validate(); err != nil {
			return errors.WithMessagef(err, "mods[%d]", i)
		}

		if err := GameModFastRconList(mod.FastRcon).Validate(); err != nil {
			return errors.WithMessagef(err, "mods[%d]", i)
		}
	}

	return nil
}

func validateModCommands(mod *GameExportMod, index int) error {
	if mod.StartCmdLinux != nil && len(*mod.StartCmdLinux) > maxStartCmdLength {
		return errors.Errorf("mods[%d].start_cmd_linux must be at most %d characters", index, maxStartCmdLength)
	}

	if mod.StartCmdWindows != nil && len(*mod.StartCmdWindows) > maxStartCmdLength {
		return errors.Errorf("mods[%d].start_cmd_windows must be at most %d characters", index, maxStartCmdLength)
	}

	commands := map[string]*string{
		"kick_cmd":     mod.KickCmd,
		"ban_cmd":      mod.BanCmd,
		"chname_cmd":   mod.ChnameCmd,
		"srestart_cmd": mod.SrestartCmd,
		"chmap_cmd":    mod.ChmapCmd,
		"sendmsg_cmd":  mod.SendmsgCmd,
		"passwd_cmd":   mod.PasswdCmd,
	}

	for name, cmd := range commands {
		if cmd != nil && len(*cmd) > maxCommandLength {
			return errors.Errorf("mods[%d].%s must be at most %d characters", index, name, maxCommandLength)
		}
	}

	if mod.RemoteRepositoryLinux != nil && len(*mod.RemoteRepositoryLinux) > maxRepositoryLength {
		return errors.Errorf("mods[%d].remote_repository_linux must be at most %d characters", index, maxRepositoryLength)
	}

	if mod.RemoteRepositoryWindows != nil && len(*mod.RemoteRepositoryWindows) > maxRepositoryLength {
		return errors.Errorf("mods[%d].remote_repository_windows must be at most %d characters", index, maxRepositoryLength)
	}

	if mod.LocalRepositoryLinux != nil && len(*mod.LocalRepositoryLinux) > maxRepositoryLength {
		return errors.Errorf("mods[%d].local_repository_linux must be at most %d characters", index, maxRepositoryLength)
	}

	if mod.LocalRepositoryWindows != nil && len(*mod.LocalRepositoryWindows) > maxRepositoryLength {
		return errors.Errorf("mods[%d].local_repository_windows must be at most %d characters", index, maxRepositoryLength)
	}

	return nil
}

func (g *GameExportGame) ToDomainGame() *Game {
	return &Game{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppIDLinux:         g.SteamAppIDLinux,
		SteamAppIDWindows:       g.SteamAppIDWindows,
		SteamAppSetConfig:       g.SteamAppSetConfig,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Enabled:                 1,
		Metadata:                g.Metadata,
	}
}

func (m *GameExportMod) ToDomainGameMod(gameCode string) *GameMod {
	fastRcon := make(GameModFastRconList, 0, len(m.FastRcon))
	for _, fr := range m.FastRcon {
		fr.Normalize()
		fastRcon = append(fastRcon, fr)
	}

	vars := make(GameModVarList, 0, len(m.Vars))
	for _, v := range m.Vars {
		// Clone the rules and the options so the export struct and the domain
		// struct share neither the pointer nor the option backing array:
		// Normalize writes to both.
		v.Rules = v.Rules.Clone()
		v.Options = v.Options.Clone()
		v.Normalize()
		vars = append(vars, v)
	}

	return &GameMod{
		GameCode:                gameCode,
		Name:                    m.Name,
		FastRcon:                fastRcon,
		Vars:                    vars,
		RemoteRepositoryLinux:   m.RemoteRepositoryLinux,
		RemoteRepositoryWindows: m.RemoteRepositoryWindows,
		LocalRepositoryLinux:    m.LocalRepositoryLinux,
		LocalRepositoryWindows:  m.LocalRepositoryWindows,
		StartCmdLinux:           m.StartCmdLinux,
		StartCmdWindows:         m.StartCmdWindows,
		KickCmd:                 m.KickCmd,
		BanCmd:                  m.BanCmd,
		ChnameCmd:               m.ChnameCmd,
		SrestartCmd:             m.SrestartCmd,
		ChmapCmd:                m.ChmapCmd,
		SendmsgCmd:              m.SendmsgCmd,
		PasswdCmd:               m.PasswdCmd,
		Metadata:                m.Metadata,
	}
}

func NewGameExportFromDomain(game *Game, mods []GameMod, version string) *GameExport {
	exportMods := make([]GameExportMod, 0, len(mods))
	for _, mod := range mods {
		exportMods = append(exportMods, newGameExportModFromDomain(&mod))
	}

	exportedAt := time.Now().UTC().Format(time.RFC3339)
	exportedBy := "GameAP"
	if version != "" {
		exportedBy = "GameAP " + version
	}

	return &GameExport{
		SchemaVersion: CurrentSchemaVersion,
		ExportedAt:    exportedAt,
		ExportedBy:    exportedBy,
		Game: GameExportGame{
			Code:                    game.Code,
			Name:                    game.Name,
			Engine:                  game.Engine,
			EngineVersion:           game.EngineVersion,
			SteamAppIDLinux:         game.SteamAppIDLinux,
			SteamAppIDWindows:       game.SteamAppIDWindows,
			SteamAppSetConfig:       game.SteamAppSetConfig,
			RemoteRepositoryLinux:   game.RemoteRepositoryLinux,
			RemoteRepositoryWindows: game.RemoteRepositoryWindows,
			LocalRepositoryLinux:    game.LocalRepositoryLinux,
			LocalRepositoryWindows:  game.LocalRepositoryWindows,
			Metadata:                game.Metadata,
		},
		Mods: exportMods,
	}
}

func newGameExportModFromDomain(mod *GameMod) GameExportMod {
	fastRcon := make([]GameExportModFastRcon, 0, len(mod.FastRcon))
	for _, fr := range mod.FastRcon {
		fr.Normalize()
		fastRcon = append(fastRcon, fr)
	}

	vars := make([]GameExportModVar, 0, len(mod.Vars))
	for _, v := range mod.Vars {
		v.Rules = v.Rules.Clone()
		v.Options = v.Options.Clone()
		v.Normalize()
		vars = append(vars, v)
	}

	return GameExportMod{
		Name:                    mod.Name,
		FastRcon:                lo.Ternary(len(fastRcon) > 0, fastRcon, nil),
		Vars:                    lo.Ternary(len(vars) > 0, vars, nil),
		RemoteRepositoryLinux:   mod.RemoteRepositoryLinux,
		RemoteRepositoryWindows: mod.RemoteRepositoryWindows,
		LocalRepositoryLinux:    mod.LocalRepositoryLinux,
		LocalRepositoryWindows:  mod.LocalRepositoryWindows,
		StartCmdLinux:           mod.StartCmdLinux,
		StartCmdWindows:         mod.StartCmdWindows,
		KickCmd:                 mod.KickCmd,
		BanCmd:                  mod.BanCmd,
		ChnameCmd:               mod.ChnameCmd,
		SrestartCmd:             mod.SrestartCmd,
		ChmapCmd:                mod.ChmapCmd,
		SendmsgCmd:              mod.SendmsgCmd,
		PasswdCmd:               mod.PasswdCmd,
		Metadata:                mod.Metadata,
	}
}
