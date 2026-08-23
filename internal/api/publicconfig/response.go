package publicconfig

type Response struct {
	DefaultLanguage string         `json:"default_language,omitempty"`
	Captcha         *CaptchaConfig `json:"captcha,omitempty"`
}

// CaptchaConfig exposes only the public parts of the captcha configuration
// the login form needs to render the widget. The secret key is never
// included here.
type CaptchaConfig struct {
	Provider    string `json:"provider"`
	SiteKey     string `json:"site_key"`
	InstanceURL string `json:"instance_url,omitempty"`
}
