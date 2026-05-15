package main

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var embeddedWebDist embed.FS

// embeddedWebFS returns the embedded Vue SPA filesystem, or nil if empty.
func embeddedWebFS() fs.FS {
	// Check if the embedded dist has any files
	entries, err := fs.ReadDir(embeddedWebDist, "web/dist")
	if err != nil || len(entries) == 0 {
		return nil
	}
	sub, err := fs.Sub(embeddedWebDist, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}
