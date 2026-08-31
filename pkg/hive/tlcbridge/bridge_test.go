package tlcbridge

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func routineBrief() []byte {
	return []byte(`{
		"schema_version":"tlc-change-brief/v1",
		"route":"Routine",
		"brief":{
			"outcome":"Fix the displayed typo",
			"scope":["settings copy"],
			"non_goals":["other copy"],
			"assumptions":[],
			"constraints":[],
			"tests":["run the affected text check"],
			"next_action":"implement the typo fix"
		}
	}`)
}

func designedBrief() []byte {
	return bytes.Replace(routineBrief(), []byte(`"route":"Routine"`), []byte(`"route":"Designed"`), 1)
}

func issueSource() Source {
	return Source{
		Kind:       SourceIssue,
		Identity:   "https://github.com/transpara-ai/repo-x/issues/42",
		Repository: "transpara-ai/repo-x",
	}
}

func TestIssueInRepoXBindsEveryRepositoryEffectToRepoX(t *testing.T) {
	bound, err := Bind(issueSource(), routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	if bound.Effects.WorktreeRepository != issueSource().Repository ||
		bound.Effects.PullRequestRepository != issueSource().Repository {
		t.Fatalf("effects = %+v, want both in %q", bound.Effects, issueSource().Repository)
	}
	if strings.Contains(string(routineBrief()), issueSource().Repository) {
		t.Fatal("TLC public brief unexpectedly contains Hive repository routing")
	}
}

func TestBriefCannotInjectHiveExecutionOrRepositoryState(t *testing.T) {
	for _, field := range []string{"source_chain", "retry", "worktree", "pull_request_repository", "provider", "effect_state"} {
		mutated := bytes.Replace(routineBrief(), []byte(`"route":"Routine",`), []byte(`"route":"Routine","`+field+`":"injected",`), 1)
		if _, err := Bind(issueSource(), mutated); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("field %q error = %v, want unknown-field refusal", field, err)
		}
	}
}

func TestBindingIsDeterministicAndSourceSpecificForSafeRetry(t *testing.T) {
	first, err := Bind(issueSource(), routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	again, err := Bind(issueSource(), routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotencyKey != again.IdempotencyKey {
		t.Fatalf("same source and brief produced %q then %q", first.IdempotencyKey, again.IdempotencyKey)
	}
	other := issueSource()
	other.Identity = "https://github.com/transpara-ai/repo-x/issues/43"
	changed, err := Bind(other, routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	if changed.IdempotencyKey == first.IdempotencyKey {
		t.Fatal("different source identities shared an idempotency key")
	}
}

func TestBindingNormalizesTextBeforeCanonicalIdempotency(t *testing.T) {
	canonical := bytes.Replace(routineBrief(), []byte("Fix the displayed typo"), []byte("Fix café"), 1)
	variant := bytes.Replace(routineBrief(), []byte("Fix the displayed typo"), []byte("  Fix cafe\u0301  "), 1)
	first, err := Bind(issueSource(), canonical)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Bind(issueSource(), variant)
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("equivalent normalized briefs produced %q and %q", first.IdempotencyKey, second.IdempotencyKey)
	}
	if second.Change.Brief.Outcome != "Fix café" {
		t.Fatalf("normalized outcome = %q, want NFC-trimmed text", second.Change.Brief.Outcome)
	}
}

func TestBindingRejectsDuplicatesAfterTextNormalization(t *testing.T) {
	variant := bytes.Replace(
		routineBrief(),
		[]byte(`"scope":["settings copy"]`),
		[]byte(`"scope":["café","  cafe\u0301  "]`),
		1,
	)
	if _, err := Bind(issueSource(), variant); err == nil || !strings.Contains(err.Error(), "duplicate item") {
		t.Fatalf("Bind error = %v, want normalized duplicate refusal", err)
	}
}

func TestDuplicateJSONObjectKeysAreRejectedBeforeBinding(t *testing.T) {
	tests := map[string][]byte{
		"route": bytes.Replace(
			routineBrief(),
			[]byte(`"route":"Routine",`),
			[]byte(`"route":"Critical","route":"Routine",`),
			1,
		),
		"nested brief field": bytes.Replace(
			routineBrief(),
			[]byte(`"outcome":"Fix the displayed typo",`),
			[]byte(`"outcome":"Revoke credentials","outcome":"Fix the displayed typo",`),
			1,
		),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Bind(issueSource(), raw); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
				t.Fatalf("Bind error = %v, want duplicate-key refusal", err)
			}
		})
	}
}

