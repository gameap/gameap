package postgamemod

import (
	"fmt"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
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

type gameModInput struct {
	GameCode                string          `json:"game_code"`
	Name                    string          `json:"name"`
	FastRcon                []fastRconInput `json:"fast_rcon,omitempty"`
	Vars                    []varInput      `json:"vars,omitempty"`
	RemoteRepositoryLinux   *string         `json:"remote_repository_linux,omitempty"`
	RemoteRepositoryWindows *string         `json:"remote_repository_windows,omitempty"`
	LocalRepositoryLinux    *string         `json:"local_repository_linux,omitempty"`
	LocalRepositoryWindows  *string         `json:"local_repository_windows,omitempty"`
	StartCmdLinux           *string         `json:"start_cmd_linux,omitempty"`
	StartCmdWindows         *string         `json:"start_cmd_windows,omitempty"`
	KickCmd                 *string         `json:"kick_cmd,omitempty"`
	BanCmd                  *string         `json:"ban_cmd,omitempty"`
	ChnameCmd               *string         `json:"chname_cmd,omitempty"`
	SrestartCmd             *string         `json:"srestart_cmd,omitempty"`
	ChmapCmd                *string         `json:"chmap_cmd,omitempty"`
	SendmsgCmd              *string         `json:"sendmsg_cmd,omitempty"`
	PasswdCmd               *string         `json:"passwd_cmd,omitempty"`
}

func (g *gameModInput) Validate() error {
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

	for i := range g.FastRcon {
		if err := g.FastRcon[i].Validate(); err != nil {
			return errors.WithMessagef(err, "game mod input FastRcon[%d]", i)
		}
	}

	for i := range g.Vars {
		if err := g.Vars[i].Validate(); err != nil {
			return errors.WithMessagef(err, "game mod input Vars[%d]", i)
		}
	}

	return nil
}

func (g *gameModInput) ToDomain() *domain.GameMod {
	fastRconList := make(domain.GameModFastRconList, 0, len(g.FastRcon))
	for _, fr := range g.FastRcon {
		fastRconList = append(fastRconList, fr.ToDomain())
	}

	varList := make(domain.GameModVarList, 0, len(g.Vars))
	for _, v := range g.Vars {
		varList = append(varList, v.ToDomain())
	}

	return &domain.GameMod{
		GameCode:                g.GameCode,
		Name:                    g.Name,
		FastRcon:                fastRconList,
		Vars:                    varList,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		StartCmdLinux:           g.StartCmdLinux,
		StartCmdWindows:         g.StartCmdWindows,
		KickCmd:                 g.KickCmd,
		BanCmd:                  g.BanCmd,
		ChnameCmd:               g.ChnameCmd,
		SrestartCmd:             g.SrestartCmd,
		ChmapCmd:                g.ChmapCmd,
		SendmsgCmd:              g.SendmsgCmd,
		PasswdCmd:               g.PasswdCmd,
	}
}

type fastRconInput struct {
	Info    string `json:"info"`
	Command string `json:"command"`
}

func (f *fastRconInput) Validate() error {
	if f.Info == "" {
		return api.NewValidationError("fast rcon info is required")
	}

	if f.Command == "" {
		return api.NewValidationError("fast rcon command is required")
	}

	return nil
}

func (f *fastRconInput) ToDomain() domain.GameModFastRcon {
	return domain.GameModFastRcon{
		Info:    f.Info,
		Command: f.Command,
	}
}

type varInput struct {
	Var      string `json:"var"`
	Default  string `json:"default"`
	Info     string `json:"info"`
	AdminVar bool   `json:"admin_var,omitempty"`
}

func (v *varInput) Validate() error {
	if v.Var == "" {
		return api.NewValidationError("var is required")
	}

	if v.Info == "" {
		return api.NewValidationError("info is required")
	}

	return nil
}

func (v *varInput) ToDomain() domain.GameModVar {
	return domain.GameModVar{
		Var:      v.Var,
		Default:  domain.GameModVarDefault(v.Default),
		Info:     v.Info,
		AdminVar: v.AdminVar,
	}
}
