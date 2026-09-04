package civilization

import (
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func eligibleCandidate(t *testing.T) MergeCandidate {
	t.Helper()
	bound, err := tlcbridge.Bind(tlcbridge.Source{
		Kind: tlcbridge.SourceIssue, Identity: "issue:1", Repository: "transpara-ai/hive",
	}, []byte(`{
  "schema_version":"tlc-envelope/v1",
  "workflow":{"name":"transpara-tlc","version":"0.1.1"},
  "route":"Routine",
  "brief":{"outcome":"Repair typo","scope":[],"non_goals":[],"assumptions":[],"constraints":[],"tests":["go test ./..."],"next_action":"Implement"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	head := "0123456789abcdef0123456789abcdef01234567"
	return MergeCandidate{
		BoundRequest: bound, Repository: "transpara-ai/hive", PullRequestNumber: 42,
		CreatedByCivilization: true, Open: true, HeadSHA: head, ReviewedHeadSHA: head,
		ValidatedHeadSHA: head, RequiredChecksPassing: true, OrdinaryReviewPassing: true,
		ChangedFiles: []string{"docs/README.md"}, ExpectedChangedFiles: []string{"docs/README.md"},
		ChangedFilesComplete: true,
	}
}

func enabledPolicy() AutoMergePolicy {
	return AutoMergePolicy{
		Enabled: true, AuthorityRef: "human:auto-merge:2026-09-03",
		Repositories:   map[string]struct{}{"transpara-ai/hive": {}},
		ProtectedPaths: DefaultProtectedPaths(),
	}
}

func TestEvaluateAutoMergeEligibleRoutine(t *testing.T) {
	decision := EvaluateAutoMerge(enabledPolicy(), eligibleCandidate(t))
	if !decision.Eligible || len(decision.Reasons) != 0 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestEvaluateAutoMergeFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AutoMergePolicy, *MergeCandidate)
	}{
		{"disabled", func(p *AutoMergePolicy, _ *MergeCandidate) { p.Enabled = false }},
		{"missing authority", func(p *AutoMergePolicy, _ *MergeCandidate) { p.AuthorityRef = "" }},
		{"designed", func(_ *AutoMergePolicy, c *MergeCandidate) { c.BoundRequest.Envelope.Route = "Designed" }},
		{"critical", func(_ *AutoMergePolicy, c *MergeCandidate) { c.BoundRequest.Envelope.Route = "Critical" }},
		{"external PR", func(_ *AutoMergePolicy, c *MergeCandidate) { c.CreatedByCivilization = false }},
		{"wrong repo", func(_ *AutoMergePolicy, c *MergeCandidate) { c.Repository = "transpara-ai/site" }},
		{"draft", func(_ *AutoMergePolicy, c *MergeCandidate) { c.Draft = true }},
		{"head race", func(_ *AutoMergePolicy, c *MergeCandidate) { c.HeadSHA = "abcdef0123456789abcdef0123456789abcdef01" }},
		{"checks", func(_ *AutoMergePolicy, c *MergeCandidate) { c.RequiredChecksPassing = false }},
		{"review", func(_ *AutoMergePolicy, c *MergeCandidate) { c.OrdinaryReviewPassing = false }},
		{"blocker", func(_ *AutoMergePolicy, c *MergeCandidate) { c.UnresolvedBlockers = 1 }},
		{"intervention", func(_ *AutoMergePolicy, c *MergeCandidate) { c.OpenInterventions = 1 }},
		{"empty changed files", func(_ *AutoMergePolicy, c *MergeCandidate) { c.ChangedFiles = nil }},
		{"incomplete changed files", func(_ *AutoMergePolicy, c *MergeCandidate) { c.ChangedFilesComplete = false }},
		{"mismatched changed files", func(_ *AutoMergePolicy, c *MergeCandidate) {
			c.ExpectedChangedFiles = []string{"docs/OTHER.md"}
		}},
		{"protected workflow", func(_ *AutoMergePolicy, c *MergeCandidate) { c.ChangedFiles = []string{".github/workflows/ci.yml"} }},
		{"protected auth", func(_ *AutoMergePolicy, c *MergeCandidate) { c.ChangedFiles = []string{"internal/auth/session.go"} }},
		{"deep protected auth", func(_ *AutoMergePolicy, c *MergeCandidate) {
			c.ChangedFiles = []string{"services/web/internal/auth/session.go"}
		}},
		{"path traversal", func(_ *AutoMergePolicy, c *MergeCandidate) { c.ChangedFiles = []string{"../README.md"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := enabledPolicy()
			candidate := eligibleCandidate(t)
			tc.mutate(&policy, &candidate)
			decision := EvaluateAutoMerge(policy, candidate)
			if decision.Eligible || len(decision.Reasons) == 0 {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}
