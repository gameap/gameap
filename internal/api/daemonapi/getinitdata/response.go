package getinitdata

import "github.com/gameap/gameap/internal/domain"

type initDataResponse struct {
	WorkPath            string  `json:"work_path"`
	SteamcmdPath        *string `json:"steamcmd_path"`
	PreferInstallMethod string  `json:"prefer_install_method"`
	ScriptInstall       *string `json:"script_install"`
	ScriptReinstall     *string `json:"script_reinstall"`
	ScriptUpdate        *string `json:"script_update"`
	ScriptStart         *string `json:"script_start"`
	ScriptPause         *string `json:"script_pause"`
	ScriptUnpause       *string `json:"script_unpause"`
	ScriptStop          *string `json:"script_stop"`
	ScriptKill          *string `json:"script_kill"`
	ScriptRestart       *string `json:"script_restart"`
	ScriptStatus        *string `json:"script_status"`
	ScriptGetConsole    *string `json:"script_get_console"`
	ScriptSendCommand   *string `json:"script_send_command"`
	ScriptDelete        *string `json:"script_delete"`
}

func newInitDataResponseFromNode(node *domain.Node) initDataResponse {
	return initDataResponse{
		WorkPath:            node.WorkPath,
		SteamcmdPath:        node.SteamcmdPath,
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
		ScriptGetConsole:    node.ScriptGetConsole,
		ScriptSendCommand:   node.ScriptSendCommand,
		ScriptDelete:        node.ScriptDelete,
	}
}
