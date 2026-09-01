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

func exerciseSource(identity string) tlcbridge.Source {
	return tlcbridge.Source{
		Kind:       tlcbridge.SourceIssue,
		Identity:   identity,
		Repository: "transpara-ai/repo-x",
	}
}

func bindExerciseRecord(source tlcbridge.Source) (exerciseRecord, error) {
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
	bound, err := tlcbridge.Bind(source, raw)
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
	source := exerciseSource("https://github.com/transpara-ai/repo-x/issues/42")
	record, err := bindExerciseRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := bindExerciseRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	differentSource, err := bindExerciseRecord(
		exerciseSource("https://github.com/transpara-ai/repo-x/issues/43"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(record.IdempotencyKey, "tlc-brief-v1-") {
		t.Fatalf("idempotency key = %q, want versioned prefix", record.IdempotencyKey)
	}
	if replay.IdempotencyKey != record.IdempotencyKey {
		t.Fatalf("replay key = %q, want %q", replay.IdempotencyKey, record.IdempotencyKey)
	}
	if differentSource.IdempotencyKey == record.IdempotencyKey {
		t.Fatal("different source identity produced the replay idempotency key")
	}
	if record.WorktreeRepository != "transpara-ai/repo-x" ||
		record.PullRequestRepository != "transpara-ai/repo-x" {
		t.Fatalf("exercise record escaped RepoX: %+v", record)
	}
}

func ExampleBind_forExerciseRecord() {
	record, err := bindExerciseRecord(
		exerciseSource("https://github.com/transpara-ai/repo-x/issues/42"),
	)
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
