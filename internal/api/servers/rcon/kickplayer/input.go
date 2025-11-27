package kickplayer

import (
	"encoding/json"

	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

type playerInput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Score  string `json:"score"`
	Ping   string `json:"ping"`
	IP     string `json:"ip"`
	UniqID string `json:"uniqid"`
}

type kickRequest struct {
	Player            json.RawMessage `json:"player"`
	Reason            string          `json:"reason"`
	DurationInMinutes flexible.Int    `json:"time"`
}

func (r *kickRequest) Validate() error {
	if len(r.Player) == 0 {
		return errors.New("player is required")
	}

	return nil
}

func (r *kickRequest) ToPlayer() (players.Player, error) {
	var stringID string
	if err := json.Unmarshal(r.Player, &stringID); err == nil {
		return players.Player{
			ID:     stringID,
			UniqID: stringID,
		}, nil
	}

	var playerObj playerInput
	if err := json.Unmarshal(r.Player, &playerObj); err != nil {
		return players.Player{}, errors.New("player must be a string ID or player object")
	}

	player := players.Player{
		ID:     playerObj.ID,
		Name:   playerObj.Name,
		Score:  playerObj.Score,
		Ping:   playerObj.Ping,
		Addr:   playerObj.IP,
		UniqID: playerObj.UniqID,
	}

	if player.ID == "" {
		return players.Player{}, errors.New("player id is required")
	}

	if player.UniqID == "" {
		player.UniqID = player.ID
	}

	return player, nil
}
