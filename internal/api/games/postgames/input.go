package postgames

import (
	"fmt"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/samber/lo"
)

const (
	minGameCodeLength      = 2
	maxGameCodeLength      = 16
	minGameNameLength      = 2
	maxGameNameLength      = 128
	maxEngineLength        = 128
	maxEngineVersionLength = 128
	maxConfigLength        = 128
	maxRepositoryLength    = 128
)

var (
	ErrGameCodeIsRequired = api.NewValidationError("game code is required")
	ErrGameCodeTooShort   = api.NewValidationError(
		fmt.Sprintf("game code must be at least %d characters", minGameCodeLength),
	)
	ErrGameCodeTooLong = api.NewValidationError(
		fmt.Sprintf("game code must not exceed %d characters", maxGameCodeLength),
	)
	ErrGameCodeInvalidFormat = api.NewValidationError(
		"game code must be a valid slug " +
			"(lowercase letters, digits, underscores, and hyphens only)",
	)
	ErrGameNameIsRequired = api.NewValidationError("game name is required")
	ErrGameNameTooShort   = api.NewValidationError(
		fmt.Sprintf("game name must be at least %d characters", minGameNameLength),
	)
	ErrGameNameTooLong = api.NewValidationError(
		fmt.Sprintf("game name must not exceed %d characters", maxGameNameLength),
	)
	ErrEngineIsRequired = api.NewValidationError("engine is required")
	ErrEngineTooLong    = api.NewValidationError(
		fmt.Sprintf("engine must not exceed %d characters", maxEngineLength),
	)
	ErrEngineVersionTooLong = api.NewValidationError(
		fmt.Sprintf("engine version must not exceed %d characters", maxEngineVersionLength),
	)
	ErrSteamAppSetConfigTooLong = api.NewValidationError(
		fmt.Sprintf("steam app set config must not exceed %d characters", maxConfigLength),
	)
	ErrRemoteRepositoryTooLong = api.NewValidationError(
		fmt.Sprintf("remote repository must not exceed %d characters", maxRepositoryLength),
	)
	ErrLocalRepositoryTooLong = api.NewValidationError(
		fmt.Sprintf("local repository must not exceed %d characters", maxRepositoryLength),
	)
)

type createGameInput struct {
	Code string `json:"code"` // required, unique, slug, minlen=2, maxlen=16
	Name string `json:"name"` // required, minlen=2, maxlen=128

	Engine                  string         `json:"engine"`                              // required, maxlen=128
	EngineVersion           string         `json:"engine_version,omitempty"`            // maxlen=128
	SteamAppIDLinux         *flexible.Uint `json:"steam_app_id_linux,omitempty"`        //
	SteamAppIDWindows       *flexible.Uint `json:"steam_app_id_windows,omitempty"`      //
	SteamAppSetConfig       *string        `json:"steam_app_set_config,omitempty"`      // maxlen=128
	RemoteRepositoryLinux   *string        `json:"remote_repository_linux,omitempty"`   // maxlen=128
	RemoteRepositoryWindows *string        `json:"remote_repository_windows,omitempty"` // maxlen=128
	LocalRepositoryLinux    *string        `json:"local_repository_linux,omitempty"`    // maxlen=128
	LocalRepositoryWindows  *string        `json:"local_repository_windows,omitempty"`  // maxlen=128
	Enabled                 int            `json:"enabled"`                             //
}

func (g *createGameInput) Validate() error {
	if g.Code == "" {
		return ErrGameCodeIsRequired
	}
	if len(g.Code) < minGameCodeLength {
		return ErrGameCodeTooShort
	}
	if len(g.Code) > maxGameCodeLength {
		return ErrGameCodeTooLong
	}
	if !validation.IsSlug(g.Code) {
		return ErrGameCodeInvalidFormat
	}

	if g.Name == "" {
		return ErrGameNameIsRequired
	}
	if len(g.Name) < minGameNameLength {
		return ErrGameNameTooShort
	}
	if len(g.Name) > maxGameNameLength {
		return ErrGameNameTooLong
	}

	if g.Engine == "" {
		return ErrEngineIsRequired
	}
	if len(g.Engine) > maxEngineLength {
		return ErrEngineTooLong
	}

	if len(g.EngineVersion) > maxEngineVersionLength {
		return ErrEngineVersionTooLong
	}

	if g.SteamAppSetConfig != nil && len(*g.SteamAppSetConfig) > maxConfigLength {
		return ErrSteamAppSetConfigTooLong
	}

	if g.RemoteRepositoryLinux != nil &&
		len(*g.RemoteRepositoryLinux) > maxRepositoryLength {
		return ErrRemoteRepositoryTooLong
	}

	if g.RemoteRepositoryWindows != nil &&
		len(*g.RemoteRepositoryWindows) > maxRepositoryLength {
		return ErrRemoteRepositoryTooLong
	}

	if g.LocalRepositoryLinux != nil &&
		len(*g.LocalRepositoryLinux) > maxRepositoryLength {
		return ErrLocalRepositoryTooLong
	}

	if g.LocalRepositoryWindows != nil &&
		len(*g.LocalRepositoryWindows) > maxRepositoryLength {
		return ErrLocalRepositoryTooLong
	}

	return nil
}

func (g *createGameInput) ToDomain() *domain.Game {
	return &domain.Game{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppIDLinux:         lo.ToPtr(g.SteamAppIDLinux.Uint()),
		SteamAppIDWindows:       lo.ToPtr(g.SteamAppIDWindows.Uint()),
		SteamAppSetConfig:       g.SteamAppSetConfig,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Enabled:                 g.Enabled,
	}
}
