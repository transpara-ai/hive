package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runnerSuiteTestManifest returns the canonical valid manifest for the managed
// ready-PR finalizer posture as a mutable generic map so each fail-closed case
// can corrupt exactly one property.
func runnerSuiteTestManifest() map[string]any {
	component := func(id, stdinKind, stdoutKind string, boundaries []string) map[string]any {
		return map[string]any{
			"id":                   id,
			"command":              "runners/" + id + ".placeholder",
			"argv":                 []any{"--stdin-json"},
			"timeout":              "10m",
			"stdin_kind":           stdinKind,
			"stdout_kind":          stdoutKind,
			"required_env":         []any{},
			"forbidden_env":        []any{"ANTHROPIC_API_KEY", "HIVE_ANTHROPIC_API_KEY", "GITHUB_TOKEN"},
			"authority_boundaries": toAnySlice(boundaries),
			"fixtures": map[string]any{
				"stdin":  "examples/" + id + "/stdin.json",
				"stdout": "examples/" + id + "/stdout.json",
			},
		}
	}
	return map[string]any{
		"kind":                "issue_scan_runner_suite_package_manifest",
		"suite_id":            "issue-scan-runner-suite",
		"lifecycle_version":   "civilization_issue_to_human_ready_pr_v0.9",
		"terminal_stage_path": "managed_ready_pr_finalizer",
		"validation_command":  "hive factory validate-issue-scan-runner-suite --package packages/issue-scan-runner-suite",
		"components": []any{
			component("stage_role_output_runner",
				"issue_scan_stage_role_output_runner_context",
				"hive.IssueScanStageRoleOutputRunnerResult",
				[]string{"does not implement code", "does not run adversarial review", "does not create, mark ready, approve, merge, or deploy a PR"}),
			component("implementation_runner",
				"issue_scan_implementation_runner_context",
				"hive.IssueScanImplementationRunnerResult",
				[]string{"may modify the target worktree/branch only within the supplied repo context", "does not create, mark ready, approve, merge, or deploy a PR"}),
			component("adversarial_review_runner",
				"issue_scan_adversarial_review_context",
				"hive.IssueScanAdversarialReviewReceipt",
				[]string{"does not repair blockers", "does not mark PRs ready", "does not approve, merge, or deploy"}),
			component("blocker_repair_runner",
				"issue_scan_blocker_repair_runner_context",
				"hive.IssueScanBlockerRepairRunnerResult",
				[]string{"may modify the target worktree/branch only within the supplied repo context", "does not create, mark ready, approve, merge, or deploy a PR"}),
			component("ready_state_review_runner",
				"issue_scan_ready_state_review_context",
				"hive.IssueScanReadyStateReviewReceipt",
				[]string{"used only with --issue-scan-ready-pr-mark-ready", "does not approve, merge, deploy, or perform production migrations"}),
		},
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// runnerSuiteTestFixtures returns synthetic stdin/stdout fixture documents per
// component id. All values are inert and public-safe.
func runnerSuiteTestFixtures() map[string]map[string]string {
	const lifecycle = "civilization_issue_to_human_ready_pr_v0.9"
	return map[string]map[string]string{
		"stage_role_output_runner": {
			"stdin":  `{"kind":"issue_scan_stage_role_output_runner_context","lifecycle_version":"` + lifecycle + `","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","stage_id":"research","stage_task_id":"task-synthetic-0001","stage_index":0,"stage_count":4}`,
			"stdout": `{"role_outputs":[{"role":"researcher","summary":"Synthetic research summary.","outputs":[{"key":"research_findings","summary":"Synthetic finding."}]}]}`,
		},
		"implementation_runner": {
			"stdin":  `{"kind":"issue_scan_implementation_runner_context","lifecycle_version":"` + lifecycle + `","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","repo_path":"workspace/synthetic-example","implementation_task_id":"task-synthetic-0002","implementation_stage_task_id":"task-synthetic-0003","design_stage_task_id":"task-synthetic-0004","design_runtime_evidence_ref":"evidence-synthetic-0001"}`,
			"stdout": `{"operate_result_body":"branch: feat/synthetic-change\ncommit: 0000000000000000000000000000000000000001\n\n 1 file changed, 1 insertion(+)\n","completion_summary":"Synthetic implementation completion."}`,
		},
		"adversarial_review_runner": {
			"stdin":  `{"kind":"issue_scan_adversarial_review_context","lifecycle_version":"` + lifecycle + `","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","implementation_task_id":"task-synthetic-0002","review_stage_task_id":"task-synthetic-0005","operate_branch":"feat/synthetic-change","operate_commit":"0000000000000000000000000000000000000001","changed_files_summary":"1 file changed (synthetic)"}`,
			"stdout": `{"repository":"transpara-ai/synthetic-example","review_ref":"synthetic-review-0001","reviewed_head_sha":"0000000000000000000000000000000000000001","verdict":"approve","summary":"Synthetic exact-head review.","issues":[],"confidence":0.9}`,
		},
		"blocker_repair_runner": {
			"stdin":  `{"kind":"issue_scan_blocker_repair_runner_context","lifecycle_version":"` + lifecycle + `","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","repo_path":"workspace/synthetic-example","implementation_task_id":"task-synthetic-0002","implementation_stage_task_id":"task-synthetic-0003","review_stage_task_id":"task-synthetic-0005","blocker_stage_task_id":"task-synthetic-0006","request_changes_review_event_id":"event-synthetic-0001","request_changes_review_summary":"Synthetic blocker summary.","request_changes_review_issues":["synthetic blocker"],"request_changes_review_confidence":0.8,"reopen_event_id":"event-synthetic-0002","reopen_reason":"synthetic reopen","previous_operate_branch":"feat/synthetic-change","previous_operate_commit":"0000000000000000000000000000000000000001","previous_changed_files_summary":"1 file changed (synthetic)"}`,
			"stdout": `{"operate_result_body":"branch: feat/synthetic-change\ncommit: 0000000000000000000000000000000000000002\n\n 1 file changed, 1 insertion(+)\n","completion_summary":"Synthetic blocker repair completion."}`,
		},
		"ready_state_review_runner": {
			"stdin":  `{"kind":"issue_scan_ready_state_review_context","lifecycle_version":"` + lifecycle + `","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","pr_number":1,"pr_url":"https://github.com/transpara-ai/synthetic-example/pull/1","ready_stage_task_id":"task-synthetic-0007","implementation_task_id":"task-synthetic-0002","operate_commit":"0000000000000000000000000000000000000001"}`,
			"stdout": `{"review_ref":"synthetic-review-0002","reviewed_head_sha":"0000000000000000000000000000000000000001","status":"pass"}`,
		},
	}
}

// writeRunnerSuiteTestPackage materialises a manifest + fixtures into dir.
func writeRunnerSuiteTestPackage(t *testing.T, dir string, manifest map[string]any, fixtures map[string]map[string]string) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	for id, docs := range fixtures {
		exampleDir := filepath.Join(dir, "examples", id)
		if err := os.MkdirAll(exampleDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", exampleDir, err)
		}
		for name, doc := range docs {
			if err := os.WriteFile(filepath.Join(exampleDir, name+".json"), []byte(doc+"\n"), 0o644); err != nil {
				t.Fatalf("write fixture %s/%s: %v", id, name, err)
			}
		}
	}
}

func writeValidRunnerSuiteTestPackage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeRunnerSuiteTestPackage(t, dir, runnerSuiteTestManifest(), runnerSuiteTestFixtures())
	return dir
}

