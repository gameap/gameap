package i18n

import "embed"

//go:embed *.json
var embedFS embed.FS

func GetFS() embed.FS {
	return embedFS
}
