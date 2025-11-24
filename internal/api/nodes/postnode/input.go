package postnode

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/validation"
)

const (
	minNameLength            = 1
	maxNameLength            = 128
	maxDescriptionLength     = 1024
	maxLocationLength        = 128
	maxProviderLength        = 128
	maxPathLength            = 512
	minPortNumber            = 1
	maxPortNumber            = 65535
	maxGdaemonServerCertSize = 10 * 1024 * 1024
)

var (
	ErrNameRequired = api.NewValidationError("name is required")
	ErrNameTooLong  = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrDescriptionTooLong = api.NewValidationError(
		fmt.Sprintf("description must not exceed %d characters", maxDescriptionLength),
	)
	ErrLocationRequired = api.NewValidationError("location is required")
	ErrLocationTooLong  = api.NewValidationError(
		fmt.Sprintf("location must not exceed %d characters", maxLocationLength),
	)
	ErrProviderTooLong = api.NewValidationError(
		fmt.Sprintf("provider must not exceed %d characters", maxProviderLength),
	)
	ErrIPRequired       = api.NewValidationError("at least one IP address is required")
	ErrInvalidIPAddress = api.NewValidationError("invalid IP address or hostname format")
	ErrOSRequired       = api.NewValidationError("os is required")
	ErrInvalidOS        = api.NewValidationError("os must be either 'linux' or 'windows'")
	ErrWorkPathRequired = api.NewValidationError("work_path is required")
	ErrWorkPathTooLong  = api.NewValidationError(
		fmt.Sprintf("work_path must not exceed %d characters", maxPathLength),
	)
	ErrSteamcmdPathTooLong = api.NewValidationError(
		fmt.Sprintf("steamcmd_path must not exceed %d characters", maxPathLength),
	)
	ErrGdaemonHostRequired = api.NewValidationError("gdaemon_host is required")
	ErrGdaemonHostTooLong  = api.NewValidationError(
		fmt.Sprintf("gdaemon_host must not exceed %d characters", maxNameLength),
	)
	ErrGdaemonPortRequired = api.NewValidationError("gdaemon_port is required")
	ErrGdaemonPortInvalid  = api.NewValidationError(
		fmt.Sprintf("gdaemon_port must be between %d and %d", minPortNumber, maxPortNumber),
	)
	ErrClientCertificateIDZero = api.NewValidationError("client_certificate_id must be greater than 0")
	ErrScriptTooLong           = api.NewValidationError("script content is too long")
)

type createDedicatedServerInput struct {
	Name                string   `json:"name"`
	Description         *string  `json:"description,omitempty"`
	Location            string   `json:"location"`
	IP                  []string `json:"ip"`
	OS                  string   `json:"os"`
	Enabled             bool     `json:"enabled"`
	Provider            *string  `json:"provider,omitempty"`
	WorkPath            string   `json:"work_path"`
	SteamcmdPath        *string  `json:"steamcmd_path,omitempty"`
	GdaemonHost         string   `json:"gdaemon_host"`
	GdaemonPort         int      `json:"gdaemon_port"`
	ClientCertificateID uint     `json:"client_certificate_id"`
	GdaemonServerCert   string   `json:"gdaemon_server_cert"`

	PreferInstallMethod *string `json:"prefer_install_method,omitempty"`
	ScriptInstall       *string `json:"script_install,omitempty"`
	ScriptReinstall     *string `json:"script_reinstall,omitempty"`
	ScriptUpdate        *string `json:"script_update,omitempty"`
	ScriptStart         *string `json:"script_start,omitempty"`
	ScriptPause         *string `json:"script_pause,omitempty"`
	ScriptUnpause       *string `json:"script_unpause,omitempty"`
	ScriptStop          *string `json:"script_stop,omitempty"`
	ScriptKill          *string `json:"script_kill,omitempty"`
	ScriptRestart       *string `json:"script_restart,omitempty"`
	ScriptStatus        *string `json:"script_status,omitempty"`
	ScriptStats         *string `json:"script_stats,omitempty"`
	ScriptGetConsole    *string `json:"script_get_console,omitempty"`
	ScriptSendCommand   *string `json:"script_send_command,omitempty"`
	ScriptDelete        *string `json:"script_delete,omitempty"`
}

