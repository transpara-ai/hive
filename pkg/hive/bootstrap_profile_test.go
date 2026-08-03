package hive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/hive/pkg/safety"
)

func TestBootstrapProfilesFullCompatibilityAndOrganicKernel(t *testing.T) {
	full := StarterAgents("Michael")
	explicitFull, err := StarterAgentsForProfile("Michael", BootstrapProfileFull)
	if err != nil {
		t.Fatalf("explicit full: %v", err)
	}
	if len(full) != 9 || len(explicitFull) != len(full) {
		t.Fatalf("full roster lengths = %d and %d, want 9", len(full), len(explicitFull))
	}
	for i := range full {
		if full[i].Name != explicitFull[i].Name || full[i].Role != explicitFull[i].Role {
			t.Fatalf("full roster[%d] changed: %+v vs %+v", i, full[i], explicitFull[i])
		}
	}

	organic, err := StarterAgentsForProfile("Michael", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("organic-v1: %v", err)
	}
	want := []string{"guardian", "sysmon", "allocator", "cto", "spawner"}
	if len(organic) != len(want) {
		t.Fatalf("organic roster length = %d, want %d", len(organic), len(want))
	}
	for i, role := range want {
		if organic[i].Name != role || organic[i].Role != role {
			t.Fatalf("organic roster[%d] = %s/%s, want %s", i, organic[i].Name, organic[i].Role, role)
		}
	}
}

func TestBootstrapProfileAndActionValidationIsTyped(t *testing.T) {
	if _, err := StarterAgentsForProfile("Michael", ""); err == nil {
		t.Fatal("empty explicit profile was accepted")
	} else {
		var typed BootstrapProfileError
		if !errors.As(err, &typed) {
			t.Fatalf("empty profile error = %T, want BootstrapProfileError", err)
		}
	}

	_, err := NormalizeProtectedActions([]safety.ProtectedAction{"unknown.action"})
	var actionErr ProtectedActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("unknown action error = %v, want ProtectedActionError", err)
	}
}

func TestOrganicConfigRejectsBroadOrIncoherentSettings(t *testing.T) {
	base := Config{
		BootstrapProfile:             BootstrapProfileOrganicV1,
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"broad approval", func(cfg *Config) { cfg.ApproveRequests = true }},
		{"wrong cap", func(cfg *Config) { cfg.MaximumDynamicActors = 4 }},
		{"wrong policy", func(cfg *Config) { cfg.GrowthPolicyVersion = "organic-v2" }},
		{"broad action", func(cfg *Config) {
			cfg.AutomaticallyApprovedActions = []safety.ProtectedAction{safety.ActionRepoMergeMain}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			var organicErr OrganicConfigError
			if err := ValidateBootstrapConfig(cfg); !errors.As(err, &organicErr) {
				t.Fatalf("error = %v, want OrganicConfigError", err)
			}
		})
	}
}

func TestOrganicExactActionAllowlistRejectsEveryOtherProtectedAction(t *testing.T) {
	actions := append([]safety.ProtectedAction(nil), safety.ProtectedActions...)
	actions = append(actions, safety.RepoProtectedActions...)
	for _, action := range actions {
		if action == safety.ActionAgentSpawnPersistent {
			continue
		}
		t.Run(string(action), func(t *testing.T) {
			err := ValidateBootstrapConfig(Config{
				BootstrapProfile:             BootstrapProfileOrganicV1,
				GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
				MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
				AutomaticallyApprovedActions: []safety.ProtectedAction{action},
			})
			var organicErr OrganicConfigError
			if !errors.As(err, &organicErr) {
				t.Fatalf("action %q error = %v, want OrganicConfigError", action, err)
			}
		})
	}
}

