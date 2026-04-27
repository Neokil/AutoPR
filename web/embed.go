// Package web embeds the compiled frontend assets for serving via the AutoPR HTTP server.
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

// Dist returns an fs.FS rooted at the embedded web/dist directory.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("sub dist: %w", err)
	}

	return sub, nil
}
