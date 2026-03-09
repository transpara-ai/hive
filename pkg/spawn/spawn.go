// Package spawn implements the agent spawning protocol for the hive.
//
// Spawn flow:
//  1. Caller requests spawn (SpawnRequest)
//  2. Trust gate check — agent's trust must meet the role's gate
//  3. Authority check — human must approve (Required level for all spawns)
//  4. Actor registered in the actor store
//  5. Lifecycle events emitted: agent.identity.created, agent.lifespan.started, agent.role.assigned
//  6. Agent returned ready to use
package spawn

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/roles"
)

// SpawnRequest describes a new agent to create.
type SpawnRequest struct {
	Role          roles.Role    // what role the agent will fill
	Name          string        // display name
	Justification string        // why this agent is needed
	RequestedBy   types.ActorID // who is requesting (CTO, Spawner, human)
}

// SpawnResult holds the outcome of a spawn attempt.
type SpawnResult struct {
	ActorID  types.ActorID
	Role     roles.Role
	Name     string
	Approved bool
	Reason   string // approval/denial reason
}

// Spawner creates new agents with authority checking and lifecycle events.
type Spawner struct {
	store      store.Store
	actors     actor.IActorStore
	trustModel *trust.DefaultTrustModel
	gate       *authority.Gate
	humanID    types.ActorID
	signer     event.Signer
	factory    *event.EventFactory
	convID     types.ConversationID
}

// Config for creating a Spawner.
type Config struct {
	Store   store.Store
	Actors  actor.IActorStore
	Trust   *trust.DefaultTrustModel
	Gate    *authority.Gate
	HumanID types.ActorID
	Signer  event.Signer
	Factory *event.EventFactory
	ConvID  types.ConversationID
}

// NewSpawner creates a Spawner.
func NewSpawner(cfg Config) *Spawner {
	return &Spawner{
		store:      cfg.Store,
		actors:     cfg.Actors,
		trustModel: cfg.Trust,
		gate:       cfg.Gate,
		humanID:    cfg.HumanID,
		signer:     cfg.Signer,
		factory:    cfg.Factory,
		convID:     cfg.ConvID,
	}
}

// Spawn creates a new agent after authority approval.
// Returns the spawn result (which includes whether it was approved).
func (s *Spawner) Spawn(_ context.Context, req SpawnRequest) (SpawnResult, error) {
	// Emit the spawn request event.
	reqEventID, err := s.emitSpawnRequested(req)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("emit spawn request: %w", err)
	}

	// Check trust gate — the requesting agent must have enough trust
	// for the target role (unless the human is requesting directly).
	if req.RequestedBy != s.humanID {
		if err := s.checkTrustGate(req); err != nil {
			return SpawnResult{
				Role:     req.Role,
				Name:     req.Name,
				Approved: false,
				Reason:   err.Error(),
			}, nil
		}
	}

	// Authority check — all spawns require human approval.
	authReq := authority.Request{
		ID:            reqEventID,
		Action:        fmt.Sprintf("spawn agent %q as %s", req.Name, req.Role),
		Actor:         req.RequestedBy,
		Level:         event.AuthorityLevelRequired,
		Justification: req.Justification,
		CreatedAt:     time.Now(),
	}

	resolution := s.gate.Check(authReq)
	if !resolution.Approved {
		if err := s.emitSpawnDenied(reqEventID, resolution.Reason); err != nil {
			return SpawnResult{}, fmt.Errorf("emit spawn denied: %w", err)
		}
		return SpawnResult{
			Role:     req.Role,
			Name:     req.Name,
			Approved: false,
			Reason:   resolution.Reason,
		}, nil
	}

	// Create the actor in the store.
	pub := DerivePublicKey("agent:" + req.Name)
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		return SpawnResult{}, fmt.Errorf("public key: %w", err)
	}
	a, err := s.actors.Register(pk, req.Name, event.ActorTypeAI)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("register actor: %w", err)
	}

	// Emit lifecycle events.
	if err := s.emitLifecycleEvents(a.ID(), req, reqEventID); err != nil {
		return SpawnResult{}, fmt.Errorf("lifecycle events: %w", err)
	}

	return SpawnResult{
		ActorID:  a.ID(),
		Role:     req.Role,
		Name:     req.Name,
		Approved: true,
		Reason:   resolution.Reason,
	}, nil
}

