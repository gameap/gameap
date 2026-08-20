package domain

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pkg/errors"
)

const (
	// maxVarNameLength matches servers_settings.name VARCHAR(32): a longer name
	// simply cannot be stored as a per-server value.
	maxVarNameLength = 32

	maxVarInfoLength        = 128
	maxVarDescriptionLength = 1000
	maxVarDefaultLength     = 64
	maxVarOptionValueLength = 64
	maxVarOptionLabelLength = 128
	maxVarBoolValueLength   = 64
	maxFastRconInfoLength   = 128

	// maxServerSettingValueLength bounds a value that carries no max_length rule
	// so a single setting cannot grow without limit.
	maxServerSettingValueLength = 4096
)

// varNameRe is a deliberate superset of the catalog schema's ^[a-z][a-z0-9_]*$:
// the panel also hosts imported Pelican eggs, whose variables keep their
// original uppercase environment variable names.
var varNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// localeRe cannot express the schema's (?!en$) lookahead — RE2 has no
// lookarounds — so the "en is not allowed" half is checked separately.
var localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]+)*$`)

var patternCache sync.Map

// compileVarPattern compiles a rules.pattern anchored so the whole value must
// match, mirroring the schema wording and the frontend's ^(?:…)$ anchoring.
func compileVarPattern(pattern string) (*regexp.Regexp, error) {
	if cached, ok := patternCache.Load(pattern); ok {
		compiled, isRe := cached.(*regexp.Regexp)
		if !isRe {
			return nil, errors.Errorf("invalid regular expression: %s", pattern)
		}

		return compiled, nil
	}

	compiled, err := regexp.Compile(`^(?:` + pattern + `)$`)
	if err != nil {
		patternCache.Store(pattern, struct{}{})

		return nil, errors.Wrapf(err, "invalid regular expression: %s", pattern)
	}

	patternCache.Store(pattern, compiled)

	return compiled, nil
}

func (l GameModVarList) Validate() error {
	seen := make(map[string]struct{}, len(l))

	for i := range l {
		if err := l[i].Validate(); err != nil {
			return errors.WithMessagef(err, "vars[%d]", i)
		}

		if _, exists := seen[l[i].Var]; exists {
			return errors.Errorf("vars[%d]: duplicate variable name: %s", i, l[i].Var)
		}
		seen[l[i].Var] = struct{}{}
	}

	return nil
}

func (v *GameModVar) Validate() error {
	if err := v.validateBase(); err != nil {
		return err
	}

	if err := v.validateOptions(); err != nil {
		return err
	}

	if err := v.validateBoolValues(); err != nil {
		return err
	}

	if err := v.Rules.validate(v); err != nil {
		return err
	}

	return v.I18n.validate()
}

func (v *GameModVar) validateBase() error {
	if v.Var == "" {
		return errors.New("variable name is required")
	}

	if utf8.RuneCountInString(v.Var) > maxVarNameLength {
		return errors.Errorf("variable name must be at most %d characters", maxVarNameLength)
	}

	if !varNameRe.MatchString(v.Var) {
		return errors.New("variable name must match pattern: ^[A-Za-z_][A-Za-z0-9_]*$")
	}

	if v.Info == "" {
		return errors.New("variable info is required")
	}

	if utf8.RuneCountInString(v.Info) > maxVarInfoLength {
		return errors.Errorf("variable info must be at most %d characters", maxVarInfoLength)
	}

	if utf8.RuneCountInString(v.Description) > maxVarDescriptionLength {
		return errors.Errorf("variable description must be at most %d characters", maxVarDescriptionLength)
	}

	if utf8.RuneCountInString(string(v.Default)) > maxVarDefaultLength {
		return errors.Errorf("variable default must be at most %d characters", maxVarDefaultLength)
	}

	if !v.Type.IsKnown() {
		return errors.Errorf("unknown variable type: %s", v.Type)
	}

	return nil
}

func (v *GameModVar) validateOptions() error {
	isSelect := v.EffectiveType() == GameModVarTypeSelect

	if !isSelect {
		if len(v.Options) > 0 {
			return errors.New("options require type select")
		}

		if v.AllowCustom {
			return errors.New("allow_custom requires type select")
		}

		return nil
	}

	if len(v.Options) == 0 {
		return errors.New("type select requires a non-empty options list")
	}

	seen := make(map[string]struct{}, len(v.Options))
	for i, option := range v.Options {
		if option.Value == "" {
			return errors.Errorf("options[%d]: value is required", i)
		}

		if utf8.RuneCountInString(option.Value) > maxVarOptionValueLength {
			return errors.Errorf("options[%d]: value must be at most %d characters", i, maxVarOptionValueLength)
		}

		if utf8.RuneCountInString(option.Label) > maxVarOptionLabelLength {
			return errors.Errorf("options[%d]: label must be at most %d characters", i, maxVarOptionLabelLength)
		}

		if _, exists := seen[option.Value]; exists {
			return errors.Errorf("options[%d]: duplicate value: %s", i, option.Value)
		}
		seen[option.Value] = struct{}{}

		if err := option.I18n.validate(); err != nil {
			return errors.WithMessagef(err, "options[%d]", i)
		}
	}

	return nil
}

func (v *GameModVar) validateBoolValues() error {
	if v.EffectiveType() != GameModVarTypeBool {
		if v.TrueValue != nil {
			return errors.New("true_value requires type bool")
		}

		if v.FalseValue != nil {
			return errors.New("false_value requires type bool")
		}

		return nil
	}

	trueValue := v.TrueValueOrDefault()
	falseValue := v.FalseValueOrDefault()

	if utf8.RuneCountInString(trueValue) > maxVarBoolValueLength {
		return errors.Errorf("true_value must be at most %d characters", maxVarBoolValueLength)
	}

	if utf8.RuneCountInString(falseValue) > maxVarBoolValueLength {
		return errors.Errorf("false_value must be at most %d characters", maxVarBoolValueLength)
	}

	if trueValue == falseValue {
		return errors.New("true_value and false_value must differ")
	}

	// The schema requires a bool default to name one of the two states; an empty
	// default means "not set" and is left alone.
	if v.Default != "" && string(v.Default) != trueValue && string(v.Default) != falseValue {
		return errors.New("default must equal true_value or false_value")
	}

	return nil
}

func (r *GameModVarRules) validate(v *GameModVar) error {
	if r == nil {
		return nil
	}

	if r.IsEmpty() {
		return errors.New("rules must contain at least one rule")
	}

	if !v.EffectiveType().IsNumeric() && (r.Min != nil || r.Max != nil) {
		return errors.New("rules min and max require type int or float")
	}

	if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
		return errors.New("rules min must not be greater than max")
	}

	if !v.acceptsFreeText() && (r.MinLength != nil || r.MaxLength != nil || r.Pattern != "") {
		return errors.New(
			"rules min_length, max_length and pattern require a textual type or select with allow_custom",
		)
	}

	if r.MinLength != nil && *r.MinLength < 0 {
		return errors.New("rules min_length must not be negative")
	}

	if r.MaxLength != nil && *r.MaxLength < 1 {
		return errors.New("rules max_length must be at least 1")
	}

	if r.MinLength != nil && r.MaxLength != nil && *r.MinLength > *r.MaxLength {
		return errors.New("rules min_length must not be greater than max_length")
	}

	if r.Pattern != "" {
		if _, err := compileVarPattern(r.Pattern); err != nil {
			return errors.WithMessage(err, "rules pattern")
		}
	}

	return nil
}

func (m GameModVarI18n) validate() error {
	for locale, translation := range m {
		if err := validateLocaleKey(locale); err != nil {
			return err
		}

		if translation.IsEmpty() {
			return errors.Errorf("i18n[%s]: at least one of info or description is required", locale)
		}

		if utf8.RuneCountInString(translation.Info) > maxVarInfoLength {
			return errors.Errorf("i18n[%s]: info must be at most %d characters", locale, maxVarInfoLength)
		}

		if utf8.RuneCountInString(translation.Description) > maxVarDescriptionLength {
			return errors.Errorf(
				"i18n[%s]: description must be at most %d characters", locale, maxVarDescriptionLength,
			)
		}
	}

	return nil
}

func (m GameModVarOptionI18n) validate() error {
	for locale, translation := range m {
		if err := validateLocaleKey(locale); err != nil {
			return err
		}

		if translation.Label == "" {
			return errors.Errorf("i18n[%s]: label is required", locale)
		}

		if utf8.RuneCountInString(translation.Label) > maxVarOptionLabelLength {
			return errors.Errorf("i18n[%s]: label must be at most %d characters", locale, maxVarOptionLabelLength)
		}
	}

	return nil
}

func (m GameModFastRconI18n) validate() error {
	for locale, translation := range m {
		if err := validateLocaleKey(locale); err != nil {
			return err
		}

		if translation.Info == "" {
			return errors.Errorf("i18n[%s]: info is required", locale)
		}

		if utf8.RuneCountInString(translation.Info) > maxFastRconInfoLength {
			return errors.Errorf("i18n[%s]: info must be at most %d characters", locale, maxFastRconInfoLength)
		}
	}

	return nil
}

func validateLocaleKey(locale string) error {
	if locale == baseLocale {
		return errors.New("i18n must not contain the en locale: it is the base language")
	}

	if !localeRe.MatchString(locale) {
		return errors.Errorf("i18n[%s]: invalid locale, a lowercase BCP-47 code is expected", locale)
	}

	return nil
}

func (l GameModFastRconList) Validate() error {
	for i := range l {
		if err := l[i].Validate(); err != nil {
			return errors.WithMessagef(err, "fast_rcon[%d]", i)
		}
	}

	return nil
}

func (f *GameModFastRcon) Validate() error {
	if f.Info == "" {
		return errors.New("info is required")
	}

	if utf8.RuneCountInString(f.Info) > maxFastRconInfoLength {
		return errors.Errorf("info must be at most %d characters", maxFastRconInfoLength)
	}

	if f.Command == "" {
		return errors.New("command is required")
	}

	return f.I18n.validate()
}

// GameModVarValueError reports one violated rule for one variable so the HTTP
// layer can build a per-field 422 body.
type GameModVarValueError struct {
	Var    string
	Rule   string
	Detail string
}

func (e *GameModVarValueError) Error() string {
	return e.Var + ": " + e.Detail
}

func newVarValueError(v *GameModVar, rule, detail string) error {
	return &GameModVarValueError{Var: v.Var, Rule: rule, Detail: detail}
}

// NormalizeValue validates a decoded JSON value against the variable definition
// and returns the canonical string that is stored and substituted into {var}.
func (v *GameModVar) NormalizeValue(value any) (string, error) {
	if v.EffectiveType() == GameModVarTypeBool {
		return v.normalizeBoolValue(value)
	}

	raw, err := varValueToString(value)
	if err != nil {
		return "", newVarValueError(v, "type", err.Error())
	}

	// The schema applies rules to non-empty values only; emptiness is rejected
	// by required and nothing else.
	if raw == "" {
		if v.Rules.IsRequired() {
			return "", newVarValueError(v, "required", "value is required")
		}

		return "", nil
	}

	switch v.EffectiveType() {
	case GameModVarTypeInt:
		return v.normalizeIntValue(raw)
	case GameModVarTypeFloat:
		return v.normalizeFloatValue(raw)
	case GameModVarTypeSelect:
		return v.normalizeSelectValue(raw)
	default:
		if err := v.checkTextRules(raw); err != nil {
			return "", err
		}

		return raw, nil
	}
}

func (v *GameModVar) normalizeBoolValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		if v.Rules.IsRequired() {
			return "", newVarValueError(v, "required", "value is required")
		}

		return v.FalseValueOrDefault(), nil
	case bool:
		if typed {
			return v.TrueValueOrDefault(), nil
		}

		return v.FalseValueOrDefault(), nil
	}

	raw, err := varValueToString(value)
	if err != nil {
		return "", newVarValueError(v, "type", err.Error())
	}

	// A definition-declared value wins over the generic spellings so a variable
	// with true_value "on" round-trips exactly.
	switch raw {
	case v.TrueValueOrDefault():
		return v.TrueValueOrDefault(), nil
	case v.FalseValueOrDefault():
		return v.FalseValueOrDefault(), nil
	}

	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "on", "yes", "y":
		return v.TrueValueOrDefault(), nil
	case "0", "false", "off", "no", "n":
		return v.FalseValueOrDefault(), nil
	case "":
		if v.Rules.IsRequired() {
			return "", newVarValueError(v, "required", "value is required")
		}

		return v.FalseValueOrDefault(), nil
	}

	return "", newVarValueError(v, "type", "value must be a boolean")
}

func (v *GameModVar) normalizeIntValue(raw string) (string, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return "", newVarValueError(v, "type", "value must be an integer")
	}

	if err := v.checkNumericRules(float64(parsed)); err != nil {
		return "", err
	}

	return strconv.FormatInt(parsed, 10), nil
}

func (v *GameModVar) normalizeFloatValue(raw string) (string, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return "", newVarValueError(v, "type", "value must be a number")
	}

	if err := v.checkNumericRules(parsed); err != nil {
		return "", err
	}

	return strconv.FormatFloat(parsed, 'f', -1, 64), nil
}

func (v *GameModVar) normalizeSelectValue(raw string) (string, error) {
	if v.Options.Contains(raw) {
		return raw, nil
	}

	if !v.AllowCustom {
		return "", newVarValueError(v, "options", "value must be one of the allowed options")
	}

	if err := v.checkTextRules(raw); err != nil {
		return "", err
	}

	return raw, nil
}

func (v *GameModVar) checkNumericRules(value float64) error {
	if v.Rules == nil {
		return nil
	}

	if v.Rules.Min != nil && value < *v.Rules.Min {
		return newVarValueError(v, "min", "value must be at least "+formatRuleNumber(*v.Rules.Min))
	}

	if v.Rules.Max != nil && value > *v.Rules.Max {
		return newVarValueError(v, "max", "value must be at most "+formatRuleNumber(*v.Rules.Max))
	}

	return nil
}

func (v *GameModVar) checkTextRules(raw string) error {
	length := utf8.RuneCountInString(raw)

	if v.Rules == nil || v.Rules.MaxLength == nil {
		if length > maxServerSettingValueLength {
			return newVarValueError(
				v, "max_length", "value must be at most "+strconv.Itoa(maxServerSettingValueLength)+" characters",
			)
		}
	}

	if v.Rules == nil {
		return nil
	}

	if v.Rules.MinLength != nil && length < *v.Rules.MinLength {
		return newVarValueError(
			v, "min_length", "value must be at least "+strconv.Itoa(*v.Rules.MinLength)+" characters",
		)
	}

	if v.Rules.MaxLength != nil && length > *v.Rules.MaxLength {
		return newVarValueError(
			v, "max_length", "value must be at most "+strconv.Itoa(*v.Rules.MaxLength)+" characters",
		)
	}

	if v.Rules.Pattern != "" {
		compiled, err := compileVarPattern(v.Rules.Pattern)
		if err != nil {
			return newVarValueError(v, "pattern", "value cannot be checked: the pattern is invalid")
		}

		if !compiled.MatchString(raw) {
			return newVarValueError(v, "pattern", "value has an invalid format")
		}
	}

	return nil
}

func formatRuleNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// varValueToString accepts the JSON scalars a client may send and renders them
// as the text that will be substituted into {var}.
func varValueToString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		return typed.String(), nil
	default:
		return "", errors.New("value must be a string, a number or a boolean")
	}
}

// FormatValue types a stored string for the JSON response according to the
// variable type. A value that does not parse is returned as-is: the response is
// best-effort, only writes are strict.
func (v *GameModVar) FormatValue(stored string) any {
	switch v.EffectiveType() {
	case GameModVarTypeBool:
		switch stored {
		case v.TrueValueOrDefault():
			return true
		case v.FalseValueOrDefault():
			return false
		}

		switch strings.ToLower(strings.TrimSpace(stored)) {
		case "1", "true", "on", "yes", "y":
			return true
		case "0", "false", "off", "no", "n", "":
			return false
		}

		return stored
	case GameModVarTypeInt:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(stored), 10, 64); err == nil {
			return parsed
		}

		return stored
	case GameModVarTypeFloat:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(stored), 64); err == nil {
			return parsed
		}

		return stored
	default:
		return stored
	}
}
