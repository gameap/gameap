package putnode

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
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
)

type updateNodeInput struct {
	Enabled             *flexible.Bool `json:"enabled,omitempty"`
	Name                *string        `json:"name,omitempty"`
	OS                  *string        `json:"os,omitempty"`
	Location            *string        `json:"location,omitempty"`
	Provider            *string        `json:"provider,omitempty"`
	IP                  []string       `json:"ip,omitempty"`
	RAM                 *string        `json:"ram,omitempty"`
	CPU                 *string        `json:"cpu,omitempty"`
	WorkPath            *string        `json:"work_path,omitempty"`
	SteamcmdPath        *string        `json:"steamcmd_path,omitempty"`
	GdaemonHost         *string        `json:"gdaemon_host,omitempty"`
	GdaemonPort         *flexible.Int  `json:"gdaemon_port,omitempty"`
	GdaemonAPIKey       *string        `json:"gdaemon_api_key,omitempty"`
	GdaemonLogin        *string        `json:"gdaemon_login,omitempty"`
	GdaemonPassword     *string        `json:"gdaemon_password,omitempty"`
	GdaemonServerCert   *string        `json:"gdaemon_server_cert,omitempty"`
	ClientCertificateID *flexible.Uint `json:"client_certificate_id,omitempty"`
	PreferInstallMethod *string        `json:"prefer_install_method,omitempty"`
	ScriptInstall       *string        `json:"script_install,omitempty"`
	ScriptReinstall     *string        `json:"script_reinstall,omitempty"`
	ScriptUpdate        *string        `json:"script_update,omitempty"`
	ScriptStart         *string        `json:"script_start,omitempty"`
	ScriptPause         *string        `json:"script_pause,omitempty"`
	ScriptUnpause       *string        `json:"script_unpause,omitempty"`
	ScriptStop          *string        `json:"script_stop,omitempty"`
	ScriptKill          *string        `json:"script_kill,omitempty"`
	ScriptRestart       *string        `json:"script_restart,omitempty"`
	ScriptStatus        *string        `json:"script_status,omitempty"`
	ScriptStats         *string        `json:"script_stats,omitempty"`
	ScriptGetConsole    *string        `json:"script_get_console,omitempty"`
	ScriptSendCommand   *string        `json:"script_send_command,omitempty"`
	ScriptDelete        *string        `json:"script_delete,omitempty"`
}

func (in *updateNodeInput) Validate() error {
	validators := []func() error{
		in.validateName,
		in.validateLocation,
		in.validateProvider,
		in.validateIPs,
		in.validateOS,
		in.validateWorkPath,
		in.validateSteamcmdPath,
		in.validateGdaemonHost,
		in.validateGdaemonPort,
		in.validateGdaemonServerCert,
	}

	for _, validator := range validators {
		if err := validator(); err != nil {
			return err
		}
	}

	return nil
}

func (in *updateNodeInput) validateName() error {
	if in.Name == nil {
		return nil
	}
	if *in.Name == "" {
		return ErrNameRequired
	}
	if len(*in.Name) > maxNameLength {
		return ErrNameTooLong
	}

	return nil
}

func (in *updateNodeInput) validateLocation() error {
	if in.Location == nil {
		return nil
	}
	if *in.Location == "" {
		return ErrLocationRequired
	}
	if len(*in.Location) > maxLocationLength {
		return ErrLocationTooLong
	}

	return nil
}

func (in *updateNodeInput) validateProvider() error {
	if in.Provider != nil && len(*in.Provider) > maxProviderLength {
		return ErrProviderTooLong
	}

	return nil
}

func (in *updateNodeInput) validateIPs() error {
	if in.IP == nil {
		return nil
	}
	if len(in.IP) == 0 {
		return ErrIPRequired
	}

	for _, ip := range in.IP {
		if !validation.IsValidIPOrHostname(ip) {
			return ErrInvalidIPAddress
		}
	}

	return nil
}

func (in *updateNodeInput) validateOS() error {
	if in.OS == nil {
		return nil
	}
	if *in.OS == "" {
		return ErrOSRequired
	}
	osLower := strings.ToLower(*in.OS)
	if osLower != "linux" && osLower != "windows" {
		return ErrInvalidOS
	}

	return nil
}

func (in *updateNodeInput) validateWorkPath() error {
	if in.WorkPath == nil {
		return nil
	}
	if *in.WorkPath == "" {
		return ErrWorkPathRequired
	}
	if len(*in.WorkPath) > maxPathLength {
		return ErrWorkPathTooLong
	}

	return nil
}

