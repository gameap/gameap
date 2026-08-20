package pelicaneggimporter

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
)

// These mirror the panel limits. A name longer than the limit cannot be stored
// as a per-server setting, so such a variable is dropped rather than becoming a
// variable nobody can override; the two text fields are truncated instead,
// because losing a long description is better than losing the whole mapping.
const (
	maxVarNameLength        = 32
	maxVarInfoLength        = 128
	maxVarDescriptionLength = 1000
)

// pelicanRegexRuleRe matches Laravel's regex:/.../ rule and captures the pattern
// between the delimiters.
var pelicanRegexRuleRe = regexp.MustCompile(`^regex:/(.*)/[a-zA-Z]*$`)

// transformVariables maps Pelican egg variables to GameAP game mod variables,
// carrying over the field type and the Laravel validation rules.
func transformVariables(variables []gamesimport.PelicanEggVariable) domain.GameModVarList {
	vars := make(domain.GameModVarList, 0, len(variables))

	for _, v := range variables {
		if utf8.RuneCountInString(v.EnvVariable) > maxVarNameLength {
			slog.Warn("skipping pelican egg variable with a name longer than the panel allows",
				slog.String("variable", v.EnvVariable),
				slog.Int("limit", maxVarNameLength),
			)

			continue
		}

		gameModVar := domain.GameModVar{
			Var:         v.EnvVariable,
			Default:     domain.GameModVarDefault(v.DefaultValue),
			Info:        buildVarInfo(v),
			AdminVar:    !v.UserEditable,
			Description: truncateRunes(v.Description, maxVarDescriptionLength),
		}

		applyPelicanRules(&gameModVar, v)
		gameModVar.Normalize()

		if err := gameModVar.Validate(); err != nil {
			slog.Warn("importing pelican egg variable without its rules: the mapping is invalid",
				slog.String("variable", v.EnvVariable),
				slog.String("error", err.Error()),
			)

			gameModVar = domain.GameModVar{
				Var:      v.EnvVariable,
				Default:  domain.GameModVarDefault(v.DefaultValue),
				Info:     buildVarInfo(v),
				AdminVar: !v.UserEditable,
			}
		}

		vars = append(vars, gameModVar)
	}

	return vars
}

// applyPelicanRules translates the egg's field_type and Laravel rules into the
// GameAP type, options and validation rules.
//
// The type is resolved first: min and max mean a numeric bound for a number and
// a length for a string, so the bounds cannot be read until the type is known,
// and Laravel rules carry no guaranteed order.
func applyPelicanRules(target *domain.GameModVar, source gamesimport.PelicanEggVariable) {
	parsed := parsePelicanRules(source.Rules)

	for _, rule := range parsed {
		switch rule.name {
		case "integer":
			target.Type = domain.GameModVarTypeInt
		case "numeric":
			target.Type = domain.GameModVarTypeFloat
		case "boolean":
			target.Type = domain.GameModVarTypeBool
		case "in":
			applyPelicanInRule(target, rule.argument)
		}
	}

	// field_type wins over the rules: an egg marks a secret as a password there
	// while its rules still say "string".
	if strings.EqualFold(source.FieldType, "password") {
		target.Type = domain.GameModVarTypePassword
	}

	rules := &domain.GameModVarRules{}

	for _, rule := range parsed {
		switch rule.name {
		case "required":
			rules.Required = new(true)
		case "min":
			applyPelicanBound(target, rules, rule.argument, true)
		case "max":
			applyPelicanBound(target, rules, rule.argument, false)
		case "between":
			applyPelicanBetween(target, rules, rule.argument)
		case "regex":
			applyPelicanRegex(rules, rule.raw)
		}
	}

	if !rules.IsEmpty() {
		target.Rules = rules
	}
}

type pelicanRule struct {
	raw      string
	name     string
	argument string
}

// parsePelicanRules flattens both shapes an egg may use: the legacy single
// "required|string|max:64" string and the PLCN_v3 array of individual rules.
func parsePelicanRules(raw gamesimport.FlexibleRules) []pelicanRule {
	parsed := make([]pelicanRule, 0, len(raw))

	for _, entry := range raw {
		for part := range strings.SplitSeq(entry, "|") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			name, argument, found := strings.Cut(part, ":")
			if !found {
				argument = ""
			}

			parsed = append(parsed, pelicanRule{
				raw:      part,
				name:     strings.ToLower(name),
				argument: argument,
			})
		}
	}

	return parsed
}

func applyPelicanInRule(target *domain.GameModVar, argument string) {
	values := splitPelicanList(argument)
	if len(values) == 0 {
		return
	}

	// in:0,1 is how an egg spells a checkbox.
	if len(values) == 2 && values[0] == "0" && values[1] == "1" {
		target.Type = domain.GameModVarTypeBool
		target.TrueValue = new("1")
		target.FalseValue = new("0")

		return
	}

	target.Type = domain.GameModVarTypeSelect

	options := make(domain.GameModVarOptions, 0, len(values))
	for _, value := range values {
		options = append(options, domain.GameModVarOption{Value: value})
	}
	target.Options = options
}

// applyPelicanBound maps min/max, which Laravel reads as a numeric bound for a
// number and as a length for a string.
func applyPelicanBound(target *domain.GameModVar, rules *domain.GameModVarRules, argument string, isMin bool) {
	if target.EffectiveType().IsNumeric() {
		bound, err := strconv.ParseFloat(argument, 64)
		if err != nil {
			return
		}

		if isMin {
			rules.Min = &bound
		} else {
			rules.Max = &bound
		}

		return
	}

	length, err := strconv.Atoi(argument)
	if err != nil {
		return
	}

	if isMin {
		rules.MinLength = &length
	} else {
		rules.MaxLength = &length
	}
}

func applyPelicanBetween(target *domain.GameModVar, rules *domain.GameModVarRules, argument string) {
	bounds := splitPelicanList(argument)
	if len(bounds) != 2 {
		return
	}

	applyPelicanBound(target, rules, bounds[0], true)
	applyPelicanBound(target, rules, bounds[1], false)
}

// applyPelicanRegex keeps the pattern only when it compiles under RE2: egg
// patterns come from PHP's PCRE and may use lookarounds Go cannot express.
func applyPelicanRegex(rules *domain.GameModVarRules, raw string) {
	match := pelicanRegexRuleRe.FindStringSubmatch(raw)
	if match == nil {
		return
	}

	pattern := strings.TrimPrefix(strings.TrimSuffix(match[1], "$"), "^")
	if pattern == "" {
		return
	}

	if _, err := regexp.Compile(pattern); err != nil {
		slog.Debug("dropping pelican egg regex rule that RE2 cannot compile",
			slog.String("pattern", pattern),
			slog.String("error", err.Error()),
		)

		return
	}

	rules.Pattern = pattern
}

func splitPelicanList(argument string) []string {
	if argument == "" {
		return nil
	}

	parts := strings.Split(argument, ",")

	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), `"`)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}

	return values
}

// buildVarInfo picks the short label. The egg's name is the short one and its
// description is the long help text, which now has a field of its own.
func buildVarInfo(v gamesimport.PelicanEggVariable) string {
	if v.Name != "" {
		return truncateRunes(v.Name, maxVarInfoLength)
	}

	if v.Description != "" {
		return truncateRunes(v.Description, maxVarInfoLength)
	}

	return v.EnvVariable
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}

	return string([]rune(value)[:limit])
}
