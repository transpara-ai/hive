package mcp

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// AuthorityChecker validates that an agent has sufficient trust to perform
// write operations. Read tools are unrestricted. Write tools require the
// agent's trust score to meet a minimum threshold.
type AuthorityChecker struct {
	actors     actor.IActorStore
	trustModel *trust.DefaultTrustModel
	agentID    types.ActorID
	minTrust   float64 // minimum trust score for write tools (0.0 = allow all)
}

// NewAuthorityChecker creates an authority checker. If minTrust is 0, all
// write operations are allowed (useful during bootstrap when trust is 0.1).
func NewAuthorityChecker(actors actor.IActorStore, trustModel *trust.DefaultTrustModel, agentID types.ActorID, minTrust float64) *AuthorityChecker {
	return &AuthorityChecker{
		actors:     actors,
		trustModel: trustModel,
		agentID:    agentID,
		minTrust:   minTrust,
	}
}

// CheckWrite verifies the agent has authority for a write operation.
// Returns nil if authorized, or an error describing why not.
func (c *AuthorityChecker) CheckWrite(action string) error {
	if c.minTrust <= 0 {
		return nil // no trust gate during bootstrap
	}

	a, err := c.actors.Get(c.agentID)
	if err != nil {
		return fmt.Errorf("authority check: agent not found: %w", err)
	}

	// Check actor status — suspended agents cannot write.
	if string(a.Status()) == "suspended" {
		return fmt.Errorf("authority denied: agent %s is suspended", c.agentID.Value())
	}

	metrics, err := c.trustModel.Score(nil, a)
	if err != nil {
		return fmt.Errorf("authority check: trust error: %w", err)
	}

	if metrics.Overall().Value() < c.minTrust {
		return fmt.Errorf("authority denied: trust %.2f < required %.2f for %s",
			metrics.Overall().Value(), c.minTrust, action)
	}

	return nil
}

// WrapWriteHandler wraps a handler with authority checking.
// If the check fails, returns an error result without calling the handler.
func WrapWriteHandler(checker *AuthorityChecker, action string, handler Handler) Handler {
	if checker == nil {
		return handler
	}
	return func(args map[string]any) (ToolCallResult, error) {
		if err := checker.CheckWrite(action); err != nil {
			return ErrorResult(err.Error()), nil
		}
		return handler(args)
	}
}
