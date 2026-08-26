package base

import (
	"github.com/gameap/gameap/internal/domain"
)

type GameModResponse struct {
	ID                      uint              `json:"id"`
	GameCode                string            `json:"game_code"`
	Name                    string            `json:"name"`
	FastRcon                []gameModFastRcon `json:"fast_rcon"`
	Vars                    []gameModVar      `json:"vars"`
	RemoteRepositoryLinux   *string           `json:"remote_repository_linux"`
	RemoteRepositoryWindows *string           `json:"remote_repository_windows"`
	LocalRepositoryLinux    *string           `json:"local_repository_linux"`
	LocalRepositoryWindows  *string           `json:"local_repository_windows"`
	StartCmdLinux           *string           `json:"start_cmd_linux"`
	StartCmdWindows         *string           `json:"start_cmd_windows"`
	KickCmd                 *string           `json:"kick_cmd"`
	BanCmd                  *string           `json:"ban_cmd"`
	ChnameCmd               *string           `json:"chname_cmd"`
	SrestartCmd             *string           `json:"srestart_cmd"`
	ChmapCmd                *string           `json:"chmap_cmd"`
	SendmsgCmd              *string           `json:"sendmsg_cmd"`
	PasswdCmd               *string           `json:"passwd_cmd"`
	Metadata                map[string]any    `json:"metadata"`
}

type gameModFastRcon struct {
	Info    string                     `json:"info"`
	Command string                     `json:"command"`
	I18n    domain.GameModFastRconI18n `json:"i18n,omitempty"`
}

type gameModVar struct {
	Var         string                  `json:"var"`
	Default     string                  `json:"default"`
	Info        string                  `json:"info"`
	AdminVar    bool                    `json:"admin_var"`
	Type        domain.GameModVarType   `json:"type,omitempty"`
	Description string                  `json:"description,omitempty"`
	Options     []gameModVarOption      `json:"options,omitempty"`
	AllowCustom bool                    `json:"allow_custom,omitempty"`
	TrueValue   *string                 `json:"true_value,omitempty"`
	FalseValue  *string                 `json:"false_value,omitempty"`
	Rules       *domain.GameModVarRules `json:"rules,omitempty"`
	I18n        domain.GameModVarI18n   `json:"i18n,omitempty"`
}

// gameModVarOption is always the object form, even when the stored definition
// uses the plain-string shorthand: the admin editor then has a single shape to
// bind to. The shorthand is restored on save.
type gameModVarOption struct {
	Value string                      `json:"value"`
	Label string                      `json:"label"`
	I18n  domain.GameModVarOptionI18n `json:"i18n,omitempty"`
}

func NewGameModsResponseFromGameMods(gameMods []domain.GameMod) []GameModResponse {
	response := make([]GameModResponse, 0, len(gameMods))

	for _, gm := range gameMods {
		response = append(response, NewGameModResponseFromGameMod(&gm))
	}

	return response
}

func NewGameModResponseFromGameMod(gm *domain.GameMod) GameModResponse {
	return GameModResponse{
		ID:                      gm.ID,
		GameCode:                gm.GameCode,
		Name:                    gm.Name,
		FastRcon:                gameModFastRconFromDomain(gm.FastRcon),
		Vars:                    gameModVarsFromDomain(gm.Vars),
		RemoteRepositoryLinux:   gm.RemoteRepositoryLinux,
		RemoteRepositoryWindows: gm.RemoteRepositoryWindows,
		LocalRepositoryLinux:    gm.LocalRepositoryLinux,
		LocalRepositoryWindows:  gm.LocalRepositoryWindows,
		StartCmdLinux:           gm.StartCmdLinux,
		StartCmdWindows:         gm.StartCmdWindows,
		KickCmd:                 gm.KickCmd,
		BanCmd:                  gm.BanCmd,
		ChnameCmd:               gm.ChnameCmd,
		SrestartCmd:             gm.SrestartCmd,
		ChmapCmd:                gm.ChmapCmd,
		SendmsgCmd:              gm.SendmsgCmd,
		PasswdCmd:               gm.PasswdCmd,
		Metadata:                gm.Metadata,
	}
}

func gameModFastRconFromDomain(fastRcon domain.GameModFastRconList) []gameModFastRcon {
	result := make([]gameModFastRcon, 0, len(fastRcon))

	for _, fr := range fastRcon {
		result = append(result, gameModFastRcon{
			Info:    fr.Info,
			Command: fr.Command,
			I18n:    fr.I18n,
		})
	}

	return result
}

func gameModVarsFromDomain(vars []domain.GameModVar) []gameModVar {
	result := make([]gameModVar, 0, len(vars))

	for _, v := range vars {
		result = append(result, gameModVar{
			Var:         v.Var,
			Default:     string(v.Default),
			Info:        v.Info,
			AdminVar:    v.AdminVar,
			Type:        v.Type,
			Description: v.Description,
			Options:     gameModVarOptionsFromDomain(v.Options),
			AllowCustom: v.AllowCustom,
			TrueValue:   v.TrueValue,
			FalseValue:  v.FalseValue,
			Rules:       v.Rules,
			I18n:        v.I18n,
		})
	}

	return result
}

func gameModVarOptionsFromDomain(options domain.GameModVarOptions) []gameModVarOption {
	if len(options) == 0 {
		return nil
	}

	result := make([]gameModVarOption, 0, len(options))

	for _, o := range options {
		result = append(result, gameModVarOption{
			Value: o.Value,
			Label: o.LabelOrValue(),
			I18n:  o.I18n,
		})
	}

	return result
}
