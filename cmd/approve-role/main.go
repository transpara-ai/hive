// approve-role emits a hive.role.approved event for a named role.
// Usage: go run ./cmd/approve-role --role researcher --store postgres://hive:hive@localhost:5432/hive
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
	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/work"
)

func main() {
	role := flag.String("role", "", "Role name to approve (required)")
	dsn := flag.String("store", "", "Postgres DSN (or set DATABASE_URL)")
	reason := flag.String("reason", "Human operator approved via CLI", "Approval reason")
	flag.Parse()

	if *role == "" {
		fmt.Fprintln(os.Stderr, "usage: approve-role --role <name> [--store postgres://...]")
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

	// Derive a deterministic signer from the human actor ID (same as hive CLI mode).
	seed := sha256.Sum256([]byte(humanID.String()))
	priv := ed25519.NewKeyFromSeed(seed[:])
	signer := &cliSigner{key: priv}
	convID := types.MustConversationID("cli-approve")

	// 1. Emit hive.role.approved
	approveContent := event.RoleApprovedContent{
		Name:       *role,
		ApprovedBy: "human",
		Reason:     *reason,
	}

	ev, err := g.Record(
		types.MustEventType("hive.role.approved"),
		humanID,
		approveContent,
		causes,
		convID,
		signer,
	)
	if err != nil {
		log.Fatalf("record approval failed: %v", err)
	}
	fmt.Printf("emitted hive.role.approved for %q (event %s)\n", *role, ev.ID())

	// 2. Emit agent.budget.adjusted so the watcher can spawn the agent.
	// AgentID must be non-empty or the store can't deserialize the head event.
	budgetContent := event.AgentBudgetAdjustedContent{
		AgentID:   humanID,
		AgentName: *role,
		Action:    "set",
		NewBudget: 200,
		Reason:    "Initial budget for newly approved role",
	}

	budgetEv, err := g.Record(
		types.MustEventType("agent.budget.adjusted"),
		humanID,
		budgetContent,
		[]types.EventID{ev.ID()},
		convID,
		signer,
	)
	if err != nil {
		log.Fatalf("record budget failed: %v", err)
	}
	fmt.Printf("emitted agent.budget.adjusted for %q (budget=200, event %s)\n", *role, budgetEv.ID())
}

type cliSigner struct {
	key ed25519.PrivateKey
}

func (s *cliSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}
