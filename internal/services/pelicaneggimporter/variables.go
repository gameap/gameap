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
		if v.EnvVariable == "" {
			slog.Warn("skipping pelican egg variable without an env_variable name",
				slog.String("name", v.Name),
			)

			continue
		}

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

			// Only the rule mapping is dropped; the help text is still the best
			// description of the variable the egg has.
			gameModVar = domain.GameModVar{
				Var:         v.EnvVariable,
				Default:     domain.GameModVarDefault(v.DefaultValue),
				Info:        buildVarInfo(v),
				AdminVar:    !v.UserEditable,
				Description: truncateRunes(v.Description, maxVarDescriptionLength),
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
		for _, part := range splitRuleEntry(entry) {
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

// splitRuleEntry splits the legacy "required|string|max:64" spelling into its
// rules. A pipe inside a regex rule is part of the pattern rather than a
// delimiter, so "regex:/^(vanilla|paper)$/" stays one rule — the array form is
// what an egg is supposed to use for such a rule, and splitting it would leave
// two fragments that compile into nothing.
func splitRuleEntry(entry string) []string {
	parts := make([]string, 0, strings.Count(entry, "|")+1)

	for start := 0; ; {
		end := ruleEnd(entry, start)
		parts = append(parts, entry[start:end])

		if end == len(entry) {
			break
		}

		start = end + 1
	}

	return parts
}

// ruleEnd finds the delimiter that closes the rule starting at start, skipping
// the body of a regex rule, and returns len(entry) for the last rule.
func ruleEnd(entry string, start int) int {
	from := start + regexRuleBodyLength(entry[start:])

	if next := strings.IndexByte(entry[from:], '|'); next >= 0 {
		return from + next
	}

	return len(entry)
}

// regexRuleBodyLength reports how many bytes of s the leading regex rule spans,
// including its closing delimiter, and 0 when s does not start with one.
func regexRuleBodyLength(s string) int {
	trimmed := strings.TrimLeft(s, " \t")
	lower := strings.ToLower(trimmed)

	prefix := ""
	for _, candidate := range []string{"regex:", "not_regex:"} {
		if strings.HasPrefix(lower, candidate) {
			prefix = candidate

			break
		}
	}

	if prefix == "" || len(trimmed) == len(prefix) {
		return 0
	}

	offset := len(s) - len(trimmed) + len(prefix)
	pattern := trimmed[len(prefix):]
	delimiter := pattern[0]

	for i := 1; i < len(pattern); {
		switch pattern[i] {
		case '\\':
			i += 2
		case delimiter:
			return offset + i + 1
		default:
			i++
		}
	}

	// An unterminated pattern swallows the rest: splitting it would only produce
	// fragments of a regexp.
	return len(s)
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