func (in *updateNodeInput) validateSteamcmdPath() error {
	if in.SteamcmdPath != nil && len(*in.SteamcmdPath) > maxPathLength {
		return ErrSteamcmdPathTooLong
	}

	return nil
}

func (in *updateNodeInput) validateGdaemonHost() error {
	if in.GdaemonHost == nil {
		return nil
	}
	if *in.GdaemonHost == "" {
		return ErrGdaemonHostRequired
	}
	if len(*in.GdaemonHost) > maxNameLength {
		return ErrGdaemonHostTooLong
	}

	return nil
}

func (in *updateNodeInput) validateGdaemonPort() error {
	if in.GdaemonPort == nil {
		return nil
	}
	if in.GdaemonPort.Int() < minPortNumber || in.GdaemonPort.Int() > maxPortNumber {
		return ErrGdaemonPortInvalid
	}

	return nil
}

func (in *updateNodeInput) validateGdaemonServerCert() error {
	if in.GdaemonServerCert != nil && len(*in.GdaemonServerCert) > maxGdaemonServerCertSize {
		return api.NewValidationError("gdaemon_server_cert is too large")
	}

	return nil
}

func (in *updateNodeInput) ApplyToNode(node *domain.Node) {
	in.applyBasicFields(node)
	in.applyGdaemonFields(node)
	in.applyScriptFields(node)
}

func (in *updateNodeInput) applyBasicFields(node *domain.Node) {
	if in.Enabled != nil {
		node.Enabled = in.Enabled.Bool()
	}
	if in.Name != nil {
		node.Name = *in.Name
	}
	if in.OS != nil {
		node.OS = domain.ParseNodeOS(*in.OS)
	}
	if in.Location != nil {
		node.Location = *in.Location
	}
	if in.Provider != nil {
		node.Provider = in.Provider
	}
	if in.IP != nil {
		node.IPs = in.IP
	}
	if in.RAM != nil {
		node.RAM = in.RAM
	}
	if in.CPU != nil {
		node.CPU = in.CPU
	}
	if in.WorkPath != nil {
		node.WorkPath = *in.WorkPath
	}
	if in.SteamcmdPath != nil {
		node.SteamcmdPath = in.SteamcmdPath
	}
}

func (in *updateNodeInput) applyGdaemonFields(node *domain.Node) {
	if in.GdaemonHost != nil {
		node.GdaemonHost = *in.GdaemonHost
	}
	if in.GdaemonPort != nil {
		node.GdaemonPort = in.GdaemonPort.Int()
	}
	if in.GdaemonAPIKey != nil {
		node.GdaemonAPIKey = *in.GdaemonAPIKey
	}
	if in.GdaemonLogin != nil {
		node.GdaemonLogin = in.GdaemonLogin
	}
	if in.GdaemonPassword != nil {
		node.GdaemonPassword = in.GdaemonPassword
	}
	if in.ClientCertificateID != nil {
		node.ClientCertificateID = in.ClientCertificateID.Uint()
	}
	if in.PreferInstallMethod != nil {
		node.PreferInstallMethod = domain.NodePreferInstallMethod(*in.PreferInstallMethod)
	}
}

func (in *updateNodeInput) applyScriptFields(node *domain.Node) {
	if in.ScriptInstall != nil {
		node.ScriptInstall = in.ScriptInstall
	}
	if in.ScriptReinstall != nil {
		node.ScriptReinstall = in.ScriptReinstall
	}
	if in.ScriptUpdate != nil {
		node.ScriptUpdate = in.ScriptUpdate
	}
	if in.ScriptStart != nil {
		node.ScriptStart = in.ScriptStart
	}
	if in.ScriptPause != nil {
		node.ScriptPause = in.ScriptPause
	}
	if in.ScriptUnpause != nil {
		node.ScriptUnpause = in.ScriptUnpause
	}
	if in.ScriptStop != nil {
		node.ScriptStop = in.ScriptStop
	}
	if in.ScriptKill != nil {
		node.ScriptKill = in.ScriptKill
	}
	if in.ScriptRestart != nil {
		node.ScriptRestart = in.ScriptRestart
	}
	if in.ScriptStatus != nil {
		node.ScriptStatus = in.ScriptStatus
	}
	if in.ScriptStats != nil {
		node.ScriptStats = in.ScriptStats
	}
	if in.ScriptGetConsole != nil {
		node.ScriptGetConsole = in.ScriptGetConsole
	}
	if in.ScriptSendCommand != nil {
		node.ScriptSendCommand = in.ScriptSendCommand
	}
	if in.ScriptDelete != nil {
		node.ScriptDelete = in.ScriptDelete
	}
}
