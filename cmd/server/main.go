package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/config"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/seed"
)

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()

	// Seed dev content when the database is empty (development only).
	if cfg.Env == "development" || os.Getenv("SEED") == "1" {
		if err := seed.Seed(conn); err != nil {
			log.Printf("seed: %v", err)
		}
	}

	h := api.NewHandler(api.Config{JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL}, conn)
	h.BuildEngine()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("i-confess backend listening on :%s (env=%s)", cfg.Port, cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
