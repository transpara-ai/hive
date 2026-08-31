package tlcbridge_test

import (
	"testing"

	hive "github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func TestRepositoryAcceptanceConformsToHiveIntake(t *testing.T) {
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
	repositories := []string{
		"transpara-ai/hive",
		"transpara-ai/repo_x.2",
		"transpara-ai/répôt",
		"attacker/hive",
		"transpara-ai/.git",
		"transpara-ai/re..po",
		"transpara-ai/repo.",
		"transpara-ai/repo two",
		"transpara-ai/repo/control\n",
		"transpara-ai/one/two",
	}
	for _, repository := range repositories {
		t.Run(repository, func(t *testing.T) {
			_, err := tlcbridge.Bind(
				tlcbridge.Source{
					Kind:       tlcbridge.SourceIssue,
					Identity:   "issue:1",
					Repository: repository,
				},
				brief,
			)
			if got, want := err == nil, hive.ValidTransparaAIRepo(repository); got != want {
				t.Fatalf("bridge acceptance = %t, Hive intake acceptance = %t (error: %v)", got, want, err)
			}
		})
	}
}
