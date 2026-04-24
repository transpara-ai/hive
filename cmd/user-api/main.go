// Command user-api starts the user management HTTP server.
//
// Configuration via environment:
//
//	PORT         – listen port (default "8080")
//	DATABASE_URL – Postgres DSN; empty = in-memory store
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/transpara-ai/hive/pkg/userapi"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "user-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dsn := os.Getenv("DATABASE_URL")

	store, err := openStore(dsn)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	mux := userapi.NewMux(store)
	handler := userapi.Chain(mux,
		userapi.RecoverMiddleware,
		userapi.LogMiddleware,
	)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background.
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	log.Printf("user-api listening on :%s (store=%s)", port, storeKind(dsn))

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("received %v, draining...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// openStore selects the store backend based on DSN.
func openStore(dsn string) (userapi.Store, error) {
	if dsn == "" {
		return userapi.NewMemStore(), nil
	}
	// TODO: return userapi.NewPGStore(dsn) when postgres store is implemented.
	return nil, fmt.Errorf("postgres store not yet implemented (dsn=%q)", dsn)
}

func storeKind(dsn string) string {
	if dsn == "" {
		return "in-memory"
	}
	return "postgres"
}
