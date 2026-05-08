package hive

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestPrepareAgentIdentityDefaultsToProductionGenerated(t *testing.T) {
	identity, err := prepareAgentIdentity(AgentDef{Name: "builder"})
	if err != nil {
		t.Fatalf("prepareAgentIdentity: %v", err)
	}
	if identity.Environment != AgentIdentityEnvironmentProduction {
		t.Fatalf("environment = %q, want production", identity.Environment)
	}
	if identity.Mode != AgentIdentityModeGenerated {
		t.Fatalf("mode = %q, want generated", identity.Mode)
	}
	if identity.Provenance != KeyProvenanceGenerated {
		t.Fatalf("provenance = %q, want generated", identity.Provenance)
	}
	if identity.SigningKey != nil {
		t.Fatal("generated identity should not carry supplied key material")
	}
}

func TestPrepareAgentIdentityRejectsProductionDeterministicFixture(t *testing.T) {
	_, err := prepareAgentIdentity(AgentDef{
		Name:         "PublicName",
		IdentityMode: AgentIdentityModeDeterministicFixture,
	})
	if err == nil {
		t.Fatal("expected production deterministic fixture identity to fail closed")
	}
	if !strings.Contains(err.Error(), "deterministic fixture identity is blocked in production") {
		t.Fatalf("error = %q, want production deterministic fixture block", err.Error())
	}
}

func TestPrepareAgentIdentityRejectsProductionPublicNameDerivedExternalKey(t *testing.T) {
	_, err := prepareAgentIdentity(AgentDef{
		Name:           "PublicName",
		IdentityMode:   AgentIdentityModeExternallyManaged,
		ExternalKeyRef: "kms://prod/agents/public-name",
		SigningKey:     deterministicAgentSigningKey("PublicName"),
	})
	if err == nil {
		t.Fatal("expected public-name-derived production signing key to fail closed")
	}
	if !strings.Contains(err.Error(), "public-name-derived identity is blocked in production") {
		t.Fatalf("error = %q, want public-name-derived production block", err.Error())
	}
}

func TestPrepareAgentIdentityRejectsExternallyManagedWithoutKeyReference(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	_, err = prepareAgentIdentity(AgentDef{
		Name:         "ManagedKeyAgent",
		IdentityMode: AgentIdentityModeExternallyManaged,
		SigningKey:   key,
	})
	if err == nil {
		t.Fatal("expected externally managed identity without key reference to fail closed")
	}
	if !strings.Contains(err.Error(), "externally_managed mode requires ExternalKeyRef") {
		t.Fatalf("error = %q, want missing ExternalKeyRef block", err.Error())
	}
}

func TestPrepareAgentIdentityAllowsExternallyManagedWithKeyReference(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	identity, err := prepareAgentIdentity(AgentDef{
		Name:           "ManagedKeyAgent",
		IdentityMode:   AgentIdentityModeExternallyManaged,
		ExternalKeyRef: "kms://prod/agents/managed-key-agent",
		SigningKey:     key,
	})
	if err != nil {
		t.Fatalf("prepareAgentIdentity: %v", err)
	}
	if identity.Provenance != KeyProvenanceExternallyManaged {
		t.Fatalf("provenance = %q, want externally_managed", identity.Provenance)
	}
}

func TestPrepareAgentIdentityAllowsFixtureOnlyInDevelopmentOrTest(t *testing.T) {
	for _, env := range []AgentIdentityEnvironment{
		AgentIdentityEnvironmentDevelopment,
		AgentIdentityEnvironmentTest,
	} {
		t.Run(string(env), func(t *testing.T) {
			identity, err := prepareAgentIdentity(AgentDef{
				Name:                "FixtureName",
				IdentityEnvironment: env,
				IdentityMode:        AgentIdentityModeDeterministicFixture,
			})
			if err != nil {
				t.Fatalf("prepareAgentIdentity: %v", err)
			}
			if identity.Provenance != KeyProvenanceDeterministicFixture {
				t.Fatalf("provenance = %q, want deterministic_fixture", identity.Provenance)
			}
		})
	}
}

func TestEmitAgentIdentityRegisteredRecordsKeyProvenance(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	identity, err := prepareAgentIdentity(AgentDef{Name: "builder"})
	if err != nil {
		t.Fatalf("prepareAgentIdentity: %v", err)
	}

	_, pub, actorID := registerIdentityTestActor(t, rt.actors, "builder")
	if err := rt.emitAgentIdentityRegistered(actorID, AgentDef{
		Name: "builder",
		Role: "builder",
	}, identity); err != nil {
		t.Fatalf("emitAgentIdentityRegistered: %v", err)
	}

	page, err := rt.store.ByType(EventTypeAgentIdentityRegistered, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(agent.identity.registered): %v", err)
	}
	items := page.Items()
	if len(items) != 1 {
		t.Fatalf("identity registered event count = %d, want 1", len(items))
	}
	content, ok := items[0].Content().(AgentIdentityRegisteredContent)
	if !ok {
		t.Fatalf("content type = %T, want AgentIdentityRegisteredContent", items[0].Content())
	}
	if content.ActorID != actorID {
		t.Fatalf("ActorID = %s, want %s", content.ActorID.Value(), actorID.Value())
	}
	if !bytes.Equal(content.PublicKey.Bytes(), pub.Bytes()) {
		t.Fatal("PublicKey was not recorded from the actor registry")
	}
	if content.KeyProvenance != string(KeyProvenanceGenerated) {
		t.Fatalf("KeyProvenance = %q, want generated", content.KeyProvenance)
	}
	if content.Environment != string(AgentIdentityEnvironmentProduction) {
		t.Fatalf("Environment = %q, want production", content.Environment)
	}
	if content.IdentityMode != string(AgentIdentityModeGenerated) {
		t.Fatalf("IdentityMode = %q, want generated", content.IdentityMode)
	}
}

func deterministicAgentSigningKey(name string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("agent:" + name))
	return ed25519.NewKeyFromSeed(seed[:])
}

func newIdentityTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "IdentityOperator")
	rt, err := New(t.Context(), Config{
		Store:   store.NewInMemoryStore(),
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("New runtime: %v", err)
	}
	bootstrap, err := event.NewBootstrapFactory(event.DefaultRegistry()).Init(humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	return rt
}

func registerIdentityTestActor(t *testing.T, actors actor.IActorStore, name string) (ed25519.PrivateKey, types.PublicKey, types.ActorID) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, err := types.NewPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	a, err := actors.Register(pub, name, event.ActorTypeAI)
	if err != nil {
		t.Fatalf("register actor: %v", err)
	}
	return priv, pub, a.ID()
}
