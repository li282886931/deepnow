package app

import (
	"net/http"

	tokenhttp "github.com/li282886931/deepnow/token-factory/internal/http"
)

func NewServer(cfg Config) (*http.Server, func() error, error) {
	mux := http.NewServeMux()
	tokenhttp.RegisterHealthz(mux)

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}
	return server, func() error { return nil }, nil
}
