package tlcbridge_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

type exerciseRecord struct {
	IdempotencyKey        string
	WorktreeRepository    string
	PullRequestRepository string
}

func bindExerciseRecord() (exerciseRecord, error) {
	raw := []byte(`{
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
	bound, err := tlcbridge.Bind(
		tlcbridge.Source{
			Kind:       tlcbridge.SourceIssue,
			Identity:   "https://github.com/transpara-ai/repo-x/issues/42",
			Repository: "transpara-ai/repo-x",
		},
		raw,
	)
	if err != nil {
		return exerciseRecord{}, err
	}
	return exerciseRecord{
		IdempotencyKey:        bound.IdempotencyKey,
		WorktreeRepository:    bound.Effects.WorktreeRepository,
		PullRequestRepository: bound.Effects.PullRequestRepository,
	}, nil
}

func TestExerciseRecord(t *testing.T) {
	record, err := bindExerciseRecord()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(record.IdempotencyKey, "tlc-brief-v1-") {
		t.Fatalf("idempotency key = %q, want versioned prefix", record.IdempotencyKey)
	}
	if record.WorktreeRepository != "transpara-ai/repo-x" ||
		record.PullRequestRepository != "transpara-ai/repo-x" {
		t.Fatalf("exercise record escaped RepoX: %+v", record)
	}
}

func ExampleBind() {
	record, err := bindExerciseRecord()
	if err != nil {
		panic(err)
	}
	fmt.Println("idempotency key recorded:", record.IdempotencyKey != "")
	fmt.Println("worktree repository:", record.WorktreeRepository)
	fmt.Println("pull-request repository:", record.PullRequestRepository)
	// Output:
	// idempotency key recorded: true
	// worktree repository: transpara-ai/repo-x
	// pull-request repository: transpara-ai/repo-x
}
