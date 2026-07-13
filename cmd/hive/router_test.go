package main

import (
	"flag"
	"strings"
	"testing"
)

func TestRouteAndDispatchNoArgs(t *testing.T) {
	err := routeAndDispatch(nil)
	if err == nil {
		t.Fatal("expected error when no verb given")
	}
	msg := err.Error()
	for _, want := range []string{"civilization", "pipeline", "role", "ingest", "council"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q, got: %s", want, msg)
		}
	}
	for _, gone := range []string{"--pipeline", "--role", "--human", "--loop", "--one-shot"} {
		if strings.Contains(msg, gone) {
			t.Errorf("error should NOT mention removed flag %q, got: %s", gone, msg)
		}
	}
}

func TestRouteAndDispatchUnknownVerb(t *testing.T) {
	err := routeAndDispatch([]string{"banana"})
	if err == nil {
		t.Fatal("expected error for unknown verb")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should name the unknown verb, got: %s", err.Error())
	}
}

func TestCmdCivilizationRequiresSubverb(t *testing.T) {
	err := cmdCivilization(nil)
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdCivilizationRunRequiresHuman(t *testing.T) {
	err := cmdCivilization([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "--human") {
		t.Fatalf("expected --human required error, got: %v", err)
	}
}

func TestCmdCivilizationDaemonAcceptsIssueScanFlagsBeforeHumanValidation(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive"})
	if err == nil || !strings.Contains(err.Error(), "--human") {
		t.Fatalf("expected missing --human error after issue-scan flag parse, got: %v", err)
	}
	if strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("civilization daemon did not accept issue-scan flags: %v", err)
	}
}

func TestCmdCivilizationDaemonIssueScanIntervalRejectsAutoApproveRequests(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive", "--approve-requests"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected issue-scan auto-approval guard error, got: %v", err)
	}
}

func TestCmdCivilizationDaemonRequiresIssueScanReviewRunnerBeforeRunnerArg(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--issue-scan-review-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-review-runner") {
		t.Fatalf("expected missing review runner error, got: %v", err)
	}
}

func TestCmdCivilizationDaemonReadyPRMarkReadyRequiresReviewRunner(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-review-runner") {
		t.Fatalf("expected missing ready PR review runner error, got: %v", err)
	}
}

func TestCmdCivilizationDaemonRequireFullChainRejectsBeforeSeedSpec(t *testing.T) {
	err := cmdCivilization([]string{
		"daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--seed-spec", "/nonexistent/spec.md",
	})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-require-full-chain") || !strings.Contains(err.Error(), "--issue-scan-stage-role-runner") {
		t.Fatalf("expected full-chain preflight error, got: %v", err)
	}
	if strings.Contains(err.Error(), "ingest seed-spec") || strings.Contains(err.Error(), "read spec") {
		t.Fatalf("full-chain preflight must fire before seed-spec ingest, got: %v", err)
	}
}

func TestCmdCivilizationDaemonSeedSpecStillIngestsBeforeLoop(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--seed-spec", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "ingest seed-spec") || !strings.Contains(err.Error(), "read spec") {
		t.Fatalf("expected seed-spec ingest error before daemon loop start, got: %v", err)
	}
}

func TestCmdCivilizationDaemonApproveRequestsWithoutIssueScanReachesSeedSpec(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--approve-requests", "--seed-spec", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "ingest seed-spec") || !strings.Contains(err.Error(), "read spec") {
		t.Fatalf("expected legacy approve-requests path to reach seed-spec ingest, got: %v", err)
	}
	if strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("approve-requests without issue scanning should not be rejected, got: %v", err)
	}
}

func TestCmdCivilizationDaemonIssueScanAutoApproveRejectsBeforeSeedSpecIngest(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive", "--approve-requests", "--seed-spec", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected issue-scan auto-approval guard error, got: %v", err)
	}
	if strings.Contains(err.Error(), "ingest seed-spec") || strings.Contains(err.Error(), "read spec") {
		t.Fatalf("issue-scan auto-approval guard must fire before seed-spec ingest, got: %v", err)
	}
}

