package putserversettings

import (
	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrSettingNameRequired = api.NewValidationError("setting name is required")
)

type settingInput struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type saveSettingsInput []settingInput

func (s *saveSettingsInput) Validate() error {
	for _, setting := range *s {
		if setting.Name == "" {
			return ErrSettingNameRequired
		}
	}

	return nil
}

func (s *saveSettingsInput) ToSettingsMap() map[string]any {
	settingsMap := make(map[string]any, len(*s))
	for _, setting := range *s {
		settingsMap[setting.Name] = setting.Value
	}

	return settingsMap
}
