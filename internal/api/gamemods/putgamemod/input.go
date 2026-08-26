package putgamemod

import (
	"fmt"

	"github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
)

const (
	maxNameLength           = 255
	maxGameCodeLength       = 255
	maxShellCmdLength       = 1000
	maxGameConsoleCmdLength = 200
)

var (
	ErrGameModNameIsRequired = api.NewValidationError("game mod name is required")
	ErrGameCodeIsRequired    = api.NewValidationError("game code is required")
	ErrGameModNameTooLong    = api.NewValidationError("game mod name must not exceed 255 characters")
	ErrGameCodeTooLong       = api.NewValidationError("game code must not exceed 255 characters")
	ErrStartCmdLinuxTooLong  = api.NewValidationError(
		fmt.Sprintf("start command linux must not exceed %d characters", maxShellCmdLength),
	)
	ErrStartCmdWindowsTooLong = api.NewValidationError(
		fmt.Sprintf("start command windows must not exceed %d characters", maxShellCmdLength),
	)
	ErrKickCmdTooLong = api.NewValidationError(
		fmt.Sprintf("kick command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrBanCmdTooLong = api.NewValidationError(
		fmt.Sprintf("ban command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrChnameCmdTooLong = api.NewValidationError(
		fmt.Sprintf("chname command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrSrestartCmdTooLong = api.NewValidationError(
		fmt.Sprintf("srestart command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrChmapCmdTooLong = api.NewValidationError(
		fmt.Sprintf("chmap command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrSendmsgCmdTooLong = api.NewValidationError(
		fmt.Sprintf("sendmsg command must not exceed %d characters", maxGameConsoleCmdLength),
	)
	ErrPasswdCmdTooLong = api.NewValidationError(
		fmt.Sprintf("passwd command must not exceed %d characters", maxGameConsoleCmdLength),
	)
)

type updateGameModInput struct {
	GameCode                string               `json:"game_code"`
	Name                    string               `json:"name"`
	FastRcon                []base.FastRconInput `json:"fast_rcon,omitempty"`
	Vars                    []base.VarInput      `json:"vars,omitempty"`
	RemoteRepositoryLinux   *string              `json:"remote_repository_linux,omitempty"`
	RemoteRepositoryWindows *string              `json:"remote_repository_windows,omitempty"`
	LocalRepositoryLinux    *string              `json:"local_repository_linux,omitempty"`
	LocalRepositoryWindows  *string              `json:"local_repository_windows,omitempty"`
	StartCmdLinux           *string              `json:"start_cmd_linux,omitempty"`
	StartCmdWindows         *string              `json:"start_cmd_windows,omitempty"`
	KickCmd                 *string              `json:"kick_cmd,omitempty"`
	BanCmd                  *string              `json:"ban_cmd,omitempty"`
	ChnameCmd               *string              `json:"chname_cmd,omitempty"`
	SrestartCmd             *string              `json:"srestart_cmd,omitempty"`
	ChmapCmd                *string              `json:"chmap_cmd,omitempty"`
	SendmsgCmd              *string              `json:"sendmsg_cmd,omitempty"`
	PasswdCmd               *string              `json:"passwd_cmd,omitempty"`
	Metadata                domain.Metadata      `json:"metadata,omitempty"`
}

func (g *updateGameModInput) Validate() error {
	if g.Name == "" {
		return ErrGameModNameIsRequired
	}

	if g.GameCode == "" {
		return ErrGameCodeIsRequired
	}

	if len(g.Name) > maxNameLength {
		return ErrGameModNameTooLong
	}

	if len(g.GameCode) > maxGameCodeLength {
		return ErrGameCodeTooLong
	}

	if g.StartCmdLinux != nil && len(*g.StartCmdLinux) > maxShellCmdLength {
		return ErrStartCmdLinuxTooLong
	}

	if g.StartCmdWindows != nil && len(*g.StartCmdWindows) > maxShellCmdLength {
		return ErrStartCmdWindowsTooLong
	}

	if g.KickCmd != nil && len(*g.KickCmd) > maxGameConsoleCmdLength {
		return ErrKickCmdTooLong
	}

	if g.BanCmd != nil && len(*g.BanCmd) > maxGameConsoleCmdLength {
		return ErrBanCmdTooLong
	}

	if g.ChnameCmd != nil && len(*g.ChnameCmd) > maxGameConsoleCmdLength {
		return ErrChnameCmdTooLong
	}

	if g.SrestartCmd != nil && len(*g.SrestartCmd) > maxGameConsoleCmdLength {
		return ErrSrestartCmdTooLong
	}

	if g.ChmapCmd != nil && len(*g.ChmapCmd) > maxGameConsoleCmdLength {
		return ErrChmapCmdTooLong
	}

	if g.SendmsgCmd != nil && len(*g.SendmsgCmd) > maxGameConsoleCmdLength {
		return ErrSendmsgCmdTooLong
	}

	if g.PasswdCmd != nil && len(*g.PasswdCmd) > maxGameConsoleCmdLength {
		return ErrPasswdCmdTooLong
	}

	if err := base.ValidateFastRconInputs(g.FastRcon); err != nil {
		return err
	}

	return base.ValidateVarInputs(g.Vars)
}

func (g *updateGameModInput) Apply(gameMod *domain.GameMod) {
	gameMod.GameCode = g.GameCode
	gameMod.Name = g.Name
	gameMod.RemoteRepositoryLinux = g.RemoteRepositoryLinux
	gameMod.RemoteRepositoryWindows = g.RemoteRepositoryWindows
	gameMod.LocalRepositoryLinux = g.LocalRepositoryLinux
	gameMod.LocalRepositoryWindows = g.LocalRepositoryWindows
	gameMod.StartCmdLinux = g.StartCmdLinux
	gameMod.StartCmdWindows = g.StartCmdWindows
	gameMod.KickCmd = g.KickCmd
	gameMod.BanCmd = g.BanCmd
	gameMod.ChnameCmd = g.ChnameCmd
	gameMod.SrestartCmd = g.SrestartCmd
	gameMod.ChmapCmd = g.ChmapCmd
	gameMod.SendmsgCmd = g.SendmsgCmd
	gameMod.PasswdCmd = g.PasswdCmd

	gameMod.FastRcon = base.FastRconInputsToDomain(g.FastRcon)
	gameMod.Vars = base.VarInputsToDomain(g.Vars)
	gameMod.Metadata = g.Metadata
}
