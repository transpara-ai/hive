// Command mcp-server is an MCP (Model Context Protocol) server that exposes
// the event graph, actor store, trust model, and workspace as tools.
// It communicates over stdio using JSON-RPC 2.0.
//
// Claude CLI spawns this as a subprocess — agents call tools mid-reasoning.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore/pgstate"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := flag.String("store", "", "Postgres connection string (required)")
	agentID := flag.String("agent-id", "", "This agent's ActorID (required)")
	humanID := flag.String("human-id", "", "The human operator's ActorID (required)")
	convID := flag.String("conv-id", "conv_mcp_000000000000000000000001", "Conversation ID for emitted events")
	flag.Parse()

	// Also check env vars (Claude CLI passes env to subprocesses).
	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
	}
	if *agentID == "" {
		*agentID = os.Getenv("HIVE_AGENT_ID")
	}
	if *humanID == "" {
		*humanID = os.Getenv("HIVE_HUMAN_ID")
	}
	if *convID == "" {
		*convID = os.Getenv("HIVE_CONV_ID")
	}

	if *dsn == "" {
		return fmt.Errorf("--store or DATABASE_URL is required (MCP server needs persistent storage)")
	}
	if *agentID == "" {
		return fmt.Errorf("--agent-id or HIVE_AGENT_ID is required")
	}
	if *humanID == "" {
		return fmt.Errorf("--human-id or HIVE_HUMAN_ID is required")
	}

	ctx := context.Background()

	// Open shared pool.
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	// Open stores from shared pool.
	s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("event store: %w", err)
	}
	defer s.Close()

	actors, err := pgactor.NewPostgresActorStoreFromPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("actor store: %w", err)
	}

	states, err := pgstate.NewPostgresStateStoreFromPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}

	// Load trust model from state store.
	trustModel := trust.NewDefaultTrustModel()
	if data, err := states.Get("trust", "model"); err == nil && data != nil {
		trustModel.ImportJSON(data)
	}

	cid, err := types.NewConversationID(*convID)
	if err != nil {
		return fmt.Errorf("invalid conversation ID: %w", err)
	}

	// Create and run MCP server.
	server := mcp.NewServer("hive-mcp", "0.1.0")
	mcp.RegisterAllTools(server, mcp.Deps{
		Store:   s,
		Actors:  actors,
		States:  states,
		Trust:   trustModel,
		AgentID: types.MustActorID(*agentID),
		HumanID: types.MustActorID(*humanID),
		ConvID:  cid,
	})

	return server.Run()
}

// Unused but available for future use — in-memory mode isn't practical for
// the MCP server (separate process can't share memory with the pipeline).
var _ store.Store = nil
var _ actor.IActorStore = nil
var _ statestore.IStateStore = nil
