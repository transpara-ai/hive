package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lovyou-ai/hive/pkg/localapi"
)

func main() {
	addr := flag.String("addr", ":8082", "listen address")
	apiKey := flag.String("api-key", "dev", "API key for authentication")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	if err := localapi.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	store := localapi.NewStore(db)
	handler := localapi.NewServer(store, *apiKey)

	fmt.Printf("localapi listening on %s (key=%s)\n", *addr, *apiKey)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
