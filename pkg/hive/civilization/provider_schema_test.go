package civilization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

// These are the provider API's strict-output constraints, not TLC policy.
// A normal JSON-schema validator accepts the open objects that caused the
// live Codex API to reject our previous schema before invoking the model.
func TestProviderSchemaMeetsStrictOutputObjectConstraints(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(providerResultSchema, &schema); err != nil {
		t.Fatal(err)
	}
	var check func(map[string]any, string)
	check = func(node map[string]any, path string) {
		if properties, ok := node["properties"].(map[string]any); ok {
			if node["additionalProperties"] != false {
				t.Errorf("%s must reject additional properties", path)
			}
			required := map[string]bool{}
			for _, name := range node["required"].([]any) {
				required[name.(string)] = true
			}
			if len(required) != len(properties) {
				t.Errorf("%s must require every property", path)
			}
			for name, child := range properties {
				if !required[name] {
					t.Errorf("%s.%s must be required (use null for optional values)", path, name)
				}
				check(child.(map[string]any), path+"."+name)
			}
		} else if node["type"] == "object" {
			t.Errorf("%s has an unconstrained object", path)
		}
		if items, ok := node["items"].(map[string]any); ok {
			check(items, path+"[]")
		}
	}
	check(schema, "result")
}

func TestProviderEnvelopeTextPreservesWorkflowExtensions(t *testing.T) {
	document := `{"schema_version":"tlc-envelope/v1","workflow":{"name":"transpara-tlc","version":"0.1.2"},"route":"Routine","brief":{"outcome":"Document smoke check","scope":[],"non_goals":[],"assumptions":[],"constraints":[],"tests":[],"next_action":"Confirm"},"future_workflow_data":{"nested":[{"value":"preserved"}]}}`
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := testCodexProvider(t)
	t.Setenv("FAKE_RESULT", fmt.Sprintf(`{"status":"passed","summary":"routed","tlc_envelope":%s,"changed_files":[],"checks":[],"review":null,"blocker":"","next_action":"confirm"}`, encoded))
	result, err := provider.Run(context.Background(), ProviderRequest{Operation: OperationRoute, RepositoryRoot: testRepository(t), Prompt: "Route the request."})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := tlcbridge.Bind(tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "schema-test", Repository: "transpara-ai/operation"}, result.TLCEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bound.CanonicalJSON), `"future_workflow_data":{"nested":[{"value":"preserved"}]}`) {
		t.Fatalf("workflow extension lost: %s", bound.CanonicalJSON)
	}
}

func TestProviderNullEnvelopeWorksForImplementation(t *testing.T) {
	provider, _ := testCodexProvider(t)
	t.Setenv("FAKE_RESULT", `{"status":"passed","summary":"implemented","tlc_envelope":null,"changed_files":["README.md"],"checks":[],"review":null,"blocker":"","next_action":"review"}`)
	result, err := provider.Run(context.Background(), ProviderRequest{Operation: OperationImplement, RepositoryRoot: testRepository(t), Prompt: "Implement."})
	if err != nil || result.TLCEnvelope != nil || result.Review != nil {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestProviderRejectsMalformedEnvelopeText(t *testing.T) {
	for _, document := range []string{"", "null", "[]", "42", "{} {}", "{broken"} {
		encoded, _ := json.Marshal(document)
		raw := fmt.Sprintf(`{"status":"passed","summary":"routed","tlc_envelope":%s,"changed_files":[],"checks":[],"review":null,"blocker":"","next_action":"confirm"}`, encoded)
		if _, err := decodeProviderResult([]byte(raw)); err == nil {
			t.Errorf("accepted invalid envelope text %q", document)
		}
	}
}
