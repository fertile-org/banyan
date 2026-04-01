// Package webdist embeds the built web dashboard static files.
//
// The static/ directory is populated by `make web-build` before building
// the CLI binary. If static/ is empty or missing, the CLI builds without
// the web dashboard (the --web flag will show an error).
package webdist

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var files embed.FS

// FS returns the embedded web dashboard files as an fs.FS rooted at the
// static/ directory. Returns nil if no files were embedded.
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		return nil
	}
	// Check if there's at least an index.html
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
