package getserversettings

import (
	"github.com/gameap/gameap/internal/domain"
)

// SettingResponse merges a game mod variable definition with the value stored
// for this server. Value and Default are typed after Type: a boolean for bool,
// a number for int and float, a string for everything else.
type SettingResponse struct {
	Name        string                  `json:"name"`
	Value       any                     `json:"value"`
	Default     any                     `json:"default"`
	Type        string                  `json:"type"`
	Label       string                  `json:"label"`
	Description string                  `json:"description,omitempty"`
	Options     []OptionResponse        `json:"options,omitempty"`
	AllowCustom bool                    `json:"allow_custom,omitempty"`
	Rules       *domain.GameModVarRules `json:"rules,omitempty"`
	I18n        domain.GameModVarI18n   `json:"i18n,omitempty"`
	AdminVar    bool                    `json:"admin_var,omitempty"`
}

// OptionResponse is always the object form so the client has a single shape to
// render, even when the stored definition used the plain-string shorthand.
type OptionResponse struct {
	Value string                      `json:"value"`
	Label string                      `json:"label"`
	I18n  domain.GameModVarOptionI18n `json:"i18n,omitempty"`
}

func newSettingResponseFromVar(gmVar *domain.GameModVar) SettingResponse {
	defaultValue := gmVar.FormatValue(string(gmVar.Default))

	return SettingResponse{
		Name:        gmVar.Var,
		Value:       defaultValue,
		Default:     defaultValue,
		Type:        string(gmVar.EffectiveType()),
		Label:       gmVar.Info,
		Description: gmVar.Description,
		Options:     newOptionsResponse(gmVar.Options),
		AllowCustom: gmVar.AllowCustom,
		Rules:       gmVar.Rules,
		I18n:        gmVar.I18n,
		AdminVar:    gmVar.AdminVar,
	}
}

func newOptionsResponse(options domain.GameModVarOptions) []OptionResponse {
	if len(options) == 0 {
		return nil
	}

	response := make([]OptionResponse, 0, len(options))
	for _, option := range options {
		response = append(response, OptionResponse{
			Value: option.Value,
			Label: option.LabelOrValue(),
			I18n:  option.I18n,
		})
	}

	return response
}