func manifestComponents(t *testing.T, manifest map[string]any) []any {
	t.Helper()
	components, ok := manifest["components"].([]any)
	if !ok {
		t.Fatalf("manifest components have unexpected type %T", manifest["components"])
	}
	return components
}

func manifestComponent(t *testing.T, manifest map[string]any, index int) map[string]any {
	t.Helper()
	component, ok := manifestComponents(t, manifest)[index].(map[string]any)
	if !ok {
		t.Fatalf("component %d has unexpected type", index)
	}
	return component
}

func TestValidateIssueScanRunnerSuitePackageAcceptsValidPackage(t *testing.T) {
	dir := writeValidRunnerSuiteTestPackage(t)
	report, err := validateIssueScanRunnerSuitePackage(dir)
	if err != nil {
		t.Fatalf("expected valid package, got %v", err)
	}
	if report.SuiteID != "issue-scan-runner-suite" {
		t.Fatalf("unexpected suite id %q", report.SuiteID)
	}
	if report.ComponentCount != 5 {
		t.Fatalf("unexpected component count %d", report.ComponentCount)
	}
	if report.LifecycleVersion != issueScanRunnerContracts().LifecycleVersion {
		t.Fatalf("report lifecycle %q does not match contracts document", report.LifecycleVersion)
	}
}

