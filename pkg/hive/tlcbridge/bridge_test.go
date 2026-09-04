package tlcbridge

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func testEnvelope() []byte {
	return []byte(`{
  "schema_version":"tlc-envelope/v1",
  "workflow":{"name":"transpara-tlc","version":"0.1.1"},
  "route":"Routine",
  "brief":{"outcome":"Fix the bounded defect","scope":["repo change"],"non_goals":[],"assumptions":[],"constraints":[],"tests":["go test ./..."],"next_action":"Implement"},
  "route_evidence":{"owned_by":"tlc","future_field":true}
}`)
}

func TestBindPreservesOpaqueTLCDataAndConfinesEffects(t *testing.T) {
	bound, err := Bind(Source{Kind: SourceIssue, Identity: "issue:42", Repository: "transpara-ai/hive"}, testEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if bound.Effects.WorktreeRepository != "transpara-ai/hive" || bound.Effects.PullRequestRepository != "transpara-ai/hive" {
		t.Fatalf("effects escaped source repository: %+v", bound.Effects)
	}
	if !bytes.Contains(bound.CanonicalJSON, []byte(`"future_field":true`)) {
		t.Fatalf("opaque TLC data was not preserved: %s", bound.CanonicalJSON)
	}
	if !strings.HasPrefix(bound.IdempotencyKey, "tlc-envelope-v1-") {
		t.Fatalf("unexpected idempotency key %q", bound.IdempotencyKey)
	}
}

func TestBindIsDeterministicAcrossJSONFormatting(t *testing.T) {
	source := Source{Kind: SourceHuman, Identity: "human:Michael", Repository: "transpara-ai/site"}
	first, err := Bind(source, testEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, testEnvelope()); err != nil {
		t.Fatal(err)
	}
	second, err := Bind(source, compact.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("format changed identity: %q != %q", first.IdempotencyKey, second.IdempotencyKey)
	}
}

func TestBindRejectsRepositoryInjection(t *testing.T) {
	raw := bytes.Replace(testEnvelope(), []byte(`"route_evidence":{`), []byte(`"repository":"attacker/repo","route_evidence":{`), 1)
	bound, err := Bind(Source{Kind: SourceIssue, Identity: "issue:42", Repository: "transpara-ai/work"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Effects.WorktreeRepository != "transpara-ai/work" || bound.Effects.PullRequestRepository != "transpara-ai/work" {
		t.Fatalf("envelope overrode source repository: %+v", bound.Effects)
	}
}

func TestBindAllowsFutureRouteWithoutEmbeddingPolicy(t *testing.T) {
	raw := bytes.Replace(testEnvelope(), []byte(`"Routine"`), []byte(`"FutureRoute"`), 1)
	bound, err := Bind(Source{Kind: SourceOrder, Identity: "order:1", Repository: "transpara-ai/docs"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Envelope.Route != "FutureRoute" {
		t.Fatalf("route = %q", bound.Envelope.Route)
	}
}

func TestBindRejectsInvalidStableTransport(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "array", raw: `[]`, want: "one JSON object"},
		{name: "multiple", raw: `{}` + `{}`, want: "multiple JSON values"},
		{name: "duplicate", raw: `{"schema_version":"tlc-envelope/v1","schema_version":"other"}`, want: "duplicate object key"},
		{name: "schema", raw: strings.Replace(string(testEnvelope()), EnvelopeSchemaVersion, "unknown/v9", 1), want: "unsupported TLC transport schema"},
		{name: "missing tests", raw: strings.Replace(string(testEnvelope()), `"tests":["go test ./..."]`, `"tests":null`, 1), want: "tests must be an array"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Bind(Source{Kind: SourceIssue, Identity: "issue:1", Repository: "transpara-ai/hive"}, []byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
