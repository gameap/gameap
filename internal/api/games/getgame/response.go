package getgame

import "github.com/gameap/gameap/internal/domain"

type gameResponse struct {
	Code                    string  `json:"code"`
	Name                    string  `json:"name"`
	Engine                  string  `json:"engine"`
	EngineVersion           string  `json:"engine_version"`
	SteamAppSetConfig       *string `json:"steam_app_set_config"`
	SteamAppIDLinux         *uint   `json:"steam_app_id_linux"`
	RemoteRepositoryLinux   *string `json:"remote_repository_linux"`
	LocalRepositoryLinux    *string `json:"local_repository_linux"`
	SteamAppIDWindows       *uint   `json:"steam_app_id_windows"`
	RemoteRepositoryWindows *string `json:"remote_repository_windows"`
	LocalRepositoryWindows  *string `json:"local_repository_windows"`
	Enabled                 bool    `json:"enabled"`
}

func newGameResponseFromGame(g *domain.Game) gameResponse {
	return gameResponse{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppSetConfig:       g.SteamAppSetConfig,
		SteamAppIDLinux:         g.SteamAppIDLinux,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		SteamAppIDWindows:       g.SteamAppIDWindows,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Enabled:                 g.Enabled == 1,
	}
}
