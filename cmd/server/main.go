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
	"github.com/Tesquo/legendary-octo-memory/internal/ffmpeg"
	"github.com/Tesquo/legendary-octo-memory/internal/storage"
)

func main() {
	cfg := config.Load()

	// State whether the media toolchain could be found, once, up front. The
	// search has an order and the answer is fixed for the process (see
	// internal/ffmpeg), so this is the only place it needs saying; without it a
	// missing ffmpeg turns into an unexplained failure on every upload.
	if err := ffmpeg.Verify(); err != nil {
		log.Printf("warning: %v", err)
	}

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

		// Bound how long a client may spend sending request headers, and how
		// long an idle connection is kept. ReadTimeout is deliberately absent:
		// one upload can legitimately take minutes, and the body is already
		// capped by MaxUploadBytes. WriteTimeout is absent for the same reason —
		// /progress is a long-lived event stream and /play serves large ranged
		// responses, and a write deadline would cut both off mid-flight.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
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
