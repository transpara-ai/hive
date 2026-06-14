package hive

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestLatestModelRolePolicyUsesCausalOrderNotTimestamp(t *testing.T) {
	s, _, signer, human, conv := newDecisionTestStore(t)
	appendModelRolePolicyEventAt(t, s, signer, human, conv, ModelRolePolicyUpdatedContent{
		Role:              "guardian",
		Model:             "sonnet",
		RequestedAuthMode: "subscription",
		ResolvedModel:     "claude-sonnet-4-6",
		ResolvedProvider:  "claude-cli",
		AuthMode:          "subscription",
	}, time.Unix(200, 0))
	want := appendModelRolePolicyEventAt(t, s, signer, human, conv, ModelRolePolicyUpdatedContent{
		Role:              "guardian",
		Model:             "api-sonnet",
		RequestedAuthMode: "api-key",
		ResolvedModel:     "api-claude-sonnet-4-6",
		ResolvedProvider:  "anthropic",
		AuthMode:          "api-key",
	}, time.Unix(100, 0))

	projection := BuildOperatorProjection(s, 50)
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.PolicyEventID != want.ID().Value() {
		t.Fatalf("projected policy event = %q, want causally latest %q", guardian.PolicyEventID, want.ID().Value())
	}
	if guardian.Model != "api-claude-sonnet-4-6" || guardian.Provider != "anthropic" || guardian.AuthMode != "api-key" {
		t.Fatalf("guardian assignment = %+v, want causally latest api-key policy", guardian)
	}

	stored, ok, err := latestModelRolePolicyUpdateForRole(s, "guardian", 50)
	if err != nil {
		t.Fatalf("latestModelRolePolicyUpdateForRole: %v", err)
	}
	if !ok {
		t.Fatal("latestModelRolePolicyUpdateForRole ok = false, want true")
	}
	if stored.EventID != want.ID().Value() {
		t.Fatalf("runtime policy event = %q, want causally latest %q", stored.EventID, want.ID().Value())
	}
}

func TestLatestModelRolePolicyForRoleIgnoresUnknownRoleEventsForOtherRoles(t *testing.T) {
	s, _, signer, human, conv := newDecisionTestStore(t)
	want := appendModelRolePolicyEventAt(t, s, signer, human, conv, ModelRolePolicyUpdatedContent{
		Role:              "guardian",
		Model:             "sonnet",
		RequestedAuthMode: "subscription",
		ResolvedModel:     "claude-sonnet-4-6",
		ResolvedProvider:  "claude-cli",
		AuthMode:          "subscription",
	}, time.Unix(100, 0))
	appendModelRolePolicyEventAt(t, s, signer, human, conv, ModelRolePolicyUpdatedContent{
		Role:              "retired-role",
		Model:             "api-sonnet",
		RequestedAuthMode: "api-key",
		ResolvedModel:     "api-claude-sonnet-4-6",
		ResolvedProvider:  "anthropic",
		AuthMode:          "api-key",
	}, time.Unix(200, 0))

	stored, ok, err := latestModelRolePolicyUpdateForRole(s, "guardian", 50)
	if err != nil {
		t.Fatalf("latestModelRolePolicyUpdateForRole: %v", err)
	}
	if !ok {
		t.Fatal("latestModelRolePolicyUpdateForRole ok = false, want true")
	}
	if stored.EventID != want.ID().Value() {
		t.Fatalf("runtime policy event = %q, want guardian event %q", stored.EventID, want.ID().Value())
	}
}

func TestLatestModelRolePolicyPaginatesPastFirstPage(t *testing.T) {
	s, _, signer, human, conv := newDecisionTestStore(t)
	const pageLimit = 10
	want := appendModelRolePolicyEventAt(t, s, signer, human, conv, modelPolicyContent("guardian", "sonnet"), time.Unix(100, 0))
	for i := 0; i < pageLimit+5; i++ {
		appendModelRolePolicyEventAt(t, s, signer, human, conv, modelPolicyContent("planner", "haiku"), time.Unix(200+int64(i), 0))
	}

	stored, ok, err := latestModelRolePolicyUpdateForRole(s, "guardian", pageLimit)
	if err != nil {
		t.Fatalf("latestModelRolePolicyUpdateForRole: %v", err)
	}
	if !ok {
		t.Fatal("latestModelRolePolicyUpdateForRole ok = false, want true after paging")
	}
	if stored.EventID != want.ID().Value() {
		t.Fatalf("runtime policy event = %q, want older guardian event beyond first page %q", stored.EventID, want.ID().Value())
	}

	projection := BuildOperatorProjection(s, pageLimit)
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.PolicyEventID != want.ID().Value() {
		t.Fatalf("projected policy event = %q, want older guardian event beyond first page %q", guardian.PolicyEventID, want.ID().Value())
	}
}

