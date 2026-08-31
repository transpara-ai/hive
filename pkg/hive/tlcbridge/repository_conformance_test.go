package tlcbridge_test

import (
	"testing"

	hive "github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func TestRepositoryAcceptanceAndOutputConformToHiveIntake(t *testing.T) {
	brief := []byte(`{
		"schema_version":"tlc-change-brief/v1",
		"route":"Routine",
		"brief":{
			"outcome":"Fix the displayed typo",
			"scope":["settings copy"],
			"non_goals":[],
			"assumptions":[],
			"constraints":[],
			"tests":["run the affected text check"],
			"next_action":"implement the typo fix"
		}
	}`)
	tests := []struct {
		repository string
		want       bool
	}{
		{repository: "transpara-ai/hive", want: true},
		{repository: "transpara-ai/repo_x.2", want: true},
		{repository: "transpara-ai/répôt", want: true},
		{repository: "  transpara-ai/hive  ", want: true},
		{repository: "transpara-ai/cafe\u0301", want: true},
		{repository: "attacker/hive"},
		{repository: "transpara-ai/.git"},
		{repository: "transpara-ai/re..po"},
		{repository: "transpara-ai/repo."},
		{repository: "transpara-ai/repo two"},
		{repository: "transpara-ai/repo/control\n"},
		{repository: "transpara-ai/one/two"},
	}
	for _, test := range tests {
		t.Run(test.repository, func(t *testing.T) {
			bound, err := tlcbridge.Bind(
				tlcbridge.Source{
					Kind:       tlcbridge.SourceIssue,
					Identity:   "issue:1",
					Repository: test.repository,
				},
				brief,
			)
			if got := err == nil; got != test.want {
				t.Fatalf("bridge acceptance = %t, want %t (error: %v)", got, test.want, err)
			}
			if err == nil && (!hive.ValidTransparaAIRepo(bound.Source.Repository) ||
				!hive.ValidTransparaAIRepo(bound.Effects.WorktreeRepository) ||
				!hive.ValidTransparaAIRepo(bound.Effects.PullRequestRepository)) {
				t.Fatalf("accepted repository escaped Hive invariants: %+v", bound)
			}
		})
	}
}
