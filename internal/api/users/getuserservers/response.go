package getuserservers

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

type gameModResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type serverResponse struct {
	ID         uint             `json:"id"`
	UUID       string           `json:"uuid"`
	UUIDShort  string           `json:"uuid_short"`
	Enabled    bool             `json:"enabled"`
	Installed  int              `json:"installed"`
	Blocked    bool             `json:"blocked"`
	Name       string           `json:"name"`
	GameID     string           `json:"game_id"`
	GameModID  uint             `json:"game_mod_id"`
	Game       *gameResponse    `json:"game,omitempty"`
	GameMod    *gameModResponse `json:"game_mod,omitempty"`
	DSID       uint             `json:"ds_id"`
	Expires    *time.Time       `json:"expires"`
	ServerIP   string           `json:"server_ip"`
	ServerPort int              `json:"server_port"`
	QueryPort  *int             `json:"query_port"`
	RconPort   *int             `json:"rcon_port"`
}

func newServersResponseFromServers(
	servers []domain.Server,
	games []domain.Game,
	gameMods []domain.GameMod,
) []serverResponse {
	gameMap := make(map[string]domain.Game, len(games))
	for _, g := range games {
		gameMap[g.Code] = g
	}

	gameModMap := make(map[uint]domain.GameMod, len(gameMods))
	for _, gm := range gameMods {
		gameModMap[gm.ID] = gm
	}

	response := make([]serverResponse, 0, len(servers))

	for _, s := range servers {
		response = append(response, newServerResponseFromServer(&s, gameMap, gameModMap))
	}

	return response
}

func newServerResponseFromServer(
	s *domain.Server,
	gameMap map[string]domain.Game,
	gameModMap map[uint]domain.GameMod,
) serverResponse {
	sr := serverResponse{
		ID:         s.ID,
		UUID:       s.UUID.String(),
		UUIDShort:  s.UUIDShort,
		Enabled:    s.Enabled,
		Installed:  int(s.Installed),
		Blocked:    s.Blocked,
		Name:       s.Name,
		GameID:     s.GameID,
		GameModID:  s.GameModID,
		DSID:       s.DSID,
		Expires:    s.Expires,
		ServerIP:   s.ServerIP,
		ServerPort: s.ServerPort,
		QueryPort:  s.QueryPort,
		RconPort:   s.RconPort,
	}

	if game, exists := gameMap[s.GameID]; exists {
		sr.Game = &gameResponse{
			Code:          game.Code,
			Name:          game.Name,
			Engine:        game.Engine,
			EngineVersion: game.EngineVersion,
		}
	}

	if gameMod, exists := gameModMap[s.GameModID]; exists {
		sr.GameMod = &gameModResponse{
			ID:   gameMod.ID,
			Name: gameMod.Name,
		}
	}

	return sr
}
