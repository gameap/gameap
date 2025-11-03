package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var frontendFS embed.FS

func GetFS() (fs.FS, error) {
	return fs.Sub(frontendFS, "dist")
}
