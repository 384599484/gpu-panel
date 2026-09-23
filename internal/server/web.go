package server

import (
	"embed"
	"io/fs"
)

//go:embed web
var webFS embed.FS

func webFiles() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
