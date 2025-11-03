package domain

// GlobalAPIResponse represents the standard response structure from the GlobalAPI.
type GlobalAPIResponse[T any] struct {
	Data    T      `json:"data"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// GlobalAPIGame represents a game from the GlobalAPI.
type GlobalAPIGame struct {
	Code                    string             `json:"code"`
	StartCode               string             `json:"start_code"`
	Name                    string             `json:"name"`
	Engine                  string             `json:"engine"`
	EngineVersion           string             `json:"engine_version"`
	SteamAppIDLinux         uint               `json:"steam_app_id_linux"`
	SteamAppIDWindows       uint               `json:"steam_app_id_windows"`
	SteamAppSetConfig       string             `json:"steam_app_set_config"`
	RemoteRepositoryLinux   string             `json:"remote_repository_linux"`
	RemoteRepositoryWindows string             `json:"remote_repository_windows"`
	Mods                    []GlobalAPIGameMod `json:"mods"`
}

func (g *GlobalAPIGame) ToDomainGame() *Game {
	game := &Game{
		Code:          g.Code,
		Name:          g.Name,
		Engine:        g.Engine,
		EngineVersion: g.EngineVersion,
		Enabled:       1,
	}

	if g.SteamAppIDLinux != 0 {
		steamAppIDLinux := g.SteamAppIDLinux
		game.SteamAppIDLinux = &steamAppIDLinux
	}

	if g.SteamAppIDWindows != 0 {
		steamAppIDWindows := g.SteamAppIDWindows
		game.SteamAppIDWindows = &steamAppIDWindows
	}

	if g.SteamAppSetConfig != "" {
		steamAppSetConfig := g.SteamAppSetConfig
		game.SteamAppSetConfig = &steamAppSetConfig
	}

	if g.RemoteRepositoryLinux != "" {
		remoteRepositoryLinux := g.RemoteRepositoryLinux
		game.RemoteRepositoryLinux = &remoteRepositoryLinux
	}

	if g.RemoteRepositoryWindows != "" {
		remoteRepositoryWindows := g.RemoteRepositoryWindows
		game.RemoteRepositoryWindows = &remoteRepositoryWindows
	}

	return game
}

// GlobalAPIGameMod represents a game mod from the GlobalAPI.
type GlobalAPIGameMod struct {
	ID                      uint                `json:"id"`
	GameCode                string              `json:"game_code"`
	Name                    string              `json:"name"`
	FastRcon                GameModFastRconList `json:"fast_rcon"`
	Vars                    GameModVarList      `json:"vars"`
	RemoteRepositoryLinux   string              `json:"remote_repository_linux"`
	RemoteRepositoryWindows string              `json:"remote_repository_windows"`
	StartCmdLinux           string              `json:"start_cmd_linux"`
	StartCmdWindows         string              `json:"start_cmd_windows"`
	KickCmd                 string              `json:"kick_cmd"`
	BanCmd                  string              `json:"ban_cmd"`
	ChnameCmd               string              `json:"chname_cmd"`
	SrestartCmd             string              `json:"srestart_cmd"`
	ChmapCmd                string              `json:"chmap_cmd"`
	SendmsgCmd              string              `json:"sendmsg_cmd"`
	PasswdCmd               string              `json:"passwd_cmd"`
}

func (mod *GlobalAPIGameMod) ToDomainGameMod() *GameMod {
	gameMod := &GameMod{}

	gameMod.GameCode = mod.GameCode
	gameMod.Name = mod.Name
	gameMod.FastRcon = mod.FastRcon
	gameMod.Vars = mod.Vars

	if mod.RemoteRepositoryLinux != "" {
		remoteRepositoryLinux := mod.RemoteRepositoryLinux
		gameMod.RemoteRepositoryLinux = &remoteRepositoryLinux
	}

	if mod.RemoteRepositoryWindows != "" {
		remoteRepositoryWindows := mod.RemoteRepositoryWindows
		gameMod.RemoteRepositoryWindows = &remoteRepositoryWindows
	}

	if mod.StartCmdLinux != "" {
		startCmdLinux := mod.StartCmdLinux
		gameMod.StartCmdLinux = &startCmdLinux
	}

	if mod.StartCmdWindows != "" {
		startCmdWindows := mod.StartCmdWindows
		gameMod.StartCmdWindows = &startCmdWindows
	}

	if mod.KickCmd != "" {
		kickCmd := mod.KickCmd
		gameMod.KickCmd = &kickCmd
	}

	if mod.BanCmd != "" {
		banCmd := mod.BanCmd
		gameMod.BanCmd = &banCmd
	}

	if mod.ChnameCmd != "" {
		chnameCmd := mod.ChnameCmd
		gameMod.ChnameCmd = &chnameCmd
	}

	if mod.SrestartCmd != "" {
		srestartCmd := mod.SrestartCmd
		gameMod.SrestartCmd = &srestartCmd
	}

	if mod.ChmapCmd != "" {
		chmapCmd := mod.ChmapCmd
		gameMod.ChmapCmd = &chmapCmd
	}

	if mod.SendmsgCmd != "" {
		sendmsgCmd := mod.SendmsgCmd
		gameMod.SendmsgCmd = &sendmsgCmd
	}

	if mod.PasswdCmd != "" {
		passwdCmd := mod.PasswdCmd
		gameMod.PasswdCmd = &passwdCmd
	}

	return gameMod
}
