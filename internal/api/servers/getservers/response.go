package getservers

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type gameResponse struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
}

type serverResponse struct {
	ID               uint          `json:"id"`
	Enabled          bool          `json:"enabled"`
	Installed        int           `json:"installed"`
	Blocked          bool          `json:"blocked"`
	Name             string        `json:"name"`
	GameID           string        `json:"game_id"`
	DSID             uint          `json:"ds_id"`
	GameModID        uint          `json:"game_mod_id"`
	Expires          *time.Time    `json:"expires"`
	ServerIP         string        `json:"server_ip"`
	ServerPort       int           `json:"server_port"`
	QueryPort        *int          `json:"query_port"`
	RconPort         *int          `json:"rcon_port"`
	ProcessActive    bool          `json:"process_active"`
	LastProcessCheck *time.Time    `json:"last_process_check"`
	Game             *gameResponse `json:"game"`
	Online           bool          `json:"online"`
}

func newServersResponseFromServers(servers []domain.Server, games []domain.Game) []serverResponse {
	// Create a map of games by code for quick lookup
	gamesByCode := make(map[string]*domain.Game)
	for i := range games {
		gamesByCode[games[i].Code] = &games[i]
	}

	response := make([]serverResponse, 0, len(servers))

	for _, s := range servers {
		response = append(response, newServerResponseFromServer(&s, gamesByCode))
	}

	return response
}

func newServerResponseFromServer(s *domain.Server, gamesByCode map[string]*domain.Game) serverResponse {
	resp := serverResponse{
		ID:               s.ID,
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
		ProcessActive:    s.ProcessActive,
		LastProcessCheck: s.LastProcessCheck,
		Online:           s.IsOnline(),
	}

	// Add game information if available
	if game, ok := gamesByCode[s.GameID]; ok {
		resp.Game = &gameResponse{
			Code:          game.Code,
			Name:          game.Name,
			Engine:        game.Engine,
			EngineVersion: game.EngineVersion,
		}
	}

	return resp
}
