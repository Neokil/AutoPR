package web

import (
	"embed"
	"io/fs"
)

// Dist contains the built frontend files from web/dist.
//
//go:embed all:dist
var assets embed.FS

func Dist() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