// checkTrustGate validates the requester has sufficient trust for the target role.
func (s *Spawner) checkTrustGate(req SpawnRequest) error {
	gate := roles.TrustGate(req.Role)
	if gate <= 0 {
		return nil
	}
	if s.trustModel == nil {
		return fmt.Errorf("trust gate %.2f for role %s: no trust model configured", gate, req.Role)
	}

	requester, err := s.actors.Get(req.RequestedBy)
	if err != nil {
		return fmt.Errorf("trust gate: requester not found: %w", err)
	}

	metrics, err := s.trustModel.Score(nil, requester)
	if err != nil {
		return fmt.Errorf("trust gate: score error: %w", err)
	}

	score := metrics.Overall().Value()
	if score < gate {
		return fmt.Errorf("trust gate denied: requester trust %.2f < required %.2f for role %s",
			score, gate, req.Role)
	}
	return nil
}

// emitSpawnRequested records that an agent spawn was requested.
// Uses agent.acted with Action="spawn_requested" to distinguish from
// escalation events (which have different semantics).
func (s *Spawner) emitSpawnRequested(req SpawnRequest) (types.EventID, error) {
	content := event.AgentActedContent{
		AgentID: req.RequestedBy,
		Action:  "spawn_requested",
		Target:  fmt.Sprintf("%s as %s: %s", req.Name, req.Role, req.Justification),
	}
	ev, err := s.appendEvent("agent.acted", req.RequestedBy, content)
	if err != nil {
		return types.EventID{}, err
	}
	return ev.ID(), nil
}

// emitSpawnDenied records that a spawn was denied.
func (s *Spawner) emitSpawnDenied(reqID types.EventID, reason string) error {
	content := event.AgentActedContent{
		AgentID: s.humanID,
		Action:  "spawn_denied",
		Target:  reason,
	}
	_, err := s.appendEvent("agent.acted", s.humanID, content)
	return err
}

// emitLifecycleEvents emits identity creation, lifespan start, and role assignment events.
func (s *Spawner) emitLifecycleEvents(actorID types.ActorID, req SpawnRequest, causeID types.EventID) error {
	// agent.acted — agent was created
	actedContent := event.AgentActedContent{
		AgentID: s.humanID,
		Action:  "spawn_agent",
		Target:  fmt.Sprintf("%s as %s", req.Name, req.Role),
	}
	actedEv, err := s.appendEvent("agent.acted", s.humanID, actedContent)
	if err != nil {
		return fmt.Errorf("acted event: %w", err)
	}

	// agent.role.assigned
	roleContent := event.AgentRoleAssignedContent{
		AgentID: actorID,
		Role:    string(req.Role),
	}
	_, err = s.appendEventAfter("agent.role.assigned", actorID, roleContent, actedEv.ID())
	if err != nil {
		return fmt.Errorf("role assigned event: %w", err)
	}

	return nil
}

// appendEvent appends an event caused by the current head.
func (s *Spawner) appendEvent(eventType string, source types.ActorID, content event.EventContent) (event.Event, error) {
	head, err := s.store.Head()
	if err != nil {
		return event.Event{}, fmt.Errorf("store head: %w", err)
	}
	if !head.IsSome() {
		return event.Event{}, fmt.Errorf("graph not bootstrapped")
	}
	return s.appendEventAfter(eventType, source, content, head.Unwrap().ID())
}

// appendEventAfter appends an event caused by a specific event.
func (s *Spawner) appendEventAfter(eventType string, source types.ActorID, content event.EventContent, cause types.EventID) (event.Event, error) {
	et, err := types.NewEventType(eventType)
	if err != nil {
		return event.Event{}, fmt.Errorf("event type %q: %w", eventType, err)
	}
	ev, err := s.factory.Create(et, source, content, []types.EventID{cause}, s.convID, s.store, s.signer)
	if err != nil {
		return event.Event{}, fmt.Errorf("create event: %w", err)
	}
	return s.store.Append(ev)
}

// DerivePublicKey generates a deterministic Ed25519 public key from a seed string.
// Used to create agents with stable, reproducible identities in the actor store.
func DerivePublicKey(seed string) ed25519.PublicKey {
	h := sha256.Sum256([]byte(seed))
	priv := ed25519.NewKeyFromSeed(h[:])
	return priv.Public().(ed25519.PublicKey)
}
