package domain

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// GameModVarType is the widget type in the panel and the base value format.
// An empty type means string: that is what games.json emits for untyped
// variables, and what every pre-schema game mod carries.
type GameModVarType string

const (
	GameModVarTypeString   GameModVarType = "string"
	GameModVarTypeText     GameModVarType = "text"
	GameModVarTypeInt      GameModVarType = "int"
	GameModVarTypeFloat    GameModVarType = "float"
	GameModVarTypeBool     GameModVarType = "bool"
	GameModVarTypeSelect   GameModVarType = "select"
	GameModVarTypePassword GameModVarType = "password"
)

const (
	defaultVarTrueValue  = "1"
	defaultVarFalseValue = "0"
)

func (t GameModVarType) Normalized() GameModVarType {
	if t == "" {
		return GameModVarTypeString
	}

	return t
}

func (t GameModVarType) IsKnown() bool {
	switch t {
	case "",
		GameModVarTypeString,
		GameModVarTypeText,
		GameModVarTypeInt,
		GameModVarTypeFloat,
		GameModVarTypeBool,
		GameModVarTypeSelect,
		GameModVarTypePassword:
		return true
	}

	return false
}

// IsNumeric reports whether the min/max rules apply to this type.
func (t GameModVarType) IsNumeric() bool {
	switch t.Normalized() {
	case GameModVarTypeInt, GameModVarTypeFloat:
		return true
	}

	return false
}

// IsTextual reports whether the min_length/max_length/pattern rules apply to
// this type. A select accepts free text only when allow_custom is set, so it is
// handled by GameModVar.acceptsFreeText instead.
func (t GameModVarType) IsTextual() bool {
	switch t.Normalized() {
	case GameModVarTypeString, GameModVarTypeText, GameModVarTypePassword:
		return true
	}

	return false
}

// GameModVar describes one template variable of a game mod. It is referenced as
// {var} in command templates and exported as an environment variable on server
// start. The first four fields predate the games.schema.json format and keep
// their JSON tags so a stored variable serializes exactly as before.
type GameModVar struct {
	Var         string            `json:"var"                    yaml:"var"`
	Default     GameModVarDefault `json:"default"                yaml:"default"`
	Info        string            `json:"info"                   yaml:"info"`
	AdminVar    bool              `json:"admin_var"              yaml:"admin_var,omitempty"`
	Type        GameModVarType    `json:"type,omitempty"         yaml:"type,omitempty"`
	Description string            `json:"description,omitempty"  yaml:"description,omitempty"`
	Options     GameModVarOptions `json:"options,omitempty"      yaml:"options,omitempty"`
	AllowCustom bool              `json:"allow_custom,omitempty" yaml:"allow_custom,omitempty"`
	TrueValue   *string           `json:"true_value,omitempty"   yaml:"true_value,omitempty"`
	FalseValue  *string           `json:"false_value,omitempty"  yaml:"false_value,omitempty"`
	Rules       *GameModVarRules  `json:"rules,omitempty"        yaml:"rules,omitempty"`
	I18n        GameModVarI18n    `json:"i18n,omitempty"         yaml:"i18n,omitempty"`
}

func (v *GameModVar) EffectiveType() GameModVarType {
	return v.Type.Normalized()
}

func (v *GameModVar) TrueValueOrDefault() string {
	if v.TrueValue != nil {
		return *v.TrueValue
	}

	return defaultVarTrueValue
}

func (v *GameModVar) FalseValueOrDefault() string {
	if v.FalseValue != nil {
		return *v.FalseValue
	}

	return defaultVarFalseValue
}

// acceptsFreeText reports whether the value may be any string, and therefore
// whether the length and pattern rules apply.
func (v *GameModVar) acceptsFreeText() bool {
	if v.EffectiveType() == GameModVarTypeSelect {
		return v.AllowCustom
	}

	return v.EffectiveType().IsTextual()
}

// Normalize drops everything that does not apply to the resolved type and
// empties out zero-value containers, so a variable that went through an editor
// or an importer round-trips without gaining meaningless keys.
func (v *GameModVar) Normalize() {
	if v.EffectiveType() != GameModVarTypeSelect {
		v.Options = nil
		v.AllowCustom = false
	}

	if v.EffectiveType() != GameModVarTypeBool {
		v.TrueValue = nil
		v.FalseValue = nil
	}

	if len(v.Options) == 0 {
		v.Options = nil
	}

	for i := range v.Options {
		v.Options[i].Normalize()
	}

	if v.Rules != nil {
		v.Rules.Normalize(v)
		if v.Rules.IsEmpty() {
			v.Rules = nil
		}
	}

	v.I18n = v.I18n.Normalized()
}

