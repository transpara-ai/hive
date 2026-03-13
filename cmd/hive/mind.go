package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/mind"
	"github.com/lovyou-ai/hive/pkg/pipeline"
)

// runMind starts an interactive chat session with the hive's mind.
func runMind(ctx context.Context, dsn, repoPath, model string) error {
	if dsn == "" {
		return fmt.Errorf("mind requires --store or DATABASE_URL (needs persistent state)")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer s.Close()

	// Register event types so the store can deserialize mind events.
	mind.RegisterEventTypes()
	pipeline.RegisterEventTypes()

	factory := event.NewEventFactory(event.DefaultRegistry())

	// Derive signer from "mind" identity — same pattern as bootstrapSigner.
	h := sha256.Sum256([]byte("signer:mind"))
	priv := ed25519.NewKeyFromSeed(h[:])
	signer := &mindSigner{key: priv}

	mindStore := mind.NewMindStore(s, factory, signer)

	// Load telemetry summary for the mind's context.
	telemetrySummary := loadTelemetrySummary(repoPath)

	provider, err := mind.CreateProvider(model)
	if err != nil {
		return fmt.Errorf("mind provider: %w", err)
	}

	m := mind.New(provider, mindStore, repoPath, telemetrySummary)

	// Show what the mind loaded.
	mindCtx := m.LoadContext()
	lines := strings.Count(mindCtx, "\n")
	fmt.Fprintf(os.Stderr, "Mind loaded: %d lines of context\n", lines)
	fmt.Fprintf(os.Stderr, "Type your message. Empty line or Ctrl+C to exit.\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "Matt> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			break
		}

		fmt.Fprintf(os.Stderr, "  ⏳ thinking...\n")
		resp, err := m.Chat(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			continue
		}
		fmt.Fprintf(os.Stdout, "\n%s\n\n", resp)
	}
	return nil
}

// loadTelemetrySummary reads telemetry and produces a summary string.
func loadTelemetrySummary(repoPath string) string {
	results, err := pipeline.ReadTelemetry(repoPath)
	if err != nil || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	n := len(results)
	start := 0
	if n > 10 {
		start = n - 10
	}
	for _, r := range results[start:] {
		mode := r.Mode
		if mode == "" {
			mode = "unknown"
		}
		status := "completed"
		if r.FailedPhase != "" {
			status = fmt.Sprintf("failed at %s", r.FailedPhase)
		}
		merged := ""
		if r.PRURL != "" {
			merged = fmt.Sprintf(" PR: %s", r.PRURL)
			if r.Merged {
				merged += " (merged)"
			}
		}
		var cost float64
		for _, u := range r.TokenUsage {
			cost += u.CostUSD
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s — %s, $%.2f%s\n",
			r.StartedAt.Format("2006-01-02 15:04"), mode, status, cost, merged))
	}
	sb.WriteString(fmt.Sprintf("\nTotal runs: %d\n", n))
	return sb.String()
}

// mindSigner signs mind events with a deterministic key.
type mindSigner struct {
	key ed25519.PrivateKey
}

func (s *mindSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}
