package web

import (
	"embed"
	"fmt"
	"io/fs"
)

// Dist contains the built frontend files from web/dist.
//
//go:embed all:dist
var assets embed.FS

func Dist() (fs.FS, error) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("sub dist: %w", err)
	}

	return sub, nil
}