func TestUnknownModelRolePolicyEventsDoNotPoisonStarterRolePolicyState(t *testing.T) {
	s, _, signer, human, conv := newDecisionTestStore(t)
	want := appendModelRolePolicyEventAt(t, s, signer, human, conv, modelPolicyContent("guardian", "sonnet"), time.Unix(100, 0))
	appendModelRolePolicyEventAt(t, s, signer, human, conv, ModelRolePolicyUpdatedContent{
		Role:              "retired-role",
		Model:             "api-sonnet",
		RequestedAuthMode: "api-key",
		ResolvedModel:     "api-claude-sonnet-4-6",
		ResolvedProvider:  "anthropic",
		AuthMode:          "api-key",
	}, time.Unix(200, 0))

	source := modelSelectionSourceWithRolePolicyUpdates(s, nil, 1)
	config := source()
	if config.RolePolicyError != "" {
		t.Fatalf("RolePolicyError = %q, want unknown-role policy event scoped away", config.RolePolicyError)
	}
	if got := config.RolePolicies["guardian"].EventID; got != want.ID().Value() {
		t.Fatalf("guardian role policy event = %q, want %q", got, want.ID().Value())
	}

	projection := BuildOperatorProjection(s, 1)
	if len(projection.Errors) != 0 {
		t.Fatalf("projection errors = %+v, want none for unknown-role policy event", projection.Errors)
	}
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.PolicyEventID != want.ID().Value() {
		t.Fatalf("projected guardian policy event = %q, want %q", guardian.PolicyEventID, want.ID().Value())
	}

	if _, err := ValidateModelOverrides([]ModelOverrideRequest{{Role: "guardian", Model: "haiku"}}, source); err != nil {
		t.Fatalf("ValidateModelOverrides with unknown-role policy event: %v", err)
	}
}

func TestMalformedModelRolePolicyEventFailsClosedAndProjectsError(t *testing.T) {
	s, _, signer, human, conv := newDecisionTestStore(t)
	want := appendModelRolePolicyEventAt(t, s, signer, human, conv, modelPolicyContent("guardian", "sonnet"), time.Unix(100, 0))
	appendMalformedModelRolePolicyEventAt(t, s, signer, human, conv, time.Unix(200, 0))

	if _, ok, err := latestModelRolePolicyUpdateForRole(s, "guardian", 50); err == nil {
		t.Fatal("latestModelRolePolicyUpdateForRole err = nil, want malformed-content fail-closed error")
	} else if ok {
		t.Fatal("latestModelRolePolicyUpdateForRole ok = true with malformed newest policy event")
	} else if !strings.Contains(err.Error(), "ModelRolePolicyUpdatedContent") {
		t.Fatalf("latestModelRolePolicyUpdateForRole err = %q, want content type error", err.Error())
	}

	projection := BuildOperatorProjection(s, 50)
	if !containsStringWith(projection.Errors, "ModelRolePolicyUpdatedContent") {
		t.Fatalf("projection errors = %+v, want malformed role-policy content error", projection.Errors)
	}
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.PolicyEventID != want.ID().Value() {
		t.Fatalf("projected guardian policy event = %q, want valid older policy %q", guardian.PolicyEventID, want.ID().Value())
	}

	config := modelSelectionSourceWithRolePolicyUpdates(s, nil, 50)()
	if !strings.Contains(config.RolePolicyError, "ModelRolePolicyUpdatedContent") {
		t.Fatalf("RolePolicyError = %q, want malformed role-policy content error", config.RolePolicyError)
	}
}

func modelPolicyContent(role, model string) ModelRolePolicyUpdatedContent {
	return ModelRolePolicyUpdatedContent{
		Role:              role,
		Model:             model,
		RequestedAuthMode: "subscription",
	}
}

func containsStringWith(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func appendModelRolePolicyEventAt(t *testing.T, s store.Store, signer event.Signer, source types.ActorID, conv types.ConversationID, content ModelRolePolicyUpdatedContent, at time.Time) event.Event {
	t.Helper()
	return appendModelRolePolicyEventContentAt(t, s, signer, source, conv, content, at)
}

func appendMalformedModelRolePolicyEventAt(t *testing.T, s store.Store, signer event.Signer, source types.ActorID, conv types.ConversationID, at time.Time) event.Event {
	t.Helper()
	content := AuthorityRequestRecordedContent{
		RequestingRole:   "guardian",
		ActionName:       "malformed-model-policy",
		Target:           "model-policy-test",
		Environment:      "test",
		RiskClass:        "test",
		RequestedOutcome: "ignored",
		Justification:    "exercise malformed content fail-closed path",
		RiskSummary:      "wrong content payload under model-policy event type",
	}
	return appendModelRolePolicyEventContentAt(t, s, signer, source, conv, content, at)
}

func appendModelRolePolicyEventContentAt(t *testing.T, s store.Store, signer event.Signer, source types.ActorID, conv types.ConversationID, content event.EventContent, at time.Time) event.Event {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.IsNone() {
		t.Fatal("store has no bootstrap head")
	}
	causes := []types.EventID{head.Unwrap().ID()}
	prevHash := head.Unwrap().Hash()
	id, err := types.NewEventIDFromNew()
	if err != nil {
		t.Fatalf("event id: %v", err)
	}
	timestamp := types.NewTimestamp(at)
	tmp := event.NewEvent(event.CurrentEventVersion, id, EventTypeModelRolePolicyUpdated, timestamp, source, content, causes, conv, types.ZeroHash(), prevHash, types.Signature{})
	hash, err := event.ComputeHash(event.CanonicalForm(tmp))
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}
	hashBytes, err := hex.DecodeString(hash.Value())
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	signature, err := signer.Sign(hashBytes)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ev := event.NewEvent(event.CurrentEventVersion, id, EventTypeModelRolePolicyUpdated, timestamp, source, content, causes, conv, hash, prevHash, signature)
	stored, err := s.Append(ev)
	if err != nil {
		t.Fatalf("append policy event: %v", err)
	}
	return stored
}
