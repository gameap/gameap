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
	Metadata                Metadata            `db:"metadata"`
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

	gm.FastRcon = mergeFastRcon(gm.FastRcon, other.FastRcon)
	gm.Vars = mergeVars(gm.Vars, other.Vars)
}

// mergeVars lets a catalog entry replace the local variable of the same name
// while keeping variables an administrator added by hand. Catalog order comes
// first so an upgrade still controls how the settings page is laid out.
func mergeVars(local, incoming GameModVarList) GameModVarList {
	if len(incoming) == 0 {
		return local
	}

	incomingNames := make(map[string]struct{}, len(incoming))
	for _, v := range incoming {
		incomingNames[v.Var] = struct{}{}
	}

	merged := make(GameModVarList, 0, len(incoming)+len(local))
	merged = append(merged, incoming...)

	for _, v := range local {
		if _, exists := incomingNames[v.Var]; !exists {
			merged = append(merged, v)
		}
	}

	return merged
}

// mergeFastRcon follows mergeVars, keyed by the command itself: two buttons
// running the same command are the same button.
func mergeFastRcon(local, incoming GameModFastRconList) GameModFastRconList {
	if len(incoming) == 0 {
		return local
	}

	incomingCommands := make(map[string]struct{}, len(incoming))
	for _, fr := range incoming {
		incomingCommands[fr.Command] = struct{}{}
	}

	merged := make(GameModFastRconList, 0, len(incoming)+len(local))
	merged = append(merged, incoming...)

	for _, fr := range local {
		if _, exists := incomingCommands[fr.Command]; !exists {
			merged = append(merged, fr)
		}
	}

	return merged
}

type GameModFastRcon struct {
	Info    string              `json:"info"           yaml:"info"`
	Command string              `json:"command"        yaml:"command"`
	I18n    GameModFastRconI18n `json:"i18n,omitempty" yaml:"i18n,omitempty"`
}

func (f *GameModFastRcon) Normalize() {
	f.I18n = f.I18n.Normalized()
}

type GameModFastRconList []GameModFastRcon

func (f *GameModFastRconList) Scan(value any) error {
	bytes, ok := jsonColumnBytes(value)
	if !ok {
		*f = nil

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

type GameModVarList []GameModVar

func (g *GameModVarList) Scan(value any) error {
	bytes, ok := jsonColumnBytes(value)
	if !ok {
		*g = nil

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

		// slog.Warn is a side-effect, not under direct test coverage.
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

// jsonColumnBytes accepts both driver shapes for a JSON column: []byte from the
// postgres and mysql drivers, string from sqlite.
func jsonColumnBytes(value any) ([]byte, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case []byte:
		if len(v) == 0 {
			return nil, false
		}

		return v, true
	case string:
		if v == "" {
			return nil, false
		}

		return []byte(v), true
	default:
		return nil, false
	}
}