func TestSourceIdentityAndRepositoryAreNormalizedBeforeBinding(t *testing.T) {
	source := issueSource()
	source.Identity = "  " + source.Identity + "  "
	source.Repository = "  " + source.Repository + "  "
	bound, err := Bind(source, routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	if bound.Source.Identity != issueSource().Identity || bound.Source.Repository != issueSource().Repository {
		t.Fatalf("normalized source = %+v, want %+v", bound.Source, issueSource())
	}
	if bound.Effects.WorktreeRepository != issueSource().Repository || bound.Effects.PullRequestRepository != issueSource().Repository {
		t.Fatalf("effects use unnormalized repository: %+v", bound.Effects)
	}
}

func TestSourceIdentityUnicodeNormalizationStabilizesIdempotency(t *testing.T) {
	canonicalSource := issueSource()
	canonicalSource.Identity = "issue:café"
	variantSource := issueSource()
	variantSource.Identity = "  issue:cafe\u0301  "
	canonical, err := Bind(canonicalSource, routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	variant, err := Bind(variantSource, routineBrief())
	if err != nil {
		t.Fatal(err)
	}
	if canonical.IdempotencyKey != variant.IdempotencyKey {
		t.Fatalf("equivalent source identities produced %q and %q", canonical.IdempotencyKey, variant.IdempotencyKey)
	}
}

func TestInvalidUTF8IsRejectedBeforeBinding(t *testing.T) {
	raw := append(append([]byte(nil), routineBrief()...), 0xff)
	if _, err := Bind(issueSource(), raw); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("Bind error = %v, want UTF-8 refusal", err)
	}
}

func TestHumanAndIssueUseTheSameTLCBriefContract(t *testing.T) {
	issue, err := Bind(issueSource(), designedBrief())
	if err != nil {
		t.Fatal(err)
	}
	humanSource := Source{Kind: SourceHuman, Identity: "conversation:123", Repository: "transpara-ai/repo-x"}
	human, err := Bind(humanSource, designedBrief())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(issue.Change, human.Change) {
		t.Fatalf("issue and Human sources produced different TLC changes: issue=%+v human=%+v", issue.Change, human.Change)
	}
}

func TestFableIsOptionalHumanSelectedMaximumAndAdvisory(t *testing.T) {
	fable := []byte(`,"fable":{"selected_by_human":true,"effort":"maximum","maximum_collaborations":1,"advisory_only":true,"review_credit":false}}`)
	selected := bytes.Replace(designedBrief(), []byte("\n\t}"), fable, 1)
	bound, err := Bind(issueSource(), selected)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Change.Fable == nil || bound.Change.Fable.Effort != "maximum" {
		t.Fatalf("Fable = %+v, want selected Maximum effort", bound.Change.Fable)
	}
	invalid := bytes.Replace(selected, []byte(`"selected_by_human":true`), []byte(`"selected_by_human":false`), 1)
	if _, err := Bind(issueSource(), invalid); err == nil {
		t.Fatal("non-Human-selected Fable was accepted")
	}
	if _, err := Bind(issueSource(), bytes.Replace(routineBrief(), []byte("\n\t}"), fable, 1)); err == nil {
		t.Fatal("Routine Fable collaboration was accepted")
	}
}

func TestCriticalRequiresNamedObservedTriggerAndSeparateEffectAuthority(t *testing.T) {
	critical := bytes.Replace(routineBrief(), []byte(`"route":"Routine"`), []byte(`"route":"Critical"`), 1)
	criticalObject := []byte(`,"critical":{"observations":[{"trigger":"credentials_auth_crypto_secrets","evidence":"replaces and revokes service keys"}],"separate_effect_authority":true}}`)
	critical = bytes.Replace(critical, []byte("\n\t}"), criticalObject, 1)
	bound, err := Bind(issueSource(), critical)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Change.Route != RouteCritical || bound.Change.Critical == nil || !bound.Change.Critical.SeparateEffectAuthority {
		t.Fatalf("critical change = %+v", bound.Change)
	}
	withoutAuthority := bytes.Replace(critical, []byte(`"separate_effect_authority":true`), []byte(`"separate_effect_authority":false`), 1)
	if _, err := Bind(issueSource(), withoutAuthority); err == nil {
		t.Fatal("Critical brief without separate effect authority was accepted")
	}
}

func TestMalformedOrUnsafeSourceFailsBeforeBinding(t *testing.T) {
	for _, source := range []Source{
		{Kind: "scrape", Identity: "x", Repository: "transpara-ai/repo-x"},
		{Kind: SourceIssue, Identity: "", Repository: "transpara-ai/repo-x"},
		{Kind: SourceIssue, Identity: "issue:1", Repository: "../repo-x"},
		{Kind: SourceIssue, Identity: "issue:1", Repository: "transpara-ai/.git"},
		{Kind: SourceIssue, Identity: "issue:1", Repository: "attacker/evil-repo"},
		{Kind: SourceIssue, Identity: "issue:1", Repository: "transpara-ai/re..po"},
		{Kind: SourceIssue, Identity: "issue:1", Repository: "transpara-ai/repo."},
	} {
		if _, err := Bind(source, routineBrief()); err == nil {
			t.Fatalf("unsafe source accepted: %+v", source)
		}
	}
}

func TestNullBriefArraysAreRejected(t *testing.T) {
	mutated := bytes.Replace(routineBrief(), []byte(`"scope":["settings copy"]`), []byte(`"scope":null`), 1)
	if _, err := Bind(issueSource(), mutated); err == nil || !strings.Contains(err.Error(), "brief.scope must be an array") {
		t.Fatalf("null scope error = %v, want array refusal", err)
	}
}