func TestCmdCivilizationDaemonPreservesLegacySpaceAPIAndApproveRolesFlags(t *testing.T) {
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--space", "ops", "--api", "http://127.0.0.1:1", "--approve-roles", "--seed-spec", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "ingest seed-spec") || !strings.Contains(err.Error(), "read spec") {
		t.Fatalf("expected legacy flags to parse before seed-spec ingest error, got: %v", err)
	}
	if strings.Contains(err.Error(), "flag provided but not defined") || strings.Contains(err.Error(), "--approve-roles") {
		t.Fatalf("legacy civilization daemon flags should remain accepted, got: %v", err)
	}
}

func TestResolveWebhookBearerTokenWholeDomain(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		requireAuth bool
		apiKey      string
		wantToken   string
		wantErr     string
	}{
		{name: "default loopback ignores ambient key", apiKey: "ambient-production-key"},
		{name: "explicit IPv4 loopback ignores ambient key", addr: "127.0.0.1:8081", apiKey: "ambient-production-key"},
		{name: "explicit IPv6 loopback ignores ambient key", addr: "[::1]:8081", apiKey: "ambient-production-key"},
		{name: "localhost ignores ambient key", addr: "localhost:8081", apiKey: "ambient-production-key"},
		{name: "public wildcard requires auth", addr: ":8081", wantErr: "--webhook-require-auth"},
		{name: "public IPv4 requires auth", addr: "0.0.0.0:8081", wantErr: "--webhook-require-auth"},
		{name: "public IPv6 requires auth", addr: "[::]:8081", wantErr: "--webhook-require-auth"},
		{name: "auth requires key", requireAuth: true, wantErr: "TRANSPARA_API_KEY"},
		{name: "public auth requires key", addr: ":8081", requireAuth: true, wantErr: "TRANSPARA_API_KEY"},
		{name: "loopback explicit auth", requireAuth: true, apiKey: "explicit-key", wantToken: "explicit-key"},
		{name: "public explicit auth", addr: ":8081", requireAuth: true, apiKey: "explicit-key", wantToken: "explicit-key"},
		{name: "auth rejects whitespace-only key", requireAuth: true, apiKey: "   \t ", wantErr: "TRANSPARA_API_KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveWebhookBearerToken(tt.addr, tt.requireAuth, tt.apiKey)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.wantToken {
				t.Fatalf("token/error = %q/%v, want %q/nil", got, err, tt.wantToken)
			}
		})
	}
}

func TestCmdCivilizationDaemonWebhookAuthFlagRequiresKeyBeforeRuntime(t *testing.T) {
	t.Setenv("TRANSPARA_API_KEY", "")
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--webhook-require-auth"})
	if err == nil || !strings.Contains(err.Error(), "TRANSPARA_API_KEY") {
		t.Fatalf("error = %v, want missing TRANSPARA_API_KEY refusal", err)
	}
	if strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("--webhook-require-auth was not registered: %v", err)
	}
}

func TestCmdCivilizationDaemonDraftPRCreateRequiresGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	err := cmdCivilization([]string{"daemon", "--human", "Michael", "--issue-scan-draft-pr-create"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected missing GITHUB_TOKEN error, got: %v", err)
	}
}

