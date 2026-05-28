package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hivepkg "github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/safety"
)

type authorityRequestEmitter interface {
	EmitAuthorityRequest(action safety.ProtectedAction, outcome safety.AuthorityOutcome, justification string) error
}

type authorityAuditEmitter struct {
	store   store.Store
	factory *event.EventFactory
	signer  event.Signer
	actorID types.ActorID
	convID  types.ConversationID
}

func newAuthorityAuditEmitter(ctx context.Context, dsn string) (*authorityAuditEmitter, func(), error) {
	if dsn == "" {
		return nil, func() {}, nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("authority audit postgres: %w", err)
	}
	closeFn := func() { pool.Close() }

	s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("authority audit store: %w", err)
	}

	actorID := types.MustActorID("actor_hive_authority_audit")
	if err := bootstrapGraph(s, actorID); err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("authority audit bootstrap: %w", err)
	}

	return newAuthorityAuditEmitterForStore(s, actorID), closeFn, nil
}

func newAuthorityAuditEmitterForStore(s store.Store, actorID types.ActorID) *authorityAuditEmitter {
	registry := event.DefaultRegistry()
	hivepkg.RegisterWithRegistry(registry)
	return &authorityAuditEmitter{
		store:   s,
		factory: event.NewEventFactory(registry),
		signer:  deriveAuthorityAuditSigner(actorID),
		actorID: actorID,
		convID:  types.MustConversationID("conv_hive_authority_audit"),
	}
}

type authorityAuditSigner struct {
	key ed25519.PrivateKey
}

func (s *authorityAuditSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

func deriveAuthorityAuditSigner(actorID types.ActorID) *authorityAuditSigner {
	seed := sha256.Sum256([]byte("signer:" + actorID.Value()))
	return &authorityAuditSigner{key: ed25519.NewKeyFromSeed(seed[:])}
}

func (e *authorityAuditEmitter) EmitAuthorityRequest(action safety.ProtectedAction, outcome safety.AuthorityOutcome, justification string) error {
	if e == nil {
		return nil
	}

	head, err := e.store.Head()
	if err != nil {
		return fmt.Errorf("authority audit head: %w", err)
	}
	if !head.IsSome() {
		return fmt.Errorf("authority audit: no chain head")
	}
	causes := []types.EventID{head.Unwrap().ID()}

	content := event.AuthorityRequestContent{
		Action:        string(action),
		Actor:         e.actorID,
		Level:         authorityLevelForOutcome(outcome),
		Justification: justification,
		Causes:        types.MustNonEmpty(causes),
	}
	ev, err := e.factory.Create(event.EventTypeAuthorityRequested, e.actorID, content, causes, e.convID, e.store, e.signer)
	if err != nil {
		return fmt.Errorf("authority audit create: %w", err)
	}
	if _, err := e.store.Append(ev); err != nil {
		return fmt.Errorf("authority audit append: %w", err)
	}
	detail := hivepkg.AuthorityRequestRecordedContent{
		RequestID:         ev.ID(),
		RequestingActor:   e.actorID,
		RequestingRole:    "hive-cli",
		ActionName:        string(action),
		Environment:       "local",
		RiskClass:         safety.RiskClass(action),
		RequestedOutcome:  string(outcome),
		Justification:     justification,
		RiskSummary:       fmt.Sprintf("%s requires %s", action, outcome),
		Scope:             []string{string(action)},
		ProposedOperation: justification,
		CausalEventIDs:    causes,
	}
	detailEv, err := e.factory.Create(hivepkg.EventTypeAuthorityRequestRecorded, e.actorID, detail, []types.EventID{ev.ID()}, e.convID, e.store, e.signer)
	if err != nil {
		return fmt.Errorf("authority audit detail create: %w", err)
	}
	if _, err := e.store.Append(detailEv); err != nil {
		return fmt.Errorf("authority audit detail append: %w", err)
	}
	return nil
}

func authorityLevelForOutcome(outcome safety.AuthorityOutcome) event.AuthorityLevel {
	switch outcome {
	case safety.Notify:
		return event.AuthorityLevelNotification
	default:
		return event.AuthorityLevelRequired
	}
}
