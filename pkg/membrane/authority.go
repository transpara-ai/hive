package membrane

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ActionDecision records a human decision on a pending action.
type ActionDecision struct {
	Decision  string // "approved", "rejected", "edited", "redirected", "auto_approved"
	DecidedBy string // actor ID of the human, or "system" for auto
	Notes     string
}

// AuthorityGate manages trust-based authority checks for membrane actions.
type AuthorityGate struct {
	bands              TrustBands
	RecommendedTimeout time.Duration

	mu      sync.Mutex
	pending map[string]chan ActionDecision
}

// NewAuthorityGate creates a gate with the given trust bands.
func NewAuthorityGate(bands TrustBands) *AuthorityGate {
	return &AuthorityGate{
		bands:              bands,
		RecommendedTimeout: 15 * time.Minute,
		pending:            make(map[string]chan ActionDecision),
	}
}

// Gate checks authority for an action and blocks if approval is required.
func (g *AuthorityGate) Gate(ctx context.Context, actionID string, trustScore float64, summary string) (ActionDecision, error) {
	level := g.bands.AuthorityFor(trustScore)

	switch level {
	case AuthNotification:
		return ActionDecision{Decision: "auto_approved", DecidedBy: "system"}, nil

	case AuthRecommended:
		ch := g.register(actionID)
		defer g.unregister(actionID)

		select {
		case d := <-ch:
			return d, nil
		case <-time.After(g.RecommendedTimeout):
			return ActionDecision{Decision: "auto_approved", DecidedBy: "system", Notes: "recommended timeout"}, nil
		case <-ctx.Done():
			return ActionDecision{}, fmt.Errorf("authority gate cancelled: %w", ctx.Err())
		}

	case AuthRequired:
		ch := g.register(actionID)
		defer g.unregister(actionID)

		select {
		case d := <-ch:
			return d, nil
		case <-ctx.Done():
			return ActionDecision{}, fmt.Errorf("authority gate cancelled: %w", ctx.Err())
		}
	}

	return ActionDecision{}, fmt.Errorf("unknown authority level")
}

// Resolve provides a human decision for a pending action.
func (g *AuthorityGate) Resolve(actionID string, decision ActionDecision) {
	g.mu.Lock()
	ch, ok := g.pending[actionID]
	g.mu.Unlock()

	if ok {
		select {
		case ch <- decision:
		default:
		}
	}
}

func (g *AuthorityGate) register(actionID string) chan ActionDecision {
	ch := make(chan ActionDecision, 1)
	g.mu.Lock()
	g.pending[actionID] = ch
	g.mu.Unlock()
	return ch
}

func (g *AuthorityGate) unregister(actionID string) {
	g.mu.Lock()
	delete(g.pending, actionID)
	g.mu.Unlock()
}
