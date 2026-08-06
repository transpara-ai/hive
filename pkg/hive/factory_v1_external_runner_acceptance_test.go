package hive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const auditedFactoryV1DemoRunnerSHA256 = "b80c6121e54c4fcd55047a1ca55680a9a6e9d6f2cb209d3f53169e8c9a9398a8"

// TestFactoryV1ConfiguredDemoBindingsAcceptance is intentionally inert during
// hermetic verification. An operator may provide a non-secret exact-binding
// fixture to exercise both digest-pinned local bindings at StageIngestWork.
// Runner state and the copied config stay inside t.TempDir; no GitHub command
// is reachable on this stage.
func TestFactoryV1ConfiguredDemoBindingsAcceptance(t *testing.T) {
	rawFixture := strings.TrimSpace(os.Getenv("FACTORY_V1_REAL_BINDING_ACCEPTANCE_JSON"))
	if rawFixture == "" {
		t.Skip("FACTORY_V1_REAL_BINDING_ACCEPTANCE_JSON is not configured")
	}
	var fixture struct {
		NonProduction                bool                      `json:"non_production"`
		ConfigPath                   string                    `json:"config_path"`
		RepositoryRoot               string                    `json:"repository_root"`
		TargetRepository             string                    `json:"target_repository"`
		Author                       factoryv1.ProviderBinding `json:"author"`
		Reviewer                     factoryv1.ProviderBinding `json:"reviewer"`
		AuthorEnvironmentAllowlist   []string                  `json:"author_environment_allowlist"`
		ReviewerEnvironmentAllowlist []string                  `json:"reviewer_environment_allowlist"`
	}
	decoder := json.NewDecoder(strings.NewReader(rawFixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode real-binding acceptance fixture: %v", err)
	}
	if !fixture.NonProduction || fixture.ConfigPath == "" || fixture.RepositoryRoot == "" || fixture.TargetRepository == "" {
		t.Fatal("real-binding acceptance requires explicit non-production mode, config, repository root, and repository identity")
	}
	for _, binding := range []factoryv1.ProviderBinding{fixture.Author, fixture.Reviewer} {
		if binding.ExecutableSHA256 != auditedFactoryV1DemoRunnerSHA256 {
			t.Fatalf("provider %s digest %s is not the audited deterministic demo runner", binding.ProviderID, binding.ExecutableSHA256)
		}
		if _, err := ResolveFactoryV1ProviderBinding(binding.ProviderID, binding.Family, binding.ExecutableRealpath, binding.ExecutableSHA256, binding.ModelID, binding.CredentialSourceID); err != nil {
			t.Fatalf("resolve exact provider %s: %v", binding.ProviderID, err)
		}
	}

	configRaw, err := os.ReadFile(fixture.ConfigPath)
	if err != nil {
		t.Fatalf("read runner config: %v", err)
	}
	providerArgs := func(providerID string) []string {
		t.Helper()
		stateDir := filepath.Join(t.TempDir(), "state-"+factoryv1.HashText(providerID)[:12])
		if err := os.Mkdir(stateDir, 0o700); err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := json.Unmarshal(configRaw, &config); err != nil {
			t.Fatalf("decode runner config copy: %v", err)
		}
		config["reviewer_evidence_dir"] = "reviewer-artifacts"
		encoded, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		configCopy := filepath.Join(stateDir, "config.json")
		if err := os.WriteFile(configCopy, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		return []string{"--state-dir", stateDir, "--config", configCopy}
	}
	authorArgs := providerArgs(fixture.Author.ProviderID)
	reviewerArgs := providerArgs(fixture.Reviewer.ProviderID)
	for _, key := range append(append([]string(nil), fixture.AuthorEnvironmentAllowlist...), fixture.ReviewerEnvironmentAllowlist...) {
		if strings.TrimSpace(key) != "" {
			t.Setenv(strings.TrimSpace(key), "selected-acceptance-sentinel")
		}
	}
	t.Setenv("FACTORY_V1_ACCEPTANCE_UNSELECTED", "unselected-acceptance-sentinel")
	runner, err := NewFactoryV1ExternalRunner([]FactoryV1RunnerProvider{
		{Binding: fixture.Author, Args: authorArgs, EnvironmentAllowlist: fixture.AuthorEnvironmentAllowlist, Timeout: time.Minute},
		{Binding: fixture.Reviewer, Args: reviewerArgs, EnvironmentAllowlist: fixture.ReviewerEnvironmentAllowlist, Timeout: time.Minute},
	}, 2*1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	order := factoryv1.FactoryOrder{
		DocID: "FO-REAL-BINDING-ACCEPTANCE", Version: "1.0.0", Status: "approved", Title: "Digest-pinned provider environment acceptance",
		Channel: factoryv1.ChannelCompletedOrder, TargetRepository: fixture.TargetRepository,
		SourceReferences:   []factoryv1.SourceReference{{Kind: "test", Identity: "acceptance:real-bindings", URI: "test://real-bindings", SHA256: strings.Repeat("a", 64)}},
		Requirements:       []factoryv1.Requirement{{ID: "R1", Statement: "Invoke the exact digest-pinned bindings at ingest_work.", Rationale: "Prove compatible environment minimization."}},
		AcceptanceCriteria: []factoryv1.AcceptanceCriterion{{ID: "AC1", Statement: "Both bindings return strict JSON success without unselected keys.", VerificationMethod: "This acceptance test", RiskClass: "low"}},
		TestPlan:           []string{"run exact binding acceptance"}, Constraints: []string{"local", "non-production"}, NonGoals: []string{"GitHub mutation"}, ExpectedOutputs: []string{"name-only environment evidence"},
		Authority: factoryv1.AuthorityScope{ActorID: "human-operator", AllowedActions: []string{"test.execute"}, TargetRepositories: []string{fixture.TargetRepository}, NonProductionOnly: true},
		Budget:    factoryv1.BudgetLimit{MaxAttempts: 2, MaxTokens: 1000, MaxCostMicros: 1000},
	}
	document, err := factoryv1.Canonicalize(order)
	if err != nil {
		t.Fatal(err)
	}
	attemptID, err := factoryv1.AttemptID(document.SHA256, factoryv1.StageIngestWork, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []factoryv1.ProviderBinding{fixture.Author, fixture.Reviewer} {
		result, err := runner.Execute(context.Background(), factoryv1.RunRequest{
			Operation: "execute", Order: order, OrderMarkdown: document.Markdown, DocumentSHA256: document.SHA256,
			Stage: factoryv1.StageIngestWork, AttemptID: attemptID, Ordinal: 1, RepositoryRoot: fixture.RepositoryRoot,
			AuthorityScope: order.Authority, BudgetRemaining: factoryv1.BudgetProjection{RemainingAttempts: 2, RemainingTokens: 1000, RemainingCostMicros: 1000},
			Peers: factoryv1.PeersForStage(factoryv1.StageIngestWork), Provider: provider,
		})
		if err != nil {
			t.Fatalf("execute exact provider %s: %v", provider.ProviderID, err)
		}
		if result.Status != factoryv1.RunnerPassed {
			t.Fatalf("provider %s status = %s", provider.ProviderID, result.Status)
		}
		var environment factoryv1.Evidence
		for _, item := range result.Evidence {
			if item.Kind == "runner_environment" {
				environment = item
			}
		}
		if environment.Reference != "provider:"+provider.ProviderID || strings.Contains(environment.Metadata["selected_keys"], "FACTORY_V1_ACCEPTANCE_UNSELECTED") {
			t.Fatalf("provider %s environment evidence = %+v", provider.ProviderID, environment)
		}
		for _, value := range environment.Metadata {
			if strings.Contains(value, "acceptance-sentinel") {
				t.Fatalf("provider %s serialized an environment value", provider.ProviderID)
			}
		}
		t.Logf("provider=%s selected_keys=%s stdout=%s stderr=%s", provider.ProviderID, environment.Metadata["selected_keys"], environment.Metadata["stdout_sha256"], environment.Metadata["stderr_sha256"])
	}
}
