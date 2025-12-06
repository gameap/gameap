package getserverid

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/google/uuid"
)

type ServerResponse struct {
	ID               uint                    `json:"id"`
	UUID             uuid.UUID               `json:"uuid"`
	UUIDShort        string                  `json:"uuid_short"`
	Enabled          bool                    `json:"enabled"`
	Installed        int                     `json:"installed"`
	Blocked          bool                    `json:"blocked"`
	Name             string                  `json:"name"`
	DSID             uint                    `json:"ds_id"`
	GameID           string                  `json:"game_id"`
	GameModID        uint                    `json:"game_mod_id"`
	Expires          *string                 `json:"expires"`
	ServerIP         string                  `json:"server_ip"`
	ServerPort       int                     `json:"server_port"`
	QueryPort        *int                    `json:"query_port"`
	RconPort         *int                    `json:"rcon_port"`
	Rcon             *string                 `json:"rcon"`
	Dir              string                  `json:"dir"`
	SuUser           *string                 `json:"su_user"`
	CPULimit         *int                    `json:"cpu_limit"`
	RAMLimit         *int                    `json:"ram_limit"`
	NetLimit         *int                    `json:"net_limit"`
	StartCommand     *string                 `json:"start_command"`
	StopCommand      *string                 `json:"stop_command"`
	ForceStopCommand *string                 `json:"force_stop_command"`
	RestartCommand   *string                 `json:"restart_command"`
	ProcessActive    bool                    `json:"process_active"`
	LastProcessCheck *string                 `json:"last_process_check"`
	Vars             *string                 `json:"vars"`
	CreatedAt        *string                 `json:"created_at"`
	UpdatedAt        *string                 `json:"updated_at"`
	DeletedAt        *string                 `json:"deleted_at"`
	Settings         []ServerSettingResponse `json:"settings"`
	Game             GameResponse            `json:"game"`
	GameMod          GameModResponse         `json:"game_mod"`
}