func TestCmdCivilizationUnknownSubverb(t *testing.T) {
	err := cmdCivilization([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestCmdPipelineRequiresSubverb(t *testing.T) {
	err := cmdPipeline(nil)
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdPipelineUnknownSubverb(t *testing.T) {
	err := cmdPipeline([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestPipelineFlagsDefaultToProposalMode(t *testing.T) {
	fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	_, _, _, _, _, _, _, direct, prMode, _, _ := pipelineFlags(fs)
	if err := fs.Parse([]string{"--pr=false"}); err != nil {
		t.Fatal(err)
	}
	if *direct {
		t.Fatal("pipeline --pr=false must not opt out of proposal mode")
	}
	if *prMode {
		t.Fatal("--pr=false should only set the compatibility flag false")
	}

	fs = flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	_, _, _, _, _, _, _, direct, _, _, _ = pipelineFlags(fs)
	if err := fs.Parse([]string{"--direct"}); err != nil {
		t.Fatal(err)
	}
	if !*direct {
		t.Fatal("--direct should opt into legacy direct mode")
	}
}

func TestContentSendingAPIDefaultsRequireProductionOptIn(t *testing.T) {
	type flagHarness struct {
		flags *flag.FlagSet
		api   func() string
	}
	commands := []struct {
		name string
		new  func() flagHarness
	}{
		{
			name: "pipeline",
			new: func() flagHarness {
				fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
				_, api, _, _, _, _, _, _, _, _, _ := pipelineFlags(fs)
				return flagHarness{flags: fs, api: func() string { return *api }}
			},
		},
		{
			name: "role",
			new: func() flagHarness {
				fs := flag.NewFlagSet("role builder run", flag.ContinueOnError)
				_, api, _, _, _, _, _ := roleFlags(fs)
				return flagHarness{flags: fs, api: func() string { return *api }}
			},
		},
		{
			name: "council",
			new: func() flagHarness {
				fs := newCouncilFlagSet()
				return flagHarness{flags: fs, api: func() string { return fs.Lookup("api").Value.String() }}
			},
		},
	}

	for _, command := range commands {
		t.Run(command.name+" default", func(t *testing.T) {
			harness := command.new()
			if err := harness.flags.Parse(nil); err != nil {
				t.Fatal(err)
			}
			if got := harness.api(); got != "http://localhost:8082" {
				t.Fatalf("bare %s --api = %q, want local-only default", command.name, got)
			}
		})
		t.Run(command.name+" explicit production", func(t *testing.T) {
			harness := command.new()
			if err := harness.flags.Parse([]string{"--api", "https://transpara.ai"}); err != nil {
				t.Fatal(err)
			}
			if got := harness.api(); got != "https://transpara.ai" {
				t.Fatalf("explicit %s production opt-in = %q", command.name, got)
			}
		})
	}
}

func TestCmdRoleRequiresName(t *testing.T) {
	err := cmdRole(nil)
	if err == nil || !strings.Contains(err.Error(), "role name") {
		t.Fatalf("expected role-name-required error, got: %v", err)
	}
}

func TestCmdRoleRequiresSubverb(t *testing.T) {
	err := cmdRole([]string{"builder"})
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdRoleUnknownSubverb(t *testing.T) {
	err := cmdRole([]string{"builder", "frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestRoleFlagsDefaultBuilderToProposalMode(t *testing.T) {
	fs := flag.NewFlagSet("role builder run", flag.ContinueOnError)
	_, _, _, _, _, direct, prMode := roleFlags(fs)
	if err := fs.Parse([]string{"--pr=false"}); err != nil {
		t.Fatal(err)
	}
	if *direct {
		t.Fatal("role --pr=false must not opt out of proposal mode")
	}
	if *prMode {
		t.Fatal("--pr=false should only set the compatibility flag false")
	}

	fs = flag.NewFlagSet("role builder run", flag.ContinueOnError)
	_, _, _, _, _, direct, _ = roleFlags(fs)
	if err := fs.Parse([]string{"--direct"}); err != nil {
		t.Fatal(err)
	}
	if !*direct {
		t.Fatal("--direct should opt into legacy direct mode")
	}
}

func TestCmdIngestRequiresFile(t *testing.T) {
	err := cmdIngest(nil)
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected file-required error, got: %v", err)
	}
}

func TestCmdCouncilRejectsUnknownFlag(t *testing.T) {
	err := cmdCouncil([]string{"--banana", "x"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Fatalf("error should mention unknown flag name, got: %v", err)
	}
}

func TestCmdCouncil_AcceptsCatalogFlag(t *testing.T) {
	fs := newCouncilFlagSet()
	args := []string{"--catalog", "/tmp/test-catalog.yaml", "--topic", "x"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := fs.Lookup("catalog").Value.String(); got != "/tmp/test-catalog.yaml" {
		t.Errorf("--catalog value=%q; want /tmp/test-catalog.yaml", got)
	}
}

func TestDaemonCommandsAcceptCatalogReloadIntervalFlag(t *testing.T) {
	for _, args := range [][]string{
		{"civilization", "daemon", "--catalog-reload-interval", "5s"},
		{"factory", "daemon", "--catalog-reload-interval", "5s"},
	} {
		err := routeAndDispatch(args)
		if err == nil || !strings.Contains(err.Error(), "human") {
			t.Fatalf("%v: expected missing-human validation after flag parse, got %v", args, err)
		}
		if strings.Contains(err.Error(), "flag provided but not defined") {
			t.Fatalf("%v: catalog reload flag was not accepted: %v", args, err)
		}
	}
}
