package roles

import (
	"os"
	"strings"
	"testing"
)

// TestPreferredModelMatchesCLAUDEMd reads the Intelligence tier table from
// CLAUDE.md and asserts that each role's model constant matches PreferredModel().
// This catches drift between roles.go and documentation before it triggers
// costly pipeline runs where the CTO flags a discrepancy.
func TestPreferredModelMatchesCLAUDEMd(t *testing.T) {
	data, err := os.ReadFile("../../CLAUDE.md")
	if err != nil {
		t.Fatalf("could not read CLAUDE.md: %v", err)
	}

	// Map from CLAUDE.md role name to Role constant.
	roleByName := map[string]Role{
		"CTO":        RoleCTO,
		"Guardian":   RoleGuardian,
		"PM":         RolePM,
		"SysMon":     RoleSysMon,
		"Spawner":    RoleSpawner,
		"Allocator":  RoleAllocator,
		"Researcher": RoleResearcher,
		"Architect":  RoleArchitect,
		"Builder":    RoleBuilder,
		"Reviewer":   RoleReviewer,
		"Tester":     RoleTester,
		"Integrator": RoleIntegrator,
	}

	// Map from Intelligence tier name (as written in CLAUDE.md) to model constant.
	modelByTier := map[string]string{
		"Sonnet": "claude-sonnet-4-6",
		"Haiku":  "claude-haiku-4-5-20251001",
	}

	// Parse table rows from CLAUDE.md.
	// Each row looks like: | Role | Responsibility | Intelligence | Trust Gate | Reports To |
	// After splitting on "|": [0]="" [1]=" Role " [2]=" Responsibility " [3]=" Intelligence " ...
	checked := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 4 {
			continue
		}
		roleName := strings.TrimSpace(cols[1])
		tierName := strings.TrimSpace(cols[3])

		role, knownRole := roleByName[roleName]
		if !knownRole {
			continue
		}
		wantModel, knownTier := modelByTier[tierName]
		if !knownTier {
			t.Errorf("CLAUDE.md lists role %q with unknown Intelligence tier %q — update modelByTier in this test", roleName, tierName)
			checked++
			continue
		}
		got := PreferredModel(role)
		if got != wantModel {
			t.Errorf("model mismatch for %s: CLAUDE.md says %q (%s) but PreferredModel(%q) returns %q",
				roleName, tierName, wantModel, role, got)
		}
		checked++
	}

	if checked != len(roleByName) {
		t.Errorf("parsed %d roles from CLAUDE.md Intelligence tier table, expected %d — table rows may be missing or role names have changed",
			checked, len(roleByName))
	}
}

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
		{RolePM, "ROLE: PM"},
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
	roles := []Role{RolePM, RoleCTO, RoleGuardian, RoleResearcher, RoleArchitect,
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

	pm := soulValues(RolePM)
	if len(pm) != 5 {
		t.Errorf("PM soul values = %d, want 5", len(pm))
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
		{RolePM, sonnet},
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
