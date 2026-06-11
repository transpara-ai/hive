package hive

import (
	"strings"
	"testing"
	"time"
)

// v14-F3: park-and-renew only works if the renewer outlives its wards. The
// allocator gets an explicit 12-hour lifespan (workers keep the 30m default
// and get renewed on-chain); its prompt must teach the duration resource or
// the mechanism exists with no operator.
func TestAllocatorOutlivesWorkers(t *testing.T) {
	defs := StarterAgents("Michael")
	var alloc *AgentDef
	for i := range defs {
		if defs[i].Name == "allocator" {
			alloc = &defs[i]
			break
		}
	}
	if alloc == nil {
		t.Fatal("no allocator in StarterAgents")
	}
	if alloc.MaxDuration != 12*time.Hour {
		t.Fatalf("allocator MaxDuration = %v; want 12h (the renewer must outlive the renewed)", alloc.MaxDuration)
	}
	if got := alloc.EffectiveMaxDuration(); got != 12*time.Hour {
		t.Fatalf("allocator EffectiveMaxDuration = %v; want 12h", got)
	}
	for _, want := range []string{`"resource"`, "duration", "parked"} {
		if !strings.Contains(alloc.SystemPrompt, want) {
			t.Fatalf("allocator prompt must teach duration renewal (missing %q)", want)
		}
	}

	// The renewer outlives every renewed agent: no worker's effective
	// lifespan may reach the allocator's. (Workers keep their existing
	// defaults — the implementer's deliberate 4h included; renewal, not a
	// blanket constant bump, is the lifespan policy.)
	for i := range defs {
		if defs[i].Name == "allocator" {
			continue
		}
		if got := defs[i].EffectiveMaxDuration(); got >= alloc.EffectiveMaxDuration() {
			t.Fatalf("%s EffectiveMaxDuration = %v; must stay below the allocator's %v (the renewer outlives the renewed)", defs[i].Name, got, alloc.EffectiveMaxDuration())
		}
	}
}
