package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

func Handler() http.Handler {
	subtree, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(subtree))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(subtree, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
