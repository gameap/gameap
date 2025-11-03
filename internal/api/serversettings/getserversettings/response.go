package getserversettings

type SettingResponse struct {
	Name     string `json:"name"`
	Value    any    `json:"value"`
	Type     string `json:"type"`
	Label    string `json:"label"`
	AdminVar bool   `json:"admin_var,omitempty"`
}
