// adjust-budget emits an agent.budget.adjusted event for a named agent.
// Usage: go run ./cmd/adjust-budget --agent cto --budget 50 --store postgres://hive:hive@localhost:5432/hive
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/graph"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/hive"
	"github.com/lovyou-ai/work"
)

func main() {
	agent := flag.String("agent", "", "Agent name (required)")
	budget := flag.Int("budget", 0, "New max iterations (required)")
	dsn := flag.String("store", "", "Postgres DSN (or set DATABASE_URL)")
	reason := flag.String("reason", "Human operator adjusted budget via CLI", "Adjustment reason")
	flag.Parse()

	if *agent == "" || *budget == 0 {
		fmt.Fprintln(os.Stderr, "usage: adjust-budget --agent <name> --budget <N> [--store postgres://...]")
		os.Exit(1)
	}

	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
	}
	if *dsn == "" {
		log.Fatal("no store DSN: use --store or DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	hive.RegisterEventTypes()
	work.RegisterEventTypes()

	s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		log.Fatalf("start graph: %v", err)
	}
	defer g.Close()

	humanID := types.MustActorID("actor_4398821bf5b937fc51b0fb7117ab9c5c")

	head, err := s.Head()
	if err != nil {
		log.Fatalf("get head: %v", err)
	}
	var causes []types.EventID
	if head.IsSome() {
		causes = []types.EventID{head.Unwrap().ID()}
	}

	seed := sha256.Sum256([]byte(humanID.String()))
	priv := ed25519.NewKeyFromSeed(seed[:])
	signer := &cliSigner{key: priv}

	content := event.AgentBudgetAdjustedContent{
		AgentID:   humanID,
		AgentName: *agent,
		Action:    "set",
		NewBudget: *budget,
		Reason:    *reason,
	}

	ev, err := g.Record(
		types.MustEventType("agent.budget.adjusted"),
		humanID,
		content,
		causes,
		types.MustConversationID("cli-adjust-budget"),
		signer,
	)
	if err != nil {
		log.Fatalf("record failed: %v", err)
	}

	fmt.Printf("emitted agent.budget.adjusted for %q (budget=%d, event %s)\n", *agent, *budget, ev.ID())
}

type cliSigner struct {
	key ed25519.PrivateKey
}

func (s *cliSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}
