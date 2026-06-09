package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
)

func main() {
	addr := flag.String("addr", envOrDefault("HIVE_OPS_API_ADDR", "127.0.0.1:8083"), "listen address")
	apiKey := flag.String("api-key", envOrDefault("HIVE_OPS_API_KEY", "dev"), "bearer token for Site operator projection reads")
	limit := flag.Int("limit", 50, "maximum records per projection section")
	catalog := flag.String("catalog", envOrDefault("HIVE_OPS_CATALOG", ""), "custom YAML model catalog for operator projection (loaded once at startup)")
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

	modelSelection, err := hive.OperatorModelSelectionFromCatalogPath(*catalog, types.Now().Value())
	if err != nil {
		log.Fatalf("load model catalog: %v", err)
	}

	opts, writeMode := opsWriterOptions()
	opts = append(opts, hive.WithOperatorProjectionModelSelection(modelSelection))
	handler := hive.NewOperatorProjectionServer(store, *apiKey, *limit, opts...)

	authMode := "disabled"
	if *apiKey != "" {
		authMode = "bearer"
	}
	fmt.Printf("hive ops api listening on %s (auth=%s, limit=%d, writes=%s, model_catalog=%s, reload=static)\n", *addr, authMode, *limit, writeMode, modelSelection.CatalogSource)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

// opsWriterOptions provisions optional operator write routes from the
// environment. Writers are enabled only when HIVE_OPS_HUMAN_ACTOR is set;
// otherwise the server stays strictly read-only (today's behavior), so an
// unconfigured ops-api must not fail. The graph is still only ever written by
// hive — this process — never by Site.
//
//   - HIVE_OPS_HUMAN_ACTOR : the human actor id recorded as the approver/signer
//     of operator decisions and launch requests. Required to enable write paths.
//   - HIVE_OPS_SIGNING_KEY : optional explicit signing seed. When unset, the
//     signer is derived deterministically from the human actor id, matching the
//     hive runtime identity scheme (sha256("signer:"+id)).
func opsWriterOptions() ([]hive.OperatorServerOption, string) {
	human := os.Getenv("HIVE_OPS_HUMAN_ACTOR")
	if human == "" {
		return nil, "read-only"
	}
	humanID, err := types.NewActorID(human)
	if err != nil {
		log.Printf("HIVE_OPS_HUMAN_ACTOR invalid (%v); operator writes disabled, server read-only", err)
		return nil, "read-only"
	}

	registry := event.DefaultRegistry()
	hive.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)

	seed := "signer:" + humanID.Value()
	if key := os.Getenv("HIVE_OPS_SIGNING_KEY"); key != "" {
		seed = key
	}
	signer := newOpsSigner(seed)
	conv := types.MustConversationID("conv_hive_ops_api")

	return []hive.OperatorServerOption{
		hive.WithOperatorDecisionWriter(factory, signer, humanID, conv),
		hive.WithOperatorRunLaunchWriter(factory, signer, humanID, conv),
	}, "enabled"
}

// opsSigner is a deterministic Ed25519 signer for the ops-api decision writer,
// matching the hive runtime's signer scheme.
type opsSigner struct {
	key ed25519.PrivateKey
}

func (s *opsSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

func newOpsSigner(seed string) *opsSigner {
	h := sha256.Sum256([]byte(seed))
	return &opsSigner{key: ed25519.NewKeyFromSeed(h[:])}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
