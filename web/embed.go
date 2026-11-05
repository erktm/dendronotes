// Package web embeds the built frontend (web/dist) into the binary.
//
// dist/.keep makes sure the package compiles even without a frontend build;
// the server then answers UI requests with 503 and a hint.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the contents of web/dist as a file system root.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // only possible with a wrong path in the code
	}
	return sub
}
