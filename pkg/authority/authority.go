// Package authority implements the three-tier approval model for hive actions.
//
// Every significant action goes through an authority gate:
//   - Required:     blocks until human approves or rejects
//   - Recommended:  auto-approves after timeout (default 15 min)
//   - Notification: auto-approves immediately, logged for audit
//
// Authority requests and resolutions are events on the graph.
package authority

import (
	"fmt"
	"sync"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Request represents a pending authority request.
type Request struct {
	ID            types.EventID        // the authority.requested event ID
	Action        string               // what is being requested
	Actor         types.ActorID        // who is requesting
	Level         event.AuthorityLevel // Required, Recommended, or Notification
	Justification string
	CreatedAt     time.Time
}

// Resolution is the outcome of an authority request.
type Resolution struct {
	RequestID types.EventID
	Approved  bool
	Resolver  types.ActorID // who resolved (human or timeout)
	Reason    string
}

// Approver is called to get a human decision on a Required authority request.
// Implementations block until the human responds. Returns approved and reason.
type Approver func(req Request) (approved bool, reason string)

// Gate evaluates authority requests against the three-tier model.
// Not safe for concurrent use.
type Gate struct {
	mu              sync.Mutex
	pending         map[types.EventID]Request
	approver        Approver
	recommendedWait time.Duration // timeout for Recommended level
}

// NewGate creates an authority gate. The approver handles Required-level requests.
// If approver is nil, Required requests block forever (useful for testing).
func NewGate(approver Approver) *Gate {
	return &Gate{
		pending:         make(map[types.EventID]Request),
		approver:        approver,
		recommendedWait: 15 * time.Minute,
	}
}

// SetRecommendedTimeout overrides the auto-approve timeout for Recommended level.
func (g *Gate) SetRecommendedTimeout(d time.Duration) {
	g.recommendedWait = d
}

// Check evaluates an authority request and returns its resolution.
// For Required: calls the approver (blocking). For Recommended: auto-approves
// after timeout. For Notification: auto-approves immediately.
func (g *Gate) Check(req Request) Resolution {
	g.mu.Lock()
	g.pending[req.ID] = req
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.pending, req.ID)
		g.mu.Unlock()
	}()

	switch req.Level {
	case event.AuthorityLevelRequired:
		return g.checkRequired(req)
	case event.AuthorityLevelRecommended:
		return g.checkRecommended(req)
	case event.AuthorityLevelNotification:
		return Resolution{
			RequestID: req.ID,
			Approved:  true,
			Resolver:  req.Actor,
			Reason:    "auto-approved (notification level)",
		}
	default:
		return Resolution{
			RequestID: req.ID,
			Approved:  false,
			Reason:    fmt.Sprintf("unknown authority level: %s", req.Level),
		}
	}
}

func (g *Gate) checkRequired(req Request) Resolution {
	if g.approver == nil {
		return Resolution{
			RequestID: req.ID,
			Approved:  false,
			Reason:    "no approver configured for Required level",
		}
	}
	approved, reason := g.approver(req)
	return Resolution{
		RequestID: req.ID,
		Approved:  approved,
		Resolver:  types.ActorID{}, // filled by caller with human ID
		Reason:    reason,
	}
}

func (g *Gate) checkRecommended(req Request) Resolution {
	if g.approver != nil {
		// Give the human a chance to respond within the timeout.
		type result struct {
			approved bool
			reason   string
		}
		ch := make(chan result, 1)
		go func() {
			a, r := g.approver(req)
			ch <- result{a, r}
		}()

		select {
		case r := <-ch:
			return Resolution{
				RequestID: req.ID,
				Approved:  r.approved,
				Reason:    r.reason,
			}
		case <-time.After(g.recommendedWait):
			return Resolution{
				RequestID: req.ID,
				Approved:  true,
				Reason:    fmt.Sprintf("auto-approved after %s (recommended level)", g.recommendedWait),
			}
		}
	}

	return Resolution{
		RequestID: req.ID,
		Approved:  true,
		Reason:    fmt.Sprintf("auto-approved (recommended level, no approver)"),
	}
}

// Pending returns all pending authority requests.
func (g *Gate) Pending() []Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Request, 0, len(g.pending))
	for _, r := range g.pending {
		out = append(out, r)
	}
	return out
}
