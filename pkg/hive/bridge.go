package hive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	hiveagent "github.com/lovyou-ai/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/runner"
)

// bridgeAgentName is the deterministic name for the hive's bridge actor.
// Every site op is anchored to the chain under this actor's identity.
const bridgeAgentName = "hive-bridge"

// bridgeProvider is a minimal intelligence.Provider used only to satisfy
// the Agent constructor — the bridge agent never calls Reason() or Operate().
// All it does is emit site.op.* and hive.spec.* anchor/translation events.
type bridgeProvider struct {
	decision.NoOpIntelligence
}

func (bridgeProvider) Name() string  { return "hive-bridge" }
func (bridgeProvider) Model() string { return "none" }

var _ intelligence.Provider = bridgeProvider{}

// initBridgeAgent constructs the bridge actor the first time it is needed.
// Safe to call multiple times — guarded by bridgeOnce.
func (r *Runtime) initBridgeAgent(ctx context.Context) error {
	var initErr error
	r.bridgeOnce.Do(func() {
		ag, err := hiveagent.New(ctx, hiveagent.Config{
			Role:           hiveagent.Role("bridge"),
			Name:           bridgeAgentName,
			Graph:          r.graph,
			Provider:       bridgeProvider{},
			ConversationID: r.convID,
		})
		if err != nil {
			initErr = fmt.Errorf("init bridge agent: %w", err)
			return
		}
		r.bridgeAgent = ag
	})
	return initErr
}

// AnchorSiteOp records site.op.received synchronously and returns the
// anchor event's ID. The raw payload is hashed (SHA-256) before being
// stored on the chain — only the hash, not the bytes.
//
// The bridge mutex wraps the emit+read pair so concurrent webhooks each
// observe their own anchor ID. Without this wrap, two concurrent emitters
// could interleave LastEvent() reads and cross their anchor IDs.
func (r *Runtime) AnchorSiteOp(ctx context.Context, op runner.OpEvent) (types.EventID, error) {
	if err := r.initBridgeAgent(ctx); err != nil {
		return types.EventID{}, err
	}

	content := event.SiteOpReceivedContent{
		ExternalRef:   event.ExternalRef{System: "site", ID: op.ID},
		SpaceID:       op.SpaceID,
		NodeID:        op.NodeID,
		NodeTitle:     op.NodeTitle,
		Actor:         op.Actor,
		ActorID:       op.ActorID,
		ActorKind:     op.ActorKind,
		OpKind:        op.Op,
		PayloadHash:   sha256HexPrefixed(op.Payload),
		ReceivedAt:    time.Now().UTC(),
		SiteCreatedAt: op.CreatedAt,
	}

	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	if err := r.bridgeAgent.EmitSiteOpReceived(content); err != nil {
		return types.EventID{}, fmt.Errorf("anchor site op: %w", err)
	}
	return r.bridgeAgent.LastEvent(), nil
}

// bridgeEmitRejected records a translation failure on the chain.
// Serialised against the bridge mutex to preserve the bridge agent's own
// causality chain.
func (r *Runtime) bridgeEmitRejected(content event.SiteOpRejectedContent) error {
	if r.bridgeAgent == nil {
		return fmt.Errorf("bridge agent not initialised")
	}
	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	return r.bridgeAgent.EmitSiteOpRejected(content)
}

// bridgeEmitTranslated records a successful translation on the chain.
func (r *Runtime) bridgeEmitTranslated(content event.SiteOpTranslatedContent) error {
	if r.bridgeAgent == nil {
		return fmt.Errorf("bridge agent not initialised")
	}
	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	return r.bridgeAgent.EmitSiteOpTranslated(content)
}

// bridgeEmitMirrored records a successful mirror POST on the chain.
func (r *Runtime) bridgeEmitMirrored(content event.SiteOpMirroredContent) error {
	if r.bridgeAgent == nil {
		return fmt.Errorf("bridge agent not initialised")
	}
	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	return r.bridgeAgent.EmitSiteOpMirrored(content)
}

// sha256HexPrefixed returns "sha256:" + hex(SHA-256(b)). Empty input is
// still hashed (produces the well-known empty-input digest) so every
// anchor has a non-empty payload_hash.
func sha256HexPrefixed(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
