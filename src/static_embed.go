package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// frontendFiles contains the accepted H5 build used by the single-binary server.
// The source files remain in frontend/ for development and are not embedded.
//
//go:embed webdist
var frontendFiles embed.FS

func embeddedFrontendHandler() http.Handler {
	webFS, err := fs.Sub(frontendFiles, "webdist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(webFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requestPath == "" || requestPath == "." {
			requestPath = "index.html"
		}
		if _, err := fs.Stat(webFS, requestPath); err != nil {
			requestPath = "index.html"
		}

		r.URL.Path = "/" + requestPath
		fileServer.ServeHTTP(w, r)
	})
}
