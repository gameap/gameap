package base

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/pkg/errors"
)

// VarInput is the wire shape of a game mod variable definition. Options accept
// both forms allowed by the catalog schema: a plain string and a
// {value, label, i18n} object.
type VarInput struct {
	Var         string                   `json:"var"`
	Default     domain.GameModVarDefault `json:"default"`
	Info        string                   `json:"info"`
	AdminVar    flexible.Bool            `json:"admin_var,omitempty"`
	Type        domain.GameModVarType    `json:"type,omitempty"`
	Description string                   `json:"description,omitempty"`
	Options     domain.GameModVarOptions `json:"options,omitempty"`
	AllowCustom flexible.Bool            `json:"allow_custom,omitempty"`
	TrueValue   *string                  `json:"true_value,omitempty"`
	FalseValue  *string                  `json:"false_value,omitempty"`
	Rules       *domain.GameModVarRules  `json:"rules,omitempty"`
	I18n        domain.GameModVarI18n    `json:"i18n,omitempty"`
}

func (v *VarInput) ToDomain() domain.GameModVar {
	gameModVar := domain.GameModVar{
		Var:         v.Var,
		Default:     v.Default,
		Info:        v.Info,
		AdminVar:    v.AdminVar.Bool(),
		Type:        v.Type,
		Description: v.Description,
		Options:     v.Options,
		AllowCustom: v.AllowCustom.Bool(),
		TrueValue:   v.TrueValue,
		FalseValue:  v.FalseValue,
		Rules:       v.Rules.Clone(),
		I18n:        v.I18n,
	}
	gameModVar.Normalize()

	return gameModVar
}

func (v *VarInput) Validate() error {
	gameModVar := v.ToDomain()
	if err := gameModVar.Validate(); err != nil {
		return api.NewValidationError(err.Error())
	}

	return nil
}

type FastRconInput struct {
	Info    string                     `json:"info"`
	Command string                     `json:"command"`
	I18n    domain.GameModFastRconI18n `json:"i18n,omitempty"`
}

func (f *FastRconInput) ToDomain() domain.GameModFastRcon {
	fastRcon := domain.GameModFastRcon{
		Info:    f.Info,
		Command: f.Command,
		I18n:    f.I18n,
	}
	fastRcon.Normalize()

	return fastRcon
}

func (f *FastRconInput) Validate() error {
	fastRcon := f.ToDomain()
	if err := fastRcon.Validate(); err != nil {
		return api.NewValidationError(err.Error())
	}

	return nil
}

// ValidateVarInputs checks every definition and then the list as a whole, so
// duplicate variable names are rejected too.
func ValidateVarInputs(inputs []VarInput) error {
	for i := range inputs {
		if err := inputs[i].Validate(); err != nil {
			return errors.WithMessagef(err, "game mod input Vars[%d]", i)
		}
	}

	if err := VarInputsToDomain(inputs).Validate(); err != nil {
		return api.NewValidationError(err.Error())
	}

	return nil
}

func ValidateFastRconInputs(inputs []FastRconInput) error {
	for i := range inputs {
		if err := inputs[i].Validate(); err != nil {
			return errors.WithMessagef(err, "game mod input FastRcon[%d]", i)
		}
	}

	return nil
}

func VarInputsToDomain(inputs []VarInput) domain.GameModVarList {
	list := make(domain.GameModVarList, 0, len(inputs))
	for i := range inputs {
		list = append(list, inputs[i].ToDomain())
	}

	return list
}

func FastRconInputsToDomain(inputs []FastRconInput) domain.GameModFastRconList {
	list := make(domain.GameModFastRconList, 0, len(inputs))
	for i := range inputs {
		list = append(list, inputs[i].ToDomain())
	}

	return list
}
