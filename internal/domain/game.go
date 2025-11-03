package domain

type Game struct {
	Code                    string  `db:"code"`                      // required, unique, slug, minlen=2, maxlen=16
	Name                    string  `db:"name"`                      // required, minlen=2, maxlen=128
	Engine                  string  `db:"engine"`                    // required, maxlen=128
	EngineVersion           string  `db:"engine_version"`            // maxlen=128
	SteamAppIDLinux         *uint   `db:"steam_app_id_linux"`        //
	SteamAppIDWindows       *uint   `db:"steam_app_id_windows"`      //
	SteamAppSetConfig       *string `db:"steam_app_set_config"`      // maxlen=128
	RemoteRepositoryLinux   *string `db:"remote_repository_linux"`   // maxlen=128
	RemoteRepositoryWindows *string `db:"remote_repository_windows"` // maxlen=128
	LocalRepositoryLinux    *string `db:"local_repository_linux"`    // maxlen=128
	LocalRepositoryWindows  *string `db:"local_repository_windows"`  // maxlen=128
	Enabled                 int     `db:"enabled"`                   //
}
