package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/config"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/jobs"
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

	// Start background job workers.
	queue := h.GetQueue()
	worker := jobs.NewWorker(queue, cfg.QueueWorkers)
	worker.Start(context.Background())
	defer worker.Stop()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("shutdown signal received, gracefully stopping...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
		worker.Stop()
	}()

	log.Printf("i-confess backend listening on :%s (env=%s, workers=%d)", cfg.Port, cfg.Env, cfg.QueueWorkers)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