// TestValidateIssueScanRunnerSuitePackageDoesNotExecuteCommands proves the R5
// no-execution boundary: command placeholders neither exist nor are executable,
// and if the harness ever executed one, the sentinel path would be created.
func TestValidateIssueScanRunnerSuitePackageDoesNotExecuteCommands(t *testing.T) {
	dir := writeValidRunnerSuiteTestPackage(t)
	sentinel := filepath.Join(dir, "executed.sentinel")
	script := "#!/bin/sh\ntouch " + sentinel + "\n"
	scriptPath := filepath.Join(dir, "runners", "stage_role_output_runner.placeholder")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir runners: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if _, err := validateIssueScanRunnerSuitePackage(dir); err != nil {
		t.Fatalf("expected valid package, got %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("validation executed a runner command (sentinel state: %v)", err)
	}
}

func TestValidateIssueScanRunnerSuitePackageFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(t *testing.T, dir string, manifest map[string]any) map[string]any
		wantErr string
	}{
		{
			name: "missing suite id",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				delete(m, "suite_id")
				return m
			},
			wantErr: "suite_id",
		},
		{
			name: "wrong manifest kind",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["kind"] = "not_a_runner_suite_manifest"
				return m
			},
			wantErr: "kind",
		},
		{
			name: "unknown manifest field",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["surprise_field"] = true
				return m
			},
			wantErr: "unknown field",
		},
		{
			name: "wrong lifecycle version",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["lifecycle_version"] = "civilization_issue_to_human_ready_pr_v0.1"
				return m
			},
			wantErr: "lifecycle_version",
		},
		{
			name: "missing validation command",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				delete(m, "validation_command")
				return m
			},
			wantErr: "validation_command",
		},
		{
			name: "unknown terminal stage path",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["terminal_stage_path"] = "yolo_terminal"
				return m
			},
			wantErr: "terminal_stage_path",
		},
		{
			name: "empty components",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["components"] = []any{}
				return m
			},
			wantErr: "components",
		},
		{
			name: "unknown component id",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["id"] = "mystery_runner"
				return m
			},
			wantErr: "unknown component",
		},
		{
			name: "duplicate component id",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 1)["id"] = "stage_role_output_runner"
				return m
			},
			wantErr: "duplicate component",
		},
		{
			name: "missing required component for managed posture",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				m["components"] = manifestComponents(t, m)[:4]
				return m
			},
			wantErr: "ready_state_review_runner",
		},
		{
			name: "excess component outside the declared terminal path",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				extra := map[string]any{
					"id":                   "ready_pr_evidence_runner",
					"command":              "runners/ready_pr_evidence_runner.placeholder",
					"argv":                 []any{},
					"timeout":              "10m",
					"stdin_kind":           "issue_scan_ready_pr_runner_context",
					"stdout_kind":          "hive.IssueScanReadyPRRunnerResult",
					"required_env":         []any{},
					"forbidden_env":        []any{"ANTHROPIC_API_KEY", "HIVE_ANTHROPIC_API_KEY"},
					"authority_boundaries": []any{"records externally supplied ready evidence only"},
					"fixtures": map[string]any{
						"stdin":  "examples/stage_role_output_runner/stdin.json",
						"stdout": "examples/stage_role_output_runner/stdout.json",
					},
				}
				m["components"] = append(manifestComponents(t, m), extra)
				return m
			},
			wantErr: "ready_pr_evidence_runner",
		},
		{
			name: "mismatched stdin kind",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["stdin_kind"] = "issue_scan_implementation_runner_context"
				return m
			},
			wantErr: "stdin",
		},
		{
			name: "mismatched stdout kind",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["stdout_kind"] = "hive.SomethingElse"
				return m
			},
			wantErr: "stdout",
		},
		{
			name: "unparseable timeout",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["timeout"] = "banana"
				return m
			},
			wantErr: "timeout",
		},
		{
			name: "non-positive timeout",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["timeout"] = "0s"
				return m
			},
			wantErr: "timeout",
		},
		{
			name: "required env var also forbidden",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["required_env"] = []any{"GITHUB_TOKEN"}
				return m
			},
			wantErr: "forbidden",
		},
		{
			name: "forbidden env missing canonical minimum",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["forbidden_env"] = []any{"GITHUB_TOKEN"}
				return m
			},
			wantErr: "ANTHROPIC_API_KEY",
		},
		{
			name: "empty authority boundaries",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["authority_boundaries"] = []any{}
				return m
			},
			wantErr: "authority_boundaries",
		},
		{
			name: "authority boundaries drop a contract boundary",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				manifestComponent(t, m, 0)["authority_boundaries"] = []any{"planning evidence only", "may approve, merge, and deploy"}
				return m
			},
			wantErr: "must include contract boundary",
		},
		{
			name: "authority boundaries add a non-contract entry",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				component := manifestComponent(t, m, 0)
				boundaries, ok := component["authority_boundaries"].([]any)
				if !ok {
					t.Fatalf("authority_boundaries has unexpected type")
				}
				component["authority_boundaries"] = append(boundaries, "may approve, merge, and deploy")
				return m
			},
			wantErr: "not a contract boundary",
		},
		{
			name: "missing argv",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				delete(manifestComponent(t, m, 0), "argv")
				return m
			},
			wantErr: "argv",
		},
		{
			name: "missing required_env",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				delete(manifestComponent(t, m, 0), "required_env")
				return m
			},
			wantErr: "required_env",
		},
		{
			name: "fixture symlink escapes package root",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				outside := filepath.Join(dir, "..", "outside-fixture.json")
				if err := os.WriteFile(outside, []byte(`{"role_outputs":[{"role":"r","summary":"s","outputs":[{"key":"k"}]}]}`), 0o644); err != nil {
					t.Fatalf("write outside fixture: %v", err)
				}
				inside := filepath.Join(dir, "examples", "stage_role_output_runner", "stdout.json")
				if err := os.Remove(inside); err != nil {
					t.Fatalf("remove fixture: %v", err)
				}
				if err := os.Symlink(outside, inside); err != nil {
					t.Fatalf("symlink fixture: %v", err)
				}
				return m
			},
			wantErr: "outside the package root",
		},
		{
			name: "missing fixture file",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				if err := os.Remove(filepath.Join(dir, "examples", "stage_role_output_runner", "stdin.json")); err != nil {
					t.Fatalf("remove fixture: %v", err)
				}
				return m
			},
			wantErr: "stdin.json",
		},
		{
			name: "malformed fixture json",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "stage_role_output_runner", "stdout.json")
				if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "fixture",
		},
		{
			name: "blocker repair stdout commit equals previous operate commit",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "blocker_repair_runner", "stdout.json")
				body := `{"operate_result_body":"branch: feat/synthetic-change\ncommit: 0000000000000000000000000000000000000001\n\n 1 file changed, 1 insertion(+)\n","completion_summary":"Synthetic blocker repair completion."}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "must differ",
		},
		{
			name: "blocker repair stdout commit equals previous operate commit ignoring case",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				stdinPath := filepath.Join(dir, "examples", "blocker_repair_runner", "stdin.json")
				stdinBody := `{"kind":"issue_scan_blocker_repair_runner_context","lifecycle_version":"civilization_issue_to_human_ready_pr_v0.9","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","repo_path":"workspace/synthetic-example","implementation_task_id":"task-synthetic-0002","implementation_stage_task_id":"task-synthetic-0003","review_stage_task_id":"task-synthetic-0005","blocker_stage_task_id":"task-synthetic-0006","request_changes_review_event_id":"event-synthetic-0001","request_changes_review_summary":"Synthetic blocker summary.","request_changes_review_issues":["synthetic blocker"],"request_changes_review_confidence":0.8,"reopen_event_id":"event-synthetic-0002","reopen_reason":"synthetic reopen","previous_operate_branch":"feat/synthetic-change","previous_operate_commit":"00000000000000000000000000000000000000ab","previous_changed_files_summary":"1 file changed (synthetic)"}`
				if err := os.WriteFile(stdinPath, []byte(stdinBody), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				stdoutPath := filepath.Join(dir, "examples", "blocker_repair_runner", "stdout.json")
				stdoutBody := `{"operate_result_body":"branch: feat/synthetic-change\ncommit: 00000000000000000000000000000000000000AB\n\n 1 file changed, 1 insertion(+)\n","completion_summary":"Synthetic blocker repair completion."}`
				if err := os.WriteFile(stdoutPath, []byte(stdoutBody), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "must differ",
		},
		{
			name: "adversarial review reviewed head does not match operate commit",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "adversarial_review_runner", "stdout.json")
				body := `{"repository":"transpara-ai/synthetic-example","review_ref":"synthetic-review-0001","reviewed_head_sha":"0000000000000000000000000000000000000999","verdict":"approve","summary":"Synthetic exact-head review.","issues":[],"confidence":0.9}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "must match",
		},
		{
			name: "ready state review reviewed head does not match operate commit",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "ready_state_review_runner", "stdout.json")
				body := `{"review_ref":"synthetic-review-0002","reviewed_head_sha":"0000000000000000000000000000000000000999","status":"pass"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "must match",
		},
		{
			name: "adversarial review verdict outside the runtime allowlist",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "adversarial_review_runner", "stdout.json")
				body := `{"repository":"transpara-ai/synthetic-example","review_ref":"synthetic-review-0001","reviewed_head_sha":"0000000000000000000000000000000000000001","verdict":"banana","summary":"Synthetic exact-head review.","issues":[],"confidence":0.9}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "verdict",
		},
		{
			name: "adversarial review confidence outside runtime bounds",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "adversarial_review_runner", "stdout.json")
				body := `{"repository":"transpara-ai/synthetic-example","review_ref":"synthetic-review-0001","reviewed_head_sha":"0000000000000000000000000000000000000001","verdict":"approve","summary":"Synthetic exact-head review.","issues":[],"confidence":0.3}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "confidence",
		},
		{
			name: "ready state review status not passing",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "ready_state_review_runner", "stdout.json")
				body := `{"review_ref":"synthetic-review-0002","reviewed_head_sha":"0000000000000000000000000000000000000001","status":"failed"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "not passing",
		},
		{
			name: "operate result body not parseable by the runtime parser",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "implementation_runner", "stdout.json")
				body := `{"operate_result_body":"{\"status\":\"completed\"}","completion_summary":"Synthetic implementation completion."}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "operate_result_body",
		},
		{
			name: "null stdout fixture",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "stage_role_output_runner", "stdout.json")
				if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "JSON null",
		},
		{
			name: "null stdin fixture",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "stage_role_output_runner", "stdin.json")
				if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "JSON null",
		},
		{
			name: "unknown field in fixture",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "implementation_runner", "stdout.json")
				body := `{"operate_result_body":"{}","completion_summary":"x","surprise":"y"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "surprise",
		},
		{
			name: "fixture stdin kind mismatch",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "stage_role_output_runner", "stdin.json")
				body := `{"kind":"wrong_kind","lifecycle_version":"civilization_issue_to_human_ready_pr_v0.9"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "kind",
		},
		{
			name: "fixture path escapes package directory",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				fixtures, ok := manifestComponent(t, m, 0)["fixtures"].(map[string]any)
				if !ok {
					t.Fatalf("fixtures field has unexpected type")
				}
				fixtures["stdin"] = "../outside.json"
				return m
			},
			wantErr: "local",
		},
		{
			name: "absolute fixture path",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				fixtures, ok := manifestComponent(t, m, 0)["fixtures"].(map[string]any)
				if !ok {
					t.Fatalf("fixtures field has unexpected type")
				}
				fixtures["stdin"] = "/etc/hostname"
				return m
			},
			wantErr: "local",
		},
		{
			name: "stdout fixture violates required field spec",
			corrupt: func(t *testing.T, dir string, m map[string]any) map[string]any {
				path := filepath.Join(dir, "examples", "stage_role_output_runner", "stdout.json")
				if err := os.WriteFile(path, []byte(`{"role_outputs":[]}`), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return m
			},
			wantErr: "role_outputs",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRunnerSuiteTestPackage(t, dir, runnerSuiteTestManifest(), runnerSuiteTestFixtures())
			manifest := tc.corrupt(t, dir, runnerSuiteTestManifest())
			body, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				t.Fatalf("marshal manifest: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(body, '\n'), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			_, err = validateIssueScanRunnerSuitePackage(dir)
			if err == nil {
				t.Fatalf("expected fail-closed error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestValidateIssueScanRunnerSuitePackageAcceptsCaseInsensitiveReviewHead
// proves the pair check mirrors the runtime's strings.EqualFold SHA
// comparison: a receipt citing the operate commit in different hex casing is
// runtime-valid and must validate.
func TestValidateIssueScanRunnerSuitePackageAcceptsCaseInsensitiveReviewHead(t *testing.T) {
	dir := writeValidRunnerSuiteTestPackage(t)
	stdinPath := filepath.Join(dir, "examples", "adversarial_review_runner", "stdin.json")
	stdinBody := `{"kind":"issue_scan_adversarial_review_context","lifecycle_version":"civilization_issue_to_human_ready_pr_v0.9","run_id":"run-synthetic-0001","factory_order_id":"fo-synthetic-0001","repository":"transpara-ai/synthetic-example","implementation_task_id":"task-synthetic-0002","review_stage_task_id":"task-synthetic-0005","operate_branch":"feat/synthetic-change","operate_commit":"00000000000000000000000000000000000000ab","changed_files_summary":"1 file changed (synthetic)"}`
	if err := os.WriteFile(stdinPath, []byte(stdinBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	stdoutPath := filepath.Join(dir, "examples", "adversarial_review_runner", "stdout.json")
	stdoutBody := `{"repository":"transpara-ai/synthetic-example","review_ref":"synthetic-review-0001","reviewed_head_sha":"00000000000000000000000000000000000000AB","verdict":"approve","summary":"Synthetic exact-head review.","issues":[],"confidence":0.9}`
	if err := os.WriteFile(stdoutPath, []byte(stdoutBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := validateIssueScanRunnerSuitePackage(dir); err != nil {
		t.Fatalf("expected case-insensitive reviewed head to validate, got %v", err)
	}
}

// TestValidateIssueScanRunnerSuitePackageRejectsTrailingManifestData proves a
// syntactically closed manifest followed by stray tokens (which
// json.Decoder.More does not flag for closing delimiters) is rejected.
func TestValidateIssueScanRunnerSuitePackageRejectsTrailingManifestData(t *testing.T) {
	dir := writeValidRunnerSuiteTestPackage(t)
	path := filepath.Join(dir, "manifest.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := os.WriteFile(path, append(body, '}'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err = validateIssueScanRunnerSuitePackage(dir)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("expected trailing-data error, got %v", err)
	}
}

func TestValidateIssueScanRunnerSuitePackageRejectsMissingManifest(t *testing.T) {
	_, err := validateIssueScanRunnerSuitePackage(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
}

// TestValidateIssueScanRunnerSuitePackageValidatesCommittedExample guards the
// committed inert example package against drift from the in-process contracts.
func TestValidateIssueScanRunnerSuitePackageValidatesCommittedExample(t *testing.T) {
	dir := filepath.Join("..", "..", "packages", "issue-scan-runner-suite")
	report, err := validateIssueScanRunnerSuitePackage(dir)
	if err != nil {
		t.Fatalf("committed example package invalid: %v", err)
	}
	if report.TerminalStagePath != "managed_ready_pr_finalizer" {
		t.Fatalf("unexpected terminal path %q", report.TerminalStagePath)
	}
}

func TestCheckIssueScanStdoutRequiredFieldSpecs(t *testing.T) {
	doc := func(body string) map[string]any {
		var out map[string]any
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("bad test doc: %v", err)
		}
		return out
	}
	cases := []struct {
		name    string
		doc     string
		spec    string
		wantErr string
	}{
		{name: "present scalar", doc: `{"a":"x"}`, spec: "a"},
		{name: "missing scalar", doc: `{}`, spec: "a", wantErr: "a"},
		{name: "empty string scalar", doc: `{"a":""}`, spec: "a", wantErr: "a"},
		{name: "null scalar", doc: `{"a":null}`, spec: "a", wantErr: "a"},
		{name: "present number", doc: `{"confidence":0.9}`, spec: "confidence"},
		{name: "non-empty array", doc: `{"a":[{"b":"x"}]}`, spec: "a[]"},
		{name: "empty array", doc: `{"a":[]}`, spec: "a[]", wantErr: "a"},
		{name: "array element field", doc: `{"a":[{"b":"x"}]}`, spec: "a[].b"},
		{name: "array element field missing", doc: `{"a":[{}]}`, spec: "a[].b", wantErr: "b"},
		{name: "array element field empty", doc: `{"a":[{"b":""}]}`, spec: "a[].b", wantErr: "b"},
		{name: "nested array", doc: `{"a":[{"b":["x"]}]}`, spec: "a[].b[]"},
		{name: "nested equality", doc: `{"a":{"kind":"v"}}`, spec: "a.kind=v"},
		{name: "nested equality mismatch", doc: `{"a":{"kind":"w"}}`, spec: "a.kind=v", wantErr: "kind"},
		{name: "unknown spec syntax", doc: `{"a":"x"}`, spec: "a b !", wantErr: "spec"},
		{name: "empty spec", doc: `{"a":"x"}`, spec: "", wantErr: "spec"},
		{name: "equality on non-final segment", doc: `{"a":{"b":"c"}}`, spec: "a=x.b", wantErr: "spec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkIssueScanStdoutRequiredField(doc(tc.doc), tc.spec)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected spec %q to pass, got %v", tc.spec, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q for spec %q, got %v", tc.wantErr, tc.spec, err)
			}
		})
	}
}

// TestIssueScanRunnerContractSpecsAreParseable proves every
// stdout_required_fields spec in the in-process contracts document is inside
// the checker grammar, so an unrecognised spec can never be silently skipped.
func TestIssueScanRunnerContractSpecsAreParseable(t *testing.T) {
	document := issueScanRunnerContracts()
	contracts := append([]issueScanRunnerContract{}, document.ExternalRunnerContracts...)
	contracts = append(contracts, document.ManagedBoundaryContracts...)
	if document.InternalFinalizerContract != nil {
		contracts = append(contracts, *document.InternalFinalizerContract)
	}
	for _, contract := range contracts {
		for _, spec := range contract.StdoutRequiredFields {
			if _, err := parseIssueScanStdoutRequiredFieldSpec(spec); err != nil {
				t.Fatalf("contract %s spec %q is outside the checker grammar: %v", contract.ID, spec, err)
			}
		}
	}
}

func TestFactoryValidateRunnerSuiteCommandRegistered(t *testing.T) {
	dir := writeValidRunnerSuiteTestPackage(t)
	if err := routeAndDispatch([]string{"factory", "validate-issue-scan-runner-suite", "--package", dir}); err != nil {
		t.Fatalf("expected valid package via CLI, got %v", err)
	}
}

func TestFactoryValidateRunnerSuiteCommandRequiresPackage(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "validate-issue-scan-runner-suite"})
	if err == nil || !strings.Contains(err.Error(), "--package") {
		t.Fatalf("expected missing --package error, got %v", err)
	}
}

func TestFactoryValidateRunnerSuiteCommandFailsClosedOnInvalidPackage(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "validate-issue-scan-runner-suite", "--package", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("expected invalid package error, got %v", err)
	}
}
