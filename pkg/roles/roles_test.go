package roles

import (
	"strings"
	"testing"
)

func TestPreferredModelUnknownPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("PreferredModel(unknown) should panic but did not")
		}
	}()
	PreferredModel(Role("unknown"))
}

func TestSystemPrompt(t *testing.T) {
	tests := []struct {
		role Role
		want string // substring that should be present
	}{
		{RoleCTO, "ROLE: CTO"},
		{RoleGuardian, "ROLE: GUARDIAN"},
		{RoleResearcher, "ROLE: RESEARCHER"},
		{RoleArchitect, "ROLE: ARCHITECT"},
		{RoleBuilder, "ROLE: BUILDER"},
		{RoleReviewer, "ROLE: REVIEWER"},
		{RoleTester, "ROLE: TESTER"},
		{RoleIntegrator, "ROLE: INTEGRATOR"},
		{RoleSysMon, "ROLE: SYSMON"},
		{RoleSpawner, "ROLE: SPAWNER"},
		{RoleAllocator, "ROLE: ALLOCATOR"},
		{Role("unknown"), "hive agent"},
	}

	for _, tt := range tests {
		prompt := SystemPrompt(tt.role)
		if prompt == "" {
			t.Errorf("SystemPrompt(%q) is empty", tt.role)
		}
		if !strings.Contains(prompt, tt.want) {
			t.Errorf("SystemPrompt(%q) does not contain %q", tt.role, tt.want)
		}
	}
}

func TestSystemPromptCarriesMission(t *testing.T) {
	// Every role prompt (except unknown) must carry the soul and mission
	roles := []Role{RoleCTO, RoleGuardian, RoleResearcher, RoleArchitect,
		RoleBuilder, RoleReviewer, RoleTester, RoleIntegrator,
		RoleSysMon, RoleSpawner, RoleAllocator}

	for _, role := range roles {
		prompt := SystemPrompt(role)
		if !strings.Contains(prompt, "SOUL") {
			t.Errorf("SystemPrompt(%q) missing SOUL section", role)
		}
		if !strings.Contains(prompt, "MISSION") {
			t.Errorf("SystemPrompt(%q) missing MISSION section", role)
		}
		if !strings.Contains(prompt, "Take care of your human") {
			t.Errorf("SystemPrompt(%q) missing soul statement", role)
		}
		if !strings.Contains(prompt, "METHOD") {
			t.Errorf("SystemPrompt(%q) missing METHOD section", role)
		}
		if !strings.Contains(prompt, "TRUST") {
			t.Errorf("SystemPrompt(%q) missing TRUST section", role)
		}
	}
}

func TestSystemPromptIncludesHumanName(t *testing.T) {
	prompt := SystemPrompt(RoleCTO, "Matt")
	if !strings.Contains(prompt, "Matt") {
		t.Error("SystemPrompt with human name does not contain the name")
	}

	// Without name, should use default
	prompt = SystemPrompt(RoleCTO)
	if !strings.Contains(prompt, "the human operator") {
		t.Error("SystemPrompt without human name missing default")
	}
}

func TestSoulValues(t *testing.T) {
	base := soulValues(Role("unknown"))
	if len(base) != 3 {
		t.Errorf("base soul values = %d, want 3", len(base))
	}

	cto := soulValues(RoleCTO)
	if len(cto) != 5 {
		t.Errorf("CTO soul values = %d, want 5", len(cto))
	}

	guardian := soulValues(RoleGuardian)
	if len(guardian) != 6 {
		t.Errorf("Guardian soul values = %d, want 6", len(guardian))
	}

	builder := soulValues(RoleBuilder)
	if len(builder) != 5 {
		t.Errorf("Builder soul values = %d, want 5", len(builder))
	}

	reviewer := soulValues(RoleReviewer)
	if len(reviewer) != 5 {
		t.Errorf("Reviewer soul values = %d, want 5", len(reviewer))
	}

	sysmon := soulValues(RoleSysMon)
	if len(sysmon) != 5 {
		t.Errorf("SysMon soul values = %d, want 5", len(sysmon))
	}

	spawner := soulValues(RoleSpawner)
	if len(spawner) != 5 {
		t.Errorf("Spawner soul values = %d, want 5", len(spawner))
	}

	allocator := soulValues(RoleAllocator)
	if len(allocator) != 5 {
		t.Errorf("Allocator soul values = %d, want 5", len(allocator))
	}
}

func TestPreferredModel(t *testing.T) {
	sonnet := "claude-sonnet-4-6"
	haiku := "claude-haiku-4-5-20251001"

	tests := []struct {
		role Role
		want string
	}{
		{RoleCTO, sonnet},
		{RoleArchitect, sonnet},
		{RoleReviewer, sonnet},
		{RoleGuardian, sonnet},
		{RoleBuilder, sonnet},
		{RoleTester, sonnet},
		{RoleIntegrator, sonnet},
		{RoleResearcher, sonnet},
		{RoleSpawner, sonnet},
		{RoleSysMon, haiku},
		{RoleAllocator, haiku},
		// unknown role panics — tested separately via TestPreferredModelUnknownPanics
	}

	for _, tt := range tests {
		got := PreferredModel(tt.role)
		if got != tt.want {
			t.Errorf("PreferredModel(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}
