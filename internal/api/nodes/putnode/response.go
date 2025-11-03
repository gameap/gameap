package putnode

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

func newNodeResponse(node *domain.Node) nodeResponse {
	return nodeResponse{
		ID:                  node.ID,
		Enabled:             node.Enabled,
		Name:                node.Name,
		OS:                  string(node.OS),
		Location:            node.Location,
		Provider:            node.Provider,
		IPs:                 node.IPs,
		RAM:                 node.RAM,
		CPU:                 node.CPU,
		WorkPath:            node.WorkPath,
		SteamcmdPath:        node.SteamcmdPath,
		GdaemonHost:         node.GdaemonHost,
		GdaemonPort:         node.GdaemonPort,
		GdaemonAPIKey:       node.GdaemonAPIKey,
		GdaemonLogin:        node.GdaemonLogin,
		GdaemonPassword:     node.GdaemonPassword,
		GdaemonServerCert:   node.GdaemonServerCert,
		ClientCertificateID: node.ClientCertificateID,
		PreferInstallMethod: string(node.PreferInstallMethod),
		ScriptInstall:       node.ScriptInstall,
		ScriptReinstall:     node.ScriptReinstall,
		ScriptUpdate:        node.ScriptUpdate,
		ScriptStart:         node.ScriptStart,
		ScriptPause:         node.ScriptPause,
		ScriptUnpause:       node.ScriptUnpause,
		ScriptStop:          node.ScriptStop,
		ScriptKill:          node.ScriptKill,
		ScriptRestart:       node.ScriptRestart,
		ScriptStatus:        node.ScriptStatus,
		ScriptStats:         node.ScriptStats,
		ScriptGetConsole:    node.ScriptGetConsole,
		ScriptSendCommand:   node.ScriptSendCommand,
		ScriptDelete:        node.ScriptDelete,
		CreatedAt:           node.CreatedAt,
		UpdatedAt:           node.UpdatedAt,
		DeletedAt:           node.DeletedAt,
	}
}
