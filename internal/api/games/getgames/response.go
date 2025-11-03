package getgames

import "github.com/gameap/gameap/internal/domain"

type gameResponse struct {
	Code                    string  `json:"code"`
	Name                    string  `json:"name"`
	Engine                  string  `json:"engine"`
	EngineVersion           string  `json:"engine_version"`
	SteamAppIDLinux         *uint   `json:"steam_app_id_linux"`
	SteamAppIDWindows       *uint   `json:"steam_app_id_windows"`
	SteamAppSetConfig       *string `json:"steam_app_set_config"`
	RemoteRepositoryLinux   *string `json:"remote_repository_linux"`
	RemoteRepositoryWindows *string `json:"remote_repository_windows"`
	LocalRepositoryLinux    *string `json:"local_repository_linux"`
	LocalRepositoryWindows  *string `json:"local_repository_windows"`
	Enabled                 int     `json:"enabled"`
}

func newGamesResponseFromGames(games []domain.Game) []gameResponse {
	response := make([]gameResponse, 0, len(games))

	for _, g := range games {
		response = append(response, newGameResponseFromGame(&g))
	}

	return response
}

func newGameResponseFromGame(g *domain.Game) gameResponse {
	return gameResponse{
		Code:                    g.Code,
		Name:                    g.Name,
		Engine:                  g.Engine,
		EngineVersion:           g.EngineVersion,
		SteamAppSetConfig:       g.SteamAppSetConfig,
		SteamAppIDLinux:         g.SteamAppIDLinux,
		SteamAppIDWindows:       g.SteamAppIDWindows,
		RemoteRepositoryLinux:   g.RemoteRepositoryLinux,
		RemoteRepositoryWindows: g.RemoteRepositoryWindows,
		LocalRepositoryLinux:    g.LocalRepositoryLinux,
		LocalRepositoryWindows:  g.LocalRepositoryWindows,
		Enabled:                 g.Enabled,
	}
}