// GameModVarOption is one allowed value of a select variable. An option that
// carries neither a label nor translations marshals back to a bare string, the
// shorthand form used throughout games.yaml.
type GameModVarOption struct {
	Value string               `json:"value" yaml:"value"`
	Label string               `json:"label,omitempty" yaml:"label,omitempty"`
	I18n  GameModVarOptionI18n `json:"i18n,omitempty"  yaml:"i18n,omitempty"`
}

// gameModVarOption breaks the marshaler recursion.
type gameModVarOption GameModVarOption

func (o GameModVarOption) IsShorthand() bool {
	return o.Label == "" && len(o.I18n) == 0
}

func (o GameModVarOption) LabelOrValue() string {
	if o.Label != "" {
		return o.Label
	}

	return o.Value
}

func (o *GameModVarOption) Normalize() {
	if o.Label == o.Value {
		o.Label = ""
	}

	o.I18n = o.I18n.Normalized()
}

func (o GameModVarOption) MarshalJSON() ([]byte, error) {
	if o.IsShorthand() {
		return json.Marshal(o.Value)
	}

	return json.Marshal(gameModVarOption(o))
}

func (o *GameModVarOption) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*o = GameModVarOption{Value: str}

		return nil
	}

	var obj gameModVarOption
	if err := json.Unmarshal(data, &obj); err != nil {
		return errors.Wrap(err, "game mod var option must be a string or an object")
	}

	*o = GameModVarOption(obj)

	return nil
}

func (o GameModVarOption) MarshalYAML() (any, error) {
	if o.IsShorthand() {
		return o.Value, nil
	}

	return gameModVarOption(o), nil
}

func (o *GameModVarOption) UnmarshalYAML(unmarshal func(any) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		*o = GameModVarOption{Value: str}

		return nil
	}

	var obj gameModVarOption
	if err := unmarshal(&obj); err != nil {
		return errors.Wrap(err, "game mod var option must be a string or a mapping")
	}

	*o = GameModVarOption(obj)

	return nil
}

type GameModVarOptions []GameModVarOption

func (l GameModVarOptions) Contains(value string) bool {
	for _, option := range l {
		if option.Value == value {
			return true
		}
	}

	return false
}

// Clone copies the option list. Normalize rewrites the elements in place, so a
// copy that still shares the backing array would rewrite the source too.
func (l GameModVarOptions) Clone() GameModVarOptions {
	if l == nil {
		return nil
	}

	return slices.Clone(l)
}

func (l GameModVarOptions) LabelFor(value string) string {
	for _, option := range l {
		if option.Value == value {
			return option.LabelOrValue()
		}
	}

	return value
}

// GameModVarRules are the validation rules the panel applies when a server
// setting is saved. Every field is a pointer because required=false, min=0 and
// min_length=0 are legal values that differ from "not set"; pattern is a plain
// string because the schema gives it minLength 1.
type GameModVarRules struct {
	Required  *bool    `json:"required,omitempty"   yaml:"required,omitempty"`
	Min       *float64 `json:"min,omitempty"        yaml:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"        yaml:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Pattern   string   `json:"pattern,omitempty"    yaml:"pattern,omitempty"`
}

func (r *GameModVarRules) IsEmpty() bool {
	if r == nil {
		return true
	}

	return r.Required == nil &&
		r.Min == nil &&
		r.Max == nil &&
		r.MinLength == nil &&
		r.MaxLength == nil &&
		r.Pattern == ""
}

func (r *GameModVarRules) IsRequired() bool {
	return r != nil && r.Required != nil && *r.Required
}

func (r *GameModVarRules) Clone() *GameModVarRules {
	if r == nil {
		return nil
	}

	clone := *r
	clone.Required = clonePtr(r.Required)
	clone.Min = clonePtr(r.Min)
	clone.Max = clonePtr(r.Max)
	clone.MinLength = clonePtr(r.MinLength)
	clone.MaxLength = clonePtr(r.MaxLength)

	return &clone
}

// Normalize drops rules that do not apply to the variable type, so switching a
// variable from int to string in the admin editor cannot leave a stale bound
// behind.
func (r *GameModVarRules) Normalize(v *GameModVar) {
	if r == nil {
		return
	}

	if r.Required != nil && !*r.Required {
		r.Required = nil
	}

	if !v.EffectiveType().IsNumeric() {
		r.Min = nil
		r.Max = nil
	}

	if !v.acceptsFreeText() {
		r.MinLength = nil
		r.MaxLength = nil
		r.Pattern = ""
	}
}

func clonePtr[T any](v *T) *T {
	if v == nil {
		return nil
	}

	return new(*v)
}

type GameModVarTranslation struct {
	Info        string `json:"info,omitempty"        yaml:"info,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (t GameModVarTranslation) IsEmpty() bool {
	return t.Info == "" && t.Description == ""
}