type ServerSettingResponse struct {
	ServerID uint   `json:"server_id"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

type GameResponse struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Engine            string  `json:"engine"`
	EngineVersion     string  `json:"engine_version"`
	RemoteRepository  *string `json:"remote_repository"`
	LocalRepository   *string `json:"local_repository"`
	SteamAppID        *uint   `json:"steam_app_id"`
	SteamAppSetConfig *string `json:"steam_app_set_config"`
}

type GameModResponse struct {
	ID                     uint                `json:"id"`
	GameCode               string              `json:"game_code"`
	Name                   string              `json:"name"`
	Vars                   []domain.GameModVar `json:"vars"`
	RemoteRepository       *string             `json:"remote_repository"`
	LocalRepository        *string             `json:"local_repository"`
	DefaultStartCmd        *string             `json:"default_start_cmd"`
	DefaultStartCmdLinux   *string             `json:"default_start_cmd_linux"`
	DefaultStartCmdWindows *string             `json:"default_start_cmd_windows"`
}

func newServerResponse(
	server *domain.Server,
	game *domain.Game,
	gameMod *domain.GameMod,
	settings []domain.ServerSetting,
	nodeOS domain.NodeOS,
) ServerResponse {
	var expires *string
	if server.Expires != nil {
		expiresStr := server.Expires.Format("2006-01-02T15:04:05.000000Z")
		expires = &expiresStr
	}

	var lastProcessCheck *string
	if server.LastProcessCheck != nil {
		lastProcessCheckStr := server.LastProcessCheck.Format("2006-01-02T15:04:05.000000Z")
		lastProcessCheck = &lastProcessCheckStr
	}

	var createdAt *string
	if server.CreatedAt != nil {
		createdAtStr := server.CreatedAt.Format("2006-01-02T15:04:05.000000Z")
		createdAt = &createdAtStr
	}

	var updatedAt *string
	if server.UpdatedAt != nil {
		updatedAtStr := server.UpdatedAt.Format("2006-01-02T15:04:05.000000Z")
		updatedAt = &updatedAtStr
	}

	var deletedAt *string
	if server.DeletedAt != nil {
		deletedAtStr := server.DeletedAt.Format("2006-01-02T15:04:05.000000Z")
		deletedAt = &deletedAtStr
	}

	settingsResponse := make([]ServerSettingResponse, 0, len(settings))
	for i := range settings {
		valueString, ok := settings[i].Value.String()
		if !ok {
			valueString = ""
		}

		settingsResponse = append(settingsResponse, ServerSettingResponse{
			ServerID: settings[i].ServerID,
			Name:     settings[i].Name,
			Value:    valueString,
		})
	}

	gameResponse := newGameResponse(game, nodeOS)
	gameModResponse := newGameModResponse(gameMod, nodeOS)

	return ServerResponse{
		ID:               server.ID,
		UUID:             server.UUID,
		UUIDShort:        server.UUIDShort,
		Enabled:          server.Enabled,
		Installed:        int(server.Installed),
		Blocked:          server.Blocked,
		Name:             server.Name,
		DSID:             server.DSID,
		GameID:           server.GameID,
		GameModID:        server.GameModID,
		Expires:          expires,
		ServerIP:         server.ServerIP,
		ServerPort:       server.ServerPort,
		QueryPort:        server.QueryPort,
		RconPort:         server.RconPort,
		Rcon:             server.Rcon,
		Dir:              server.Dir,
		SuUser:           server.SuUser,
		CPULimit:         server.CPULimit,
		RAMLimit:         server.RAMLimit,
		NetLimit:         server.NetLimit,
		StartCommand:     server.StartCommand,
		StopCommand:      server.StopCommand,
		ForceStopCommand: server.ForceStopCommand,
		RestartCommand:   server.RestartCommand,
		ProcessActive:    server.ProcessActive,
		LastProcessCheck: lastProcessCheck,
		Vars:             server.Vars,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		DeletedAt:        deletedAt,
		Settings:         settingsResponse,
		Game:             gameResponse,
		GameMod:          gameModResponse,
	}
}

func newGameResponse(game *domain.Game, nodeOS domain.NodeOS) GameResponse {
	var remoteRepository *string
	var localRepository *string
	var steamAppID *uint

	switch nodeOS {
	case domain.NodeOSLinux:
		remoteRepository = game.RemoteRepositoryLinux
		localRepository = game.LocalRepositoryLinux
		steamAppID = game.SteamAppIDLinux
	case domain.NodeOSWindows:
		remoteRepository = game.RemoteRepositoryWindows
		localRepository = game.LocalRepositoryWindows
		steamAppID = game.SteamAppIDWindows
	}

	return GameResponse{
		Code:              game.Code,
		Name:              game.Name,
		Engine:            game.Engine,
		EngineVersion:     game.EngineVersion,
		RemoteRepository:  remoteRepository,
		LocalRepository:   localRepository,
		SteamAppID:        steamAppID,
		SteamAppSetConfig: game.SteamAppSetConfig,
	}
}

func newGameModResponse(gameMod *domain.GameMod, nodeOS domain.NodeOS) GameModResponse {
	var remoteRepository *string
	var localRepository *string
	var defaultStartCmd *string

	switch nodeOS {
	case domain.NodeOSLinux:
		remoteRepository = gameMod.RemoteRepositoryLinux
		localRepository = gameMod.LocalRepositoryLinux
		defaultStartCmd = gameMod.StartCmdLinux
	case domain.NodeOSWindows:
		remoteRepository = gameMod.RemoteRepositoryWindows
		localRepository = gameMod.LocalRepositoryWindows
		defaultStartCmd = gameMod.StartCmdWindows
	}

	vars := make([]domain.GameModVar, 0)
	if gameMod.Vars != nil {
		vars = gameMod.Vars
	}

	return GameModResponse{
		ID:                     gameMod.ID,
		GameCode:               gameMod.GameCode,
		Name:                   gameMod.Name,
		Vars:                   vars,
		RemoteRepository:       remoteRepository,
		LocalRepository:        localRepository,
		DefaultStartCmd:        defaultStartCmd,
		DefaultStartCmdLinux:   gameMod.StartCmdLinux,
		DefaultStartCmdWindows: gameMod.StartCmdWindows,
	}
}
