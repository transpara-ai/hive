package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/hive/pkg/hive"
)

func main() {
	addr := flag.String("addr", envOrDefault("HIVE_OPS_API_ADDR", ":8083"), "listen address")
	apiKey := flag.String("api-key", os.Getenv("HIVE_OPS_API_KEY"), "bearer token for Site operator projection reads")
	limit := flag.Int("limit", 50, "maximum records per projection section")
	flag.Parse()

	dsn := envOrDefault("HIVE_OPS_DATABASE_URL", os.Getenv("DATABASE_URL"))
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	store, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		log.Fatalf("open eventgraph store: %v", err)
	}
	defer store.Close()

	hive.RegisterEventTypes()
	handler := hive.NewOperatorProjectionServer(store, *apiKey, *limit)

	authMode := "disabled"
	if *apiKey != "" {
		authMode = "bearer"
	}
	fmt.Printf("hive ops api listening on %s (auth=%s, limit=%d)\n", *addr, authMode, *limit)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