// GameModVarI18n holds translations keyed by lowercase BCP-47 locale. English
// is the base language and lives in the Info/Description fields, so "en" never
// appears here.
type GameModVarI18n map[string]GameModVarTranslation

func (m GameModVarI18n) Normalized() GameModVarI18n {
	if len(m) == 0 {
		return nil
	}

	normalized := make(GameModVarI18n, len(m))
	for locale, translation := range m {
		key := normalizeLocaleKey(locale)
		if key == "" || key == baseLocale || translation.IsEmpty() {
			continue
		}

		normalized[key] = translation
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

type GameModVarOptionTranslation struct {
	Label string `json:"label" yaml:"label"`
}

type GameModVarOptionI18n map[string]GameModVarOptionTranslation

func (m GameModVarOptionI18n) Normalized() GameModVarOptionI18n {
	if len(m) == 0 {
		return nil
	}

	normalized := make(GameModVarOptionI18n, len(m))
	for locale, translation := range m {
		key := normalizeLocaleKey(locale)
		if key == "" || key == baseLocale || translation.Label == "" {
			continue
		}

		normalized[key] = translation
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

type GameModFastRconTranslation struct {
	Info string `json:"info" yaml:"info"`
}

type GameModFastRconI18n map[string]GameModFastRconTranslation

func (m GameModFastRconI18n) Normalized() GameModFastRconI18n {
	if len(m) == 0 {
		return nil
	}

	normalized := make(GameModFastRconI18n, len(m))
	for locale, translation := range m {
		key := normalizeLocaleKey(locale)
		if key == "" || key == baseLocale || translation.Info == "" {
			continue
		}

		normalized[key] = translation
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

const baseLocale = "en"

func normalizeLocaleKey(locale string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "_", "-")
}

// GameModVarDefault is the default value of a variable. The schema types it as a
// string or null, but hand-written YAML and older catalogs also carry bare
// numbers and booleans, so those are accepted and rendered as their literal text.
type GameModVarDefault string

func (gmvd GameModVarDefault) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(gmvd))
}

func (gmvd *GameModVarDefault) UnmarshalJSON(data []byte) error {
	// UseNumber keeps a large integer exact: decoding into any would route it
	// through float64 and lose the last digits.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return errors.Wrap(err, "failed to unmarshal game mod var default")
	}

	value, err := varDefaultFromAny(raw)
	if err != nil {
		return err
	}

	*gmvd = value

	return nil
}

func (gmvd GameModVarDefault) MarshalYAML() (any, error) {
	return string(gmvd), nil
}

func (gmvd *GameModVarDefault) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return errors.Wrap(err, "failed to unmarshal game mod var default")
	}

	value, err := varDefaultFromAny(raw)
	if err != nil {
		return err
	}

	*gmvd = value

	return nil
}

func varDefaultFromAny(raw any) (GameModVarDefault, error) {
	switch v := raw.(type) {
	case nil:
		return "", nil
	case string:
		return GameModVarDefault(v), nil
	case bool:
		return GameModVarDefault(strconv.FormatBool(v)), nil
	case int:
		return GameModVarDefault(strconv.Itoa(v)), nil
	case int64:
		return GameModVarDefault(strconv.FormatInt(v, 10)), nil
	case uint64:
		return GameModVarDefault(strconv.FormatUint(v, 10)), nil
	case float64:
		return GameModVarDefault(strconv.FormatFloat(v, 'f', -1, 64)), nil
	case json.Number:
		return GameModVarDefault(v.String()), nil
	default:
		return "", errors.Errorf("unsupported game mod var default of type %T", raw)
	}
}
