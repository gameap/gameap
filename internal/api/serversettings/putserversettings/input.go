package putserversettings

import (
	settingsbase "github.com/gameap/gameap/internal/api/serversettings/base"
	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrSettingNameRequired = api.NewValidationError("setting name is required")
)

type saveSettingsInput []settingsbase.InputSetting

func (s *saveSettingsInput) Validate() error {
	for _, setting := range *s {
		if setting.Name == "" {
			return ErrSettingNameRequired
		}
	}

	return nil
}