func TestPrewriteRejectRunsBeforeGraphOrWorkDependencies(t *testing.T) {
	tests := []Config{
		{BootstrapProfile: BootstrapProfile("unknown")},
		{AutomaticallyApprovedActions: []safety.ProtectedAction{"unknown.action"}},
		{MinimumIterationsBeforeQuiescence: -1},
		{MinimumIterationsBeforeQuiescence: 1},
		{
			BootstrapProfile:             BootstrapProfileOrganicV1,
			GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
			MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
			AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
			ApproveRequests:              true,
		},
	}
	for i, cfg := range tests {
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatalf("case %d reached nil graph/actor dependencies instead of pre-write rejection", i)
		}
	}
}

func TestOrganicMinimumIterationsBeforeQuiescenceIsBoundedAndWired(t *testing.T) {
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "OrganicOperator")
	rt, err := New(t.Context(), Config{
		Store:                             store.NewInMemoryStore(),
		Actors:                            actors,
		HumanID:                           humanID,
		BootstrapProfile:                  BootstrapProfileOrganicV1,
		GrowthPolicyVersion:               OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:              OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions:      []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
		MinimumIterationsBeforeQuiescence: 30,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.minimumIterationsBeforeQuiescence != 30 {
		t.Fatalf("runtime floor = %d, want 30", rt.minimumIterationsBeforeQuiescence)
	}
}

func TestOrganicRuntimeRejectsAnyNonExactKernelBeforeRunWrite(t *testing.T) {
	expected, err := StarterAgentsForProfile("OrganicOperator", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("organic starters: %v", err)
	}
	tests := []struct {
		name   string
		mutate func([]AgentDef) []AgentDef
	}{
		{"missing", func(defs []AgentDef) []AgentDef { return defs[:len(defs)-1] }},
		{"extra", func(defs []AgentDef) []AgentDef { return append(defs, defs[0]) }},
		{"reordered", func(defs []AgentDef) []AgentDef {
			defs[0], defs[1] = defs[1], defs[0]
			return defs
		}},
		{"operative", func(defs []AgentDef) []AgentDef {
			defs[0].CanOperate = true
			return defs
		}},
		{"definition drift", func(defs []AgentDef) []AgentDef {
			defs[0].SystemPrompt += "\ndrift"
			return defs
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eventStore := store.NewInMemoryStore()
			actors := actor.NewInMemoryActorStore()
			humanID := registerTestHuman(t, actors, "OrganicOperator")
			rt, err := New(t.Context(), Config{
				Store:                        eventStore,
				Actors:                       actors,
				HumanID:                      humanID,
				BootstrapProfile:             BootstrapProfileOrganicV1,
				GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
				MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
				AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { _ = rt.graph.Close() })
			defs := append([]AgentDef(nil), expected...)
			rt.defs = tc.mutate(defs)
			before, err := eventStore.Count()
			if err != nil {
				t.Fatalf("count before: %v", err)
			}
			var organicErr OrganicConfigError
			if err := rt.Run(t.Context(), "must not persist"); !errors.As(err, &organicErr) {
				t.Fatalf("Run error = %v, want OrganicConfigError", err)
			}
			after, err := eventStore.Count()
			if err != nil {
				t.Fatalf("count after: %v", err)
			}
			if after != before {
				t.Fatalf("invalid kernel wrote %d events", after-before)
			}
		})
	}
}

func TestFullProfilePreservesNewTimeSystemActorAndBootstrapHiveEventOrder(t *testing.T) {
	eventStore := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "FullOperator")
	rt, err := New(t.Context(), Config{
		Store:   eventStore,
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("New full runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.bootstrapProfile != BootstrapProfileFull || rt.systemID.IsZero() {
		t.Fatalf("full New posture profile=%q system=%s", rt.bootstrapProfile, rt.systemID)
	}
	bootstrap, err := event.NewBootstrapFactory(event.DefaultRegistry()).Init(humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := eventStore.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	provider := &organicTestProvider{
		runtime: rt,
		role:    "guardian",
		seen:    make(chan bool, 1),
	}
	rt.providerFactory = func(intelligence.Config) (intelligence.Provider, error) {
		return provider, nil
	}
	def := StarterAgents("FullOperator")[0]
	if _, _, err := rt.spawnAgent(t.Context(), def); err != nil {
		t.Fatalf("spawn full bootstrap agent: %v", err)
	}
	events, err := eventsByConversationChronological(eventStore, rt.convID)
	if err != nil {
		t.Fatalf("full conversation: %v", err)
	}
	spawned, identity, definition := -1, -1, -1
	for i, ev := range events {
		switch content := ev.Content().(type) {
		case AgentSpawnedContent:
			if content.Name == def.Name {
				spawned = i
			}
		case AgentIdentityRegisteredContent:
			if content.DisplayName == def.Name {
				identity = i
			}
		case RoleDefinitionContent:
			if content.Name == def.Name {
				definition = i
			}
		}
	}
	if !(spawned >= 0 && spawned < identity && identity < definition) {
		t.Fatalf("full bootstrap Hive event order spawned=%d identity=%d definition=%d", spawned, identity, definition)
	}
}

func TestRunEvidenceLegacyDecodeAndNewFields(t *testing.T) {
	var legacyStart RunStartedContent
	if err := json.Unmarshal([]byte(`{"Idea":"legacy"}`), &legacyStart); err != nil {
		t.Fatalf("decode legacy start: %v", err)
	}
	if legacyStart.BootstrapProfile != "" {
		t.Fatalf("legacy profile = %q, want unrecorded", legacyStart.BootstrapProfile)
	}
	var legacySpawn AgentSpawnedContent
	if err := json.Unmarshal([]byte(`{"Name":"old","Role":"old","Model":"m","ActorID":"a"}`), &legacySpawn); err != nil {
		t.Fatalf("decode legacy spawn: %v", err)
	}
	if legacySpawn.Recovered {
		t.Fatal("legacy spawn invented recovered=true")
	}
	var legacyCompleted RunCompletedContent
	if err := json.Unmarshal([]byte(`{"AgentCount":9,"DurationMs":42,"TotalCost":1.5}`), &legacyCompleted); err != nil {
		t.Fatalf("decode legacy completion: %v", err)
	}
	if legacyCompleted.BootstrapActorCount != 0 ||
		legacyCompleted.RecoveredDynamicActorCount != 0 ||
		legacyCompleted.NewDynamicActorCount != 0 ||
		legacyCompleted.DynamicActorCount != 0 {
		t.Fatalf("legacy completion invented new counts: %+v", legacyCompleted)
	}

	start := RunStartedContent{
		BootstrapProfile:             BootstrapProfileOrganicV1,
		BootstrapRoles:               []string{"guardian", "sysmon", "allocator", "cto", "spawner"},
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []string{string(safety.ActionAgentSpawnPersistent)},
	}
	data, err := json.Marshal(start)
	if err != nil {
		t.Fatalf("marshal start: %v", err)
	}
	var roundTrip RunStartedContent
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("round-trip start: %v", err)
	}
	if roundTrip.BootstrapProfile != BootstrapProfileOrganicV1 ||
		roundTrip.MaximumDynamicActors != OrganicV1MaximumDynamicActors {
		t.Fatalf("round-trip evidence = %+v", roundTrip)
	}
	completedData, err := json.Marshal(RunCompletedContent{
		AgentCount:                 5,
		DurationMs:                 42,
		BootstrapActorCount:        5,
		RecoveredDynamicActorCount: 1,
		NewDynamicActorCount:       2,
		DynamicActorCount:          3,
	})
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	var completedRoundTrip RunCompletedContent
	if err := json.Unmarshal(completedData, &completedRoundTrip); err != nil {
		t.Fatalf("round-trip completion: %v", err)
	}
	if completedRoundTrip.BootstrapActorCount != 5 ||
		completedRoundTrip.RecoveredDynamicActorCount != 1 ||
		completedRoundTrip.NewDynamicActorCount != 2 ||
		completedRoundTrip.DynamicActorCount != 3 {
		t.Fatalf("round-trip completion evidence = %+v", completedRoundTrip)
	}
}
