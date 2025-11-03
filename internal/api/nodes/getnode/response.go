package getnode

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type nodeResponse struct {
	ID                  uint       `json:"id"`
	Enabled             bool       `json:"enabled"`
	Name                string     `json:"name"`
	OS                  string     `json:"os"`
	Location            string     `json:"location"`
	Provider            *string    `json:"provider"`
	IPs                 []string   `json:"ip"`
	RAM                 *string    `json:"ram"`
	CPU                 *string    `json:"cpu"`
	WorkPath            string     `json:"work_path"`
	SteamcmdPath        *string    `json:"steamcmd_path"`
	GdaemonHost         string     `json:"gdaemon_host"`
	GdaemonPort         int        `json:"gdaemon_port"`
	GdaemonAPIKey       string     `json:"gdaemon_api_key"`
	GdaemonLogin        *string    `json:"gdaemon_login"`
	GdaemonPassword     *string    `json:"gdaemon_password"`
	GdaemonServerCert   string     `json:"gdaemon_server_cert"`
	ClientCertificateID uint       `json:"client_certificate_id"`
	PreferInstallMethod string     `json:"prefer_install_method"`
	ScriptInstall       *string    `json:"script_install"`
	ScriptReinstall     *string    `json:"script_reinstall"`
	ScriptUpdate        *string    `json:"script_update"`
	ScriptStart         *string    `json:"script_start"`
	ScriptPause         *string    `json:"script_pause"`
	ScriptUnpause       *string    `json:"script_unpause"`
	ScriptStop          *string    `json:"script_stop"`
	ScriptKill          *string    `json:"script_kill"`
	ScriptRestart       *string    `json:"script_restart"`
	ScriptStatus        *string    `json:"script_status"`
	ScriptStats         *string    `json:"script_stats"`
	ScriptGetConsole    *string    `json:"script_get_console"`
	ScriptSendCommand   *string    `json:"script_send_command"`
	ScriptDelete        *string    `json:"script_delete"`
	CreatedAt           *time.Time `json:"created_at"`
	UpdatedAt           *time.Time `json:"updated_at"`
	DeletedAt           *time.Time `json:"deleted_at"`
}

func newNodeResponseFromNode(n *domain.Node) nodeResponse {
	return nodeResponse{
		ID:                  n.ID,
		Enabled:             n.Enabled,
		Name:                n.Name,
		OS:                  string(n.OS),
		Location:            n.Location,
		Provider:            n.Provider,
		IPs:                 n.IPs,
		RAM:                 n.RAM,
		CPU:                 n.CPU,
		WorkPath:            n.WorkPath,
		SteamcmdPath:        n.SteamcmdPath,
		GdaemonHost:         n.GdaemonHost,
		GdaemonPort:         n.GdaemonPort,
		GdaemonAPIKey:       n.GdaemonAPIKey,
		GdaemonLogin:        n.GdaemonLogin,
		GdaemonPassword:     n.GdaemonPassword,
		GdaemonServerCert:   n.GdaemonServerCert,
		ClientCertificateID: n.ClientCertificateID,
		PreferInstallMethod: string(n.PreferInstallMethod),
		ScriptInstall:       n.ScriptInstall,
		ScriptReinstall:     n.ScriptReinstall,
		ScriptUpdate:        n.ScriptUpdate,
		ScriptStart:         n.ScriptStart,
		ScriptPause:         n.ScriptPause,
		ScriptUnpause:       n.ScriptUnpause,
		ScriptStop:          n.ScriptStop,
		ScriptKill:          n.ScriptKill,
		ScriptRestart:       n.ScriptRestart,
		ScriptStatus:        n.ScriptStatus,
		ScriptStats:         n.ScriptStats,
		ScriptGetConsole:    n.ScriptGetConsole,
		ScriptSendCommand:   n.ScriptSendCommand,
		ScriptDelete:        n.ScriptDelete,
		CreatedAt:           n.CreatedAt,
		UpdatedAt:           n.UpdatedAt,
		DeletedAt:           n.DeletedAt,
	}
}
