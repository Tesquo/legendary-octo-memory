package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tesquo/legendary-octo-memory/config"
	"github.com/Tesquo/legendary-octo-memory/internal/api"
	"github.com/Tesquo/legendary-octo-memory/internal/storage"
)

func main() {
	cfg := config.Load()

	db, err := storage.InitDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// A small worker pool (configurable via WORKER_COUNT) lets several files
	// prepare concurrently without saturating the machine.
	server := api.NewServer(db, cfg)
	defer server.Pipeline.Close()

	router := api.NewRouter(server)

	// Bind to loopback by default. The API is unauthenticated, and by design
	// only the host's own browser talks to it, so the safe default is that no
	// other machine on the network can reach it. Set HOST=0.0.0.0 to opt in.
	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Run the server in the background so we can wait for a shutdown signal.
	go func() {
		log.Printf("Server running on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Block until Ctrl+C / SIGTERM, then shut down gracefully. This also lets
	// in-flight ffmpeg jobs finish rather than orphaning child processes.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
