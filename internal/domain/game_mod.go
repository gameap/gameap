package domain

import (
	"database/sql/driver"
	"encoding/json"
	"log/slog"

	"github.com/pkg/errors"
)

type GameMod struct {
	ID                      uint                `db:"id"`
	GameCode                string              `db:"game_code"`
	Name                    string              `db:"name"`
	FastRcon                GameModFastRconList `db:"fast_rcon"`
	Vars                    GameModVarList      `db:"vars"`
	RemoteRepositoryLinux   *string             `db:"remote_repository_linux"`
	RemoteRepositoryWindows *string             `db:"remote_repository_windows"`
	LocalRepositoryLinux    *string             `db:"local_repository_linux"`
	LocalRepositoryWindows  *string             `db:"local_repository_windows"`
	StartCmdLinux           *string             `db:"start_cmd_linux"`
	StartCmdWindows         *string             `db:"start_cmd_windows"`
	KickCmd                 *string             `db:"kick_cmd"`
	BanCmd                  *string             `db:"ban_cmd"`
	ChnameCmd               *string             `db:"chname_cmd"`
	SrestartCmd             *string             `db:"srestart_cmd"`
	ChmapCmd                *string             `db:"chmap_cmd"`
	SendmsgCmd              *string             `db:"sendmsg_cmd"`
	PasswdCmd               *string             `db:"passwd_cmd"`
}

func (gm *GameMod) Merge(other *GameMod) {
	if other.RemoteRepositoryLinux != nil {
		gm.RemoteRepositoryLinux = other.RemoteRepositoryLinux
	}

	if other.RemoteRepositoryWindows != nil {
		gm.RemoteRepositoryWindows = other.RemoteRepositoryWindows
	}

	if other.StartCmdLinux != nil {
		gm.StartCmdLinux = other.StartCmdLinux
	}

	if other.StartCmdWindows != nil {
		gm.StartCmdWindows = other.StartCmdWindows
	}

	if other.KickCmd != nil {
		gm.KickCmd = other.KickCmd
	}

	if other.BanCmd != nil {
		gm.BanCmd = other.BanCmd
	}

	if other.ChnameCmd != nil {
		gm.ChnameCmd = other.ChnameCmd
	}

	if other.SrestartCmd != nil {
		gm.SrestartCmd = other.SrestartCmd
	}

	if other.ChmapCmd != nil {
		gm.ChmapCmd = other.ChmapCmd
	}

	if other.SendmsgCmd != nil {
		gm.SendmsgCmd = other.SendmsgCmd
	}

	if other.PasswdCmd != nil {
		gm.PasswdCmd = other.PasswdCmd
	}

	gm.FastRcon = other.FastRcon
	gm.Vars = other.Vars
}

type GameModFastRcon struct {
	Info    string `json:"info"`
	Command string `json:"command"`
}

type GameModFastRconList []GameModFastRcon

func (f *GameModFastRconList) Scan(value any) error {
	if value == nil {
		*f = nil

		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, f)
}

func (f GameModFastRconList) Value() (driver.Value, error) {
	if f == nil {
		return nil, nil
	}

	return json.Marshal(f)
}

type GameModVar struct {
	Var      string            `json:"var"`
	Default  GameModVarDefault `json:"default"`
	Info     string            `json:"info"`
	AdminVar bool              `json:"admin_var"`
}

type GameModVarDefault string

func (gmvd GameModVarDefault) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(gmvd))
}

func (gmvd *GameModVarDefault) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*gmvd = GameModVarDefault(str)
	}

	// Try to unmarshal as number
	var num int
	if err := json.Unmarshal(data, &num); err == nil {
		*gmvd = GameModVarDefault(rune(num))
	}

	return nil
}

type GameModVarList []GameModVar

func (g *GameModVarList) Scan(value any) error {
	if value == nil {
		*g = nil

		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	err := json.Unmarshal(bytes, g)
	if err != nil {
		// If unmarshaling into a slice fails, try unmarshaling into a single object
		singleVar := GameModVar{}
		err2 := json.Unmarshal(bytes, &singleVar)
		if err2 != nil {
			// Return the original error if both unmarshaling attempts fail
			return errors.WithMessage(err, "failed to unmarshal game mod vars")
		}

		slog.Warn(
			"GameModVarList: received single object instead of array, wrapping into array",
			"value", bytes,
		)

		*g = []GameModVar{singleVar}
	}

	return nil
}

func (g GameModVarList) Value() (driver.Value, error) {
	if g == nil {
		return nil, nil
	}

	return json.Marshal(g)
}