func (in *createDedicatedServerInput) Validate() error {
	if in.Name == "" {
		return ErrNameRequired
	}
	if len(in.Name) > maxNameLength {
		return ErrNameTooLong
	}

	if in.Description != nil && len(*in.Description) > maxDescriptionLength {
		return ErrDescriptionTooLong
	}

	if in.Location == "" {
		return ErrLocationRequired
	}
	if len(in.Location) > maxLocationLength {
		return ErrLocationTooLong
	}

	if in.Provider != nil && len(*in.Provider) > maxProviderLength {
		return ErrProviderTooLong
	}

	if len(in.IP) == 0 {
		return ErrIPRequired
	}

	for _, ip := range in.IP {
		if !validation.IsValidIPOrHostname(ip) {
			return ErrInvalidIPAddress
		}
	}

	if in.OS == "" {
		return ErrOSRequired
	}
	osLower := strings.ToLower(in.OS)
	if osLower != "linux" && osLower != "windows" {
		return ErrInvalidOS
	}

	if in.WorkPath == "" {
		return ErrWorkPathRequired
	}
	if len(in.WorkPath) > maxPathLength {
		return ErrWorkPathTooLong
	}

	if in.SteamcmdPath != nil && len(*in.SteamcmdPath) > maxPathLength {
		return ErrSteamcmdPathTooLong
	}

	if in.GdaemonHost == "" {
		return ErrGdaemonHostRequired
	}
	if len(in.GdaemonHost) > maxNameLength {
		return ErrGdaemonHostTooLong
	}

	if in.GdaemonPort < minPortNumber || in.GdaemonPort > maxPortNumber {
		return ErrGdaemonPortInvalid
	}

	if in.ClientCertificateID == 0 {
		return ErrClientCertificateIDZero
	}

	if len(in.GdaemonServerCert) > maxGdaemonServerCertSize {
		return api.NewValidationError("gdaemon_server_cert is too large")
	}

	return nil
}

func (in *createDedicatedServerInput) ToDomain(apiKey, certPath string) *domain.Node {
	return &domain.Node{
		Enabled:             in.Enabled,
		Name:                strings.TrimSpace(in.Name),
		OS:                  domain.ParseNodeOS(in.OS),
		Location:            strings.TrimSpace(in.Location),
		Provider:            trimStringPtr(in.Provider),
		IPs:                 in.IP,
		WorkPath:            strings.TrimSpace(in.WorkPath),
		SteamcmdPath:        trimStringPtr(in.SteamcmdPath),
		GdaemonHost:         strings.TrimSpace(in.GdaemonHost),
		GdaemonPort:         in.GdaemonPort,
		GdaemonAPIKey:       apiKey,
		GdaemonServerCert:   certPath,
		ClientCertificateID: in.ClientCertificateID,
		PreferInstallMethod: valueOrDefault(in.PreferInstallMethod, domain.NodePreferInstallMethodAuto),
		ScriptInstall:       trimStringPtr(in.ScriptInstall),
		ScriptReinstall:     trimStringPtr(in.ScriptReinstall),
		ScriptUpdate:        trimStringPtr(in.ScriptUpdate),
		ScriptStart:         trimStringPtr(in.ScriptStart),
		ScriptPause:         trimStringPtr(in.ScriptPause),
		ScriptUnpause:       trimStringPtr(in.ScriptUnpause),
		ScriptStop:          trimStringPtr(in.ScriptStop),
		ScriptKill:          trimStringPtr(in.ScriptKill),
		ScriptRestart:       trimStringPtr(in.ScriptRestart),
		ScriptStatus:        trimStringPtr(in.ScriptStatus),
		ScriptStats:         trimStringPtr(in.ScriptStats),
		ScriptGetConsole:    trimStringPtr(in.ScriptGetConsole),
		ScriptSendCommand:   trimStringPtr(in.ScriptSendCommand),
		ScriptDelete:        trimStringPtr(in.ScriptDelete),
	}
}

func valueOrDefault[T any](ptr any, defaultValue T) T {
	if ptr == nil {
		return defaultValue
	}

	v, ok := ptr.(T)
	if !ok {
		return defaultValue
	}

	return v
}

func trimStringPtr(s *string) *string {
	if s == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*s)

	return &trimmed
}
