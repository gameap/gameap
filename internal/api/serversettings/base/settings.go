package base

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

const (
	// AutostartSettingKey and the two keys below are panel-owned settings: they
	// are not game mod variables and are always real booleans.
	AutostartSettingKey         = "autostart"
	AutostartCurrentSettingKey  = "autostart_current"
	UpdateBeforeStartSettingKey = "update_before_start"
)

// InputSetting is one submitted name/value pair. Value is untyped because its
// JSON type follows the variable definition.
type InputSetting struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// NormalizedSetting is a validated setting ready to be persisted.
type NormalizedSetting struct {
	Name  string
	Value domain.ServerSettingValue
}

// IsWritableBuiltIn reports whether the name is a panel-owned setting a client
// may write. autostart_current is deliberately excluded: it is maintained by the
// server control service, not by the settings form.
func IsWritableBuiltIn(name string) bool {
	switch name {
	case AutostartSettingKey, UpdateBeforeStartSettingKey:
		return true
	}

	return false
}

// Normalize whitelists the submitted settings against the game mod definition,
// drops admin-only variables for non-admins, validates every value against its
// rules and converts it to the canonical form that is stored.
//
// It reports every violation at once so the client can highlight all bad fields
// in one pass, and it returns nothing when any value is invalid: the caller must
// not perform a partial write.
func Normalize(gameMod *domain.GameMod, input []InputSetting, isAdmin bool) ([]NormalizedSetting, error) {
	definitions := varDefinitions(gameMod)

	// The input slice is walked in order so both the error order and the write
	// order are deterministic.
	normalized := make([]NormalizedSetting, 0, len(input))
	fieldErrors := make(map[string][]string)

	for _, setting := range input {
		if IsWritableBuiltIn(setting.Name) {
			normalized = append(normalized, NormalizedSetting{
				Name:  setting.Name,
				Value: domain.NewServerSettingValue(readLenientBool(setting.Value)),
			})

			continue
		}

		definition, known := definitions[setting.Name]
		if !known {
			continue
		}

		if definition.AdminVar && !isAdmin {
			continue
		}

		value, err := definition.NormalizeValue(setting.Value)
		if err != nil {
			fieldErrors[setting.Name] = append(fieldErrors[setting.Name], violationDetail(err))

			continue
		}

		normalized = append(normalized, NormalizedSetting{
			Name:  setting.Name,
			Value: domain.NewServerSettingValue(value),
		})
	}

	if len(fieldErrors) > 0 {
		return nil, api.NewFieldValidationError(fieldErrors)
	}

	return normalized, nil
}

// NormalizeVars applies the same validation to the raw servers.vars map. Keys
// that are not game mod variables are low-level administrator overrides and pass
// through untouched.
func NormalizeVars(gameMod *domain.GameMod, vars map[string]string) (map[string]string, error) {
	if vars == nil {
		return nil, nil
	}

	definitions := varDefinitions(gameMod)

	normalized := make(map[string]string, len(vars))
	fieldErrors := make(map[string][]string)

	for name, value := range vars {
		definition, known := definitions[name]
		if !known {
			normalized[name] = value

			continue
		}

		canonical, err := definition.NormalizeValue(value)
		if err != nil {
			fieldErrors[name] = append(fieldErrors[name], violationDetail(err))

			continue
		}

		normalized[name] = canonical
	}

	if len(fieldErrors) > 0 {
		return nil, api.NewFieldValidationError(fieldErrors)
	}

	return normalized, nil
}

func varDefinitions(gameMod *domain.GameMod) map[string]*domain.GameModVar {
	if gameMod == nil {
		return nil
	}

	definitions := make(map[string]*domain.GameModVar, len(gameMod.Vars))
	for i := range gameMod.Vars {
		definitions[gameMod.Vars[i].Var] = &gameMod.Vars[i]
	}

	return definitions
}

func violationDetail(err error) string {
	var valueErr *domain.GameModVarValueError
	if errors.As(err, &valueErr) {
		return valueErr.Detail
	}

	return err.Error()
}

// readLenientBool keeps the built-in settings working for every client that ever
// wrote them: the panel has posted "true", "1" and a real boolean over the years.
func readLenientBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(typed)))
		if err != nil {
			return strings.EqualFold(strings.TrimSpace(typed), "on")
		}

		return parsed
	case json.Number:
		parsed, err := typed.Float64()

		return err == nil && parsed != 0
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	default:
		return false
	}
}
