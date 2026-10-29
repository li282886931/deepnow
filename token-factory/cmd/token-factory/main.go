package main

import (
	"log"
	"net/http"
	"os"

	"github.com/li282886931/deepnow/token-factory/internal/app"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		log.Printf("token-factory configuration error: %v", err)
		os.Exit(1)
	}

	server, cleanup, err := app.NewServer(cfg)
	if err != nil {
		log.Printf("token-factory server setup error: %v", err)
		os.Exit(1)
	}
	defer func() {
		if err := cleanup(); err != nil {
			log.Printf("token-factory cleanup error: %v", err)
		}
	}()

	log.Printf("token-factory listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("token-factory server error: %v", err)
		os.Exit(1)
	}
}
