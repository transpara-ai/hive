package roles

import "testing"

func TestSystemPrompt(t *testing.T) {
	tests := []struct {
		role Role
		want string // substring that should be present
	}{
		{RoleCTO, "CTO of the hive"},
		{RoleGuardian, "Guardian"},
		{RoleResearcher, "Researcher"},
		{RoleArchitect, "Architect"},
		{RoleBuilder, "Builder"},
		{RoleReviewer, "Reviewer"},
		{RoleTester, "Tester"},
		{RoleIntegrator, "Integrator"},
		{Role("unknown"), "hive agent"},
	}

	for _, tt := range tests {
		prompt := SystemPrompt(tt.role)
		if prompt == "" {
			t.Errorf("SystemPrompt(%q) is empty", tt.role)
		}
		found := false
		for i := 0; i <= len(prompt)-len(tt.want); i++ {
			if prompt[i:i+len(tt.want)] == tt.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SystemPrompt(%q) does not contain %q", tt.role, tt.want)
		}
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
}
