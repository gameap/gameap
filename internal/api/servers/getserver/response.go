package getserver

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type adminGameResponse struct {
	Code                    string  `json:"code"`
	Name                    string  `json:"name"`
	Engine                  string  `json:"engine"`
	EngineVersion           string  `json:"engine_version"`
	SteamAppIDLinux         uint    `json:"steam_app_id_linux"`
	SteamAppIDWindows       uint    `json:"steam_app_id_windows"`
	SteamAppSetConfig       string  `json:"steam_app_set_config"`
	RemoteRepositoryLinux   *string `json:"remote_repository_linux"`
	RemoteRepositoryWindows *string `json:"remote_repository_windows"`
	LocalRepositoryLinux    *string `json:"local_repository_linux"`
	LocalRepositoryWindows  *string `json:"local_repository_windows"`
	Enabled                 int     `json:"enabled"`
}

type adminServerResponse struct {
	ID               uint               `json:"id"`
	UUID             string             `json:"uuid"`
	UUIDShort        string             `json:"uuid_short"`
	Enabled          bool               `json:"enabled"`
	Installed        int                `json:"installed"`
	Blocked          bool               `json:"blocked"`
	Name             string             `json:"name"`
	GameID           string             `json:"game_id"`
	DSID             uint               `json:"ds_id"`
	GameModID        uint               `json:"game_mod_id"`
	Expires          *time.Time         `json:"expires"`
	ServerIP         string             `json:"server_ip"`
	ServerPort       int                `json:"server_port"`
	QueryPort        *int               `json:"query_port"`
	RconPort         *int               `json:"rcon_port"`
	Game             *adminGameResponse `json:"game"`
	LastProcessCheck *time.Time         `json:"last_process_check"`
	Online           bool               `json:"online"`
	Rcon             *string            `json:"rcon"`
	Dir              string             `json:"dir"`
	SuUser           *string            `json:"su_user"`
	CPULimit         *int               `json:"cpu_limit"`
	RAMLimit         *int               `json:"ram_limit"`
	NetLimit         *int               `json:"net_limit"`
	StartCommand     *string            `json:"start_command"`
	StopCommand      *string            `json:"stop_command"`
	ForceStopCommand *string            `json:"force_stop_command"`
	RestartCommand   *string            `json:"restart_command"`
	ProcessActive    bool               `json:"process_active"`
	Aliases          map[string]any     `json:"aliases"`
	Vars             *string            `json:"vars"`
	CreatedAt        *time.Time         `json:"created_at"`
	UpdatedAt        *time.Time         `json:"updated_at"`
}

func newAdminGameResponse(g *domain.Game) *adminGameResponse {
	if g == nil {
		return nil
	}

	steamAppIDLinux := uint(0)
	if g.SteamAppIDLinux != nil {
		steamAppIDLinux = *g.SteamAppIDLinux
	}

	steamAppIDWindows := uint(0)
	if g.SteamAppIDWindows != nil {
		steamAppIDWindows = *g.SteamAppIDWindows
	}

	steamAppSetConfig := ""
	if g.SteamAppSetConfig != nil {
		steamAppSetConfig = *g.SteamAppSetConfig
	}

	return &adminGameResponse{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppIDLinux:         steamAppIDLinux,
		SteamAppIDWindows:       steamAppIDWindows,
		SteamAppSetConfig:       steamAppSetConfig,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Enabled:                 g.Enabled,
	}
}

func buildAliases(s *domain.Server) map[string]any {
	aliases := map[string]any{
		"ip":         s.ServerIP,
		"port":       s.ServerPort,
		"uuid":       s.UUID.String(),
		"uuid_short": s.UUIDShort,
	}

	if s.QueryPort != nil {
		aliases["query_port"] = *s.QueryPort
	}

	if s.RconPort != nil {
		aliases["rcon_port"] = *s.RconPort
	}

	if s.Rcon != nil {
		aliases["rcon_password"] = *s.Rcon
	}

	if s.Vars != nil && *s.Vars != "" {
		var varsMap map[string]any
		if err := json.Unmarshal([]byte(*s.Vars), &varsMap); err == nil {
			maps.Copy(aliases, varsMap)
		}
	}

	return aliases
}

func newAdminServerResponseFromServer(s *domain.Server, game *domain.Game) adminServerResponse {
	return adminServerResponse{
		ID:               s.ID,
		UUID:             s.UUID.String(),
		UUIDShort:        s.UUIDShort,
		Enabled:          s.Enabled,
		Installed:        int(s.Installed),
		Blocked:          s.Blocked,
		Name:             s.Name,
		GameID:           s.GameID,
		DSID:             s.DSID,
		GameModID:        s.GameModID,
		Expires:          s.Expires,
		ServerIP:         s.ServerIP,
		ServerPort:       s.ServerPort,
		QueryPort:        s.QueryPort,
		RconPort:         s.RconPort,
		Game:             newAdminGameResponse(game),
		LastProcessCheck: s.LastProcessCheck,
		Online:           s.IsOnline(),
		Rcon:             s.Rcon,
		Dir:              s.Dir,
		SuUser:           s.SuUser,
		CPULimit:         s.CPULimit,
		RAMLimit:         s.RAMLimit,
		NetLimit:         s.NetLimit,
		StartCommand:     s.StartCommand,
		StopCommand:      s.StopCommand,
		ForceStopCommand: s.ForceStopCommand,
		RestartCommand:   s.RestartCommand,
		ProcessActive:    s.ProcessActive,
		Aliases:          buildAliases(s),
		Vars:             s.Vars,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

type userGameResponse struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
}

type userServerResponse struct {
	ID               uint              `json:"id"`
	Enabled          bool              `json:"enabled"`
	Installed        int               `json:"installed"`
	Blocked          bool              `json:"blocked"`
	Name             string            `json:"name"`
	GameID           string            `json:"game_id"`
	GameModID        uint              `json:"game_mod_id"`
	Expires          *time.Time        `json:"expires"`
	ServerIP         string            `json:"server_ip"`
	ServerPort       int               `json:"server_port"`
	QueryPort        *int              `json:"query_port"`
	RconPort         *int              `json:"rcon_port"`
	Game             *userGameResponse `json:"game"`
	LastProcessCheck *time.Time        `json:"last_process_check"`
	Online           bool              `json:"online"`
	ProcessActive    bool              `json:"process_active"`
}

func newUserServerResponseFromServer(s *domain.Server, game *domain.Game) userServerResponse {
	return userServerResponse{
		ID:               s.ID,
		Enabled:          s.Enabled,
		Installed:        int(s.Installed),
		Blocked:          s.Blocked,
		Name:             s.Name,
		GameID:           s.GameID,
		GameModID:        s.GameModID,
		Expires:          s.Expires,
		ServerIP:         s.ServerIP,
		ServerPort:       s.ServerPort,
		QueryPort:        s.QueryPort,
		RconPort:         s.RconPort,
		Game:             newUserGameResponse(game),
		LastProcessCheck: s.LastProcessCheck,
		Online:           s.IsOnline(),
		ProcessActive:    s.ProcessActive,
	}
}

func newUserGameResponse(g *domain.Game) *userGameResponse {
	if g == nil {
		return nil
	}

	return &userGameResponse{
		Code:          g.Code,
		Name:          g.Name,
		Engine:        g.Engine,
		EngineVersion: g.EngineVersion,
	}
}
