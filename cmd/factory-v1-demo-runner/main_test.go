package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

func TestDecodeRunRequestStrict(t *testing.T) {
	request := testRunRequest(t, t.TempDir(), factoryv1.StageIngestWork)
	raw, _ := json.Marshal(request)
	if _, err := decodeRunRequest(bytes.NewReader(raw)); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	withUnknown := strings.TrimSuffix(string(raw), "}") + `,"unknown":true}`
	if _, err := decodeRunRequest(strings.NewReader(withUnknown)); err == nil {
		t.Fatal("unknown RunRequest field was accepted")
	}
	if _, err := decodeRunRequest(strings.NewReader(string(raw) + `{}`)); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

func TestCrossFamilyGateConsumesExactIndependentArtifact(t *testing.T) {
	state := privateTempDir(t)
	repositoryRoot := t.TempDir()
	request := testRunRequest(t, repositoryRoot, factoryv1.StageCFADA)
	cfg := testConfig(t, repositoryRoot)
	cfg.AuthorFamily = "Codex/OpenAI"
	runner, err := newDemoRunner(state, cfg, execCommander{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.executeDesign(request); err != nil {
		t.Fatal(err)
	}
	design, err := runner.requireDesign(request)
	if err != nil {
		t.Fatal(err)
	}
	path := runner.reviewerArtifactPath(request, "cfada")
	artifact := reviewerArtifact{
		SchemaVersion: runnerSchemaVersion, Gate: "cfada", OrderID: request.Order.DocID,
		DocumentSHA256: request.DocumentSHA256, DesignBlobSHA: design, BlockerCount: 0,
		AuthorFamily: cfg.AuthorFamily, ReviewerFamily: cfg.AuthorFamily, Provider: request.Provider,
		Reference: "review:cfada:test",
	}
	if err := writePrivateJSON(path, artifact); err != nil {
		t.Fatal(err)
	}
	result, _, err := runner.executeCrossFamilyGate(request, "cfada")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != factoryv1.RunnerBlocked {
		t.Fatalf("same-family artifact status = %s, want blocked", result.Status)
	}
	artifact.ReviewerFamily = request.Provider.Family
	if err := writePrivateJSON(path, artifact); err != nil {
		t.Fatal(err)
	}
	result, _, err = runner.executeCrossFamilyGate(request, "cfada")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != factoryv1.RunnerPassed || len(result.Evidence) != 1 || result.Evidence[0].BlockerCount == nil || *result.Evidence[0].BlockerCount != 0 || result.Evidence[0].DesignBlobSHA != design {
		t.Fatalf("exact independent artifact did not pass: %+v", result)
	}
}

func TestWriteCodeUsesBoundedBranchAndPrivateIdempotentReceipt(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	repositoryRoot := filepath.Join(base, "repository")
	runGitTest(t, "", gitPath, "init", "--bare", remote)
	runGitTest(t, "", gitPath, "init", "-b", "main", seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, seed, gitPath, "add", "README.md")
	runGitTest(t, seed, gitPath, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	runGitTest(t, seed, gitPath, "remote", "add", "origin", remote)
	runGitTest(t, seed, gitPath, "push", "origin", "main")
	runGitTest(t, "", gitPath, "clone", "--branch", "main", remote, repositoryRoot)
	mainBefore := strings.TrimSpace(runGitTest(t, "", gitPath, "--git-dir", remote, "rev-parse", "refs/heads/main"))

	state := privateTempDir(t)
	cfg := testConfig(t, repositoryRoot)
	cfg.GitExecutable = gitPath
	ghFake := filepath.Join(base, "gh-fake")
	if err := os.WriteFile(ghFake, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg.GHExecutable = ghFake
	commands := &pinningCommander{delegate: execCommander{}, git: gitPath, gh: ghFake, identity: "transpara-ai/demo"}
	runner, err := newDemoRunner(state, cfg, commands)
	if err != nil {
		t.Fatal(err)
	}
	request := testRunRequest(t, repositoryRoot, factoryv1.StageWriteCode)
	request.Order.TargetRepository = "transpara-ai/demo"
	request.Order.Authority.TargetRepositories = []string{"transpara-ai/demo"}
	document, err := factoryv1.Canonicalize(request.Order)
	if err != nil {
		t.Fatal(err)
	}
	request.OrderMarkdown, request.DocumentSHA256 = document.Markdown, document.SHA256
	request.AttemptID, _ = factoryv1.AttemptID(document.SHA256, request.Stage, request.Ordinal)
	if err := runner.validateRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	result, err := runner.execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != factoryv1.RunnerPassed {
		t.Fatalf("write_code result = %+v", result)
	}
	branch := branchName(request)
	branchHead := strings.TrimSpace(runGitTest(t, "", gitPath, "--git-dir", remote, "rev-parse", "refs/heads/"+branch))
	mainAfter := strings.TrimSpace(runGitTest(t, "", gitPath, "--git-dir", remote, "rev-parse", "refs/heads/main"))
	if mainAfter != mainBefore {
		t.Fatalf("default branch changed from %s to %s", mainBefore, mainAfter)
	}
	if branchHead != result.Evidence[0].PRHeadSHA {
		t.Fatalf("remote branch head = %s, evidence = %s", branchHead, result.Evidence[0].PRHeadSHA)
	}
	file := runGitTest(t, "", gitPath, "--git-dir", remote, "show", branch+":"+evidenceRelativePath(request))
	if file != renderDemoEvidence(request) {
		t.Fatalf("remote bounded evidence differs:\n%s", file)
	}
	receiptInfo, err := os.Stat(runner.receiptPath(request.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	if receiptInfo.Mode().Perm() != 0o600 {
		t.Fatalf("receipt mode = %o, want 600", receiptInfo.Mode().Perm())
	}
	pushes := commands.pushes
	again, err := runner.execute(ctx, request)
	if err != nil || again.Status != factoryv1.RunnerPassed {
		t.Fatalf("idempotent retry = (%+v, %v)", again, err)
	}
	if commands.pushes != pushes {
		t.Fatalf("idempotent retry pushed again: before=%d after=%d", pushes, commands.pushes)
	}
	reconcileRequest := request
	reconcileRequest.Operation = "reconcile"
	reconciled, err := runner.reconcile(ctx, reconcileRequest)
	if err != nil || !reconciled.EffectExists || reconciled.Conflict {
		t.Fatalf("reconcile = (%+v, %v)", reconciled, err)
	}
}

func TestPRFlowStopsAtExactHeadHumanReview(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	repositoryRoot := filepath.Join(base, "repository")
	runGitTest(t, "", gitPath, "init", "--bare", remote)
	runGitTest(t, "", gitPath, "init", "-b", "main", seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, seed, gitPath, "add", "README.md")
	runGitTest(t, seed, gitPath, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	runGitTest(t, seed, gitPath, "remote", "add", "origin", remote)
	runGitTest(t, seed, gitPath, "push", "origin", "main")
	runGitTest(t, "", gitPath, "clone", "--branch", "main", remote, repositoryRoot)

	state := privateTempDir(t)
	cfg := testConfig(t, repositoryRoot)
	cfg.GitExecutable = gitPath
	ghFake := filepath.Join(base, "gh-fake")
	if err := os.WriteFile(ghFake, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg.GHExecutable = ghFake
	commands := &githubFlowCommander{delegate: execCommander{}, git: gitPath, gh: ghFake, identity: "transpara-ai/demo"}
	runner, err := newDemoRunner(state, cfg, commands)
	if err != nil {
		t.Fatal(err)
	}

	writeRequest := testRunRequest(t, repositoryRoot, factoryv1.StageWriteCode)
	writeResult, err := runner.execute(ctx, writeRequest)
	if err != nil || writeResult.Status != factoryv1.RunnerPassed {
		t.Fatalf("write result=(%+v,%v)", writeResult, err)
	}
	head := writeResult.Evidence[0].PRHeadSHA
	commands.head = head
	commands.branch = branchName(writeRequest)

	draftRequest := stageRequest(t, writeRequest, factoryv1.StageCreateDraftPR, writeResult.Evidence)
	draftResult, err := runner.execute(ctx, draftRequest)
	if err != nil || draftResult.Status != factoryv1.RunnerPassed || draftResult.Evidence[0].PR == nil || !draftResult.Evidence[0].PR.Draft {
		t.Fatalf("draft result=(%+v,%v)", draftResult, err)
	}

	iarPrior := append(append([]factoryv1.Evidence{}, writeResult.Evidence...), draftResult.Evidence...)
	iarRequest := stageRequest(t, writeRequest, factoryv1.StageIAR, iarPrior)
	iarResult, err := runner.execute(ctx, iarRequest)
	if err != nil || iarResult.Status != factoryv1.RunnerPassed || iarResult.Evidence[0].PRHeadSHA != head || iarResult.Evidence[0].ReviewedHeadSHA != head {
		t.Fatalf("IAR result=(%+v,%v)", iarResult, err)
	}
	iarReconcile := iarRequest
	iarReconcile.Operation = "reconcile"
	reconciledIAR, err := runner.reconcile(ctx, iarReconcile)
	if err != nil || !reconciledIAR.EffectExists || reconciledIAR.Conflict {
		t.Fatalf("IAR reconcile=(%+v,%v)", reconciledIAR, err)
	}

	cfarPrior := append(iarPrior, iarResult.Evidence...)
	cfarRequest := stageRequest(t, writeRequest, factoryv1.StageCFAR, cfarPrior)
	artifact := reviewerArtifact{
		SchemaVersion: runnerSchemaVersion, Gate: "cfar", OrderID: cfarRequest.Order.DocID,
		DocumentSHA256: cfarRequest.DocumentSHA256, PRHeadSHA: head, BlockerCount: 0,
		AuthorFamily: cfg.AuthorFamily, ReviewerFamily: cfarRequest.Provider.Family, Provider: cfarRequest.Provider,
		Reference: "review:cfar:test",
	}
	if err := writePrivateJSON(runner.reviewerArtifactPath(cfarRequest, "cfar"), artifact); err != nil {
		t.Fatal(err)
	}
	cfarResult, err := runner.execute(ctx, cfarRequest)
	if err != nil || cfarResult.Status != factoryv1.RunnerPassed {
		t.Fatalf("CFAR result=(%+v,%v)", cfarResult, err)
	}

	readyPrior := append(cfarPrior, cfarResult.Evidence...)
	readyRequest := stageRequest(t, writeRequest, factoryv1.StageMarkPRReady, readyPrior)
	readyResult, err := runner.execute(ctx, readyRequest)
	if err != nil || readyResult.Status != factoryv1.RunnerPassed || readyResult.Evidence[0].PR == nil {
		t.Fatalf("ready result=(%+v,%v)", readyResult, err)
	}
	if err := factoryv1.ValidateReadyPR(*readyResult.Evidence[0].PR); err != nil {
		t.Fatalf("runner emitted invalid ready PR: %v", err)
	}

	humanPrior := append(readyPrior, readyResult.Evidence...)
	humanRequest := stageRequest(t, writeRequest, factoryv1.StageHumanReview, humanPrior)
	humanResult, err := runner.execute(ctx, humanRequest)
	if err != nil || humanResult.Status != factoryv1.RunnerHumanRequired || !strings.Contains(humanResult.NextAction, "will not merge") {
		t.Fatalf("Human Review result=(%+v,%v)", humanResult, err)
	}
	if commands.mergeCalls != 0 || commands.readyCalls != 1 || commands.pr.IsDraft {
		t.Fatalf("GitHub calls: ready=%d merge=%d pr=%+v", commands.readyCalls, commands.mergeCalls, commands.pr)
	}
}

type pinningCommander struct {
	delegate commander
	git      string
	gh       string
	identity string
	pushes   int
}

type githubFlowCommander struct {
	delegate   commander
	git        string
	gh         string
	identity   string
	head       string
	branch     string
	pr         pullRequestView
	readyCalls int
	mergeCalls int
}

type requiredPolicyCommander struct {
	policy string
	rollup string
	calls  int
}

func (c *requiredPolicyCommander) Run(_ context.Context, _ string, _ string, args ...string) (commandResult, error) {
	c.calls++
	if len(args) >= 2 && args[0] == "api" {
		return commandResult{Stdout: c.policy}, nil
	}
	if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
		return commandResult{Stdout: c.rollup}, nil
	}
	return commandResult{}, errors.New("unexpected check query")
}

func (c *githubFlowCommander) Run(ctx context.Context, dir, executable string, args ...string) (commandResult, error) {
	if executable == c.git && len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" {
		return commandResult{Stdout: "https://github.com/" + c.identity + ".git\n"}, nil
	}
	if executable != c.gh {
		return c.delegate.Run(ctx, dir, executable, args...)
	}
	if len(args) >= 2 && args[0] == "repo" && args[1] == "view" {
		return commandResult{Stdout: c.identity + "\n"}, nil
	}
	if len(args) >= 2 && args[0] == "api" {
		return commandResult{Stdout: `{"contexts":["verify"]}`}, nil
	}
	if len(args) >= 2 && args[0] == "pr" {
		switch args[1] {
		case "list":
			views := []pullRequestView{}
			if c.pr.Number != 0 {
				views = append(views, c.pr)
			}
			raw, _ := json.Marshal(views)
			return commandResult{Stdout: string(raw)}, nil
		case "create":
			c.pr = pullRequestView{Number: 101, URL: "https://github.com/" + c.identity + "/pull/101", HeadRefOID: c.head, HeadRefName: c.branch, BaseRefName: "main", IsDraft: true, State: "OPEN"}
			return commandResult{Stdout: c.pr.URL + "\n"}, nil
		case "checks":
			return commandResult{Stdout: `[{"name":"verify","state":"SUCCESS","bucket":"pass","link":"https://checks.invalid/verify"}]`}, nil
		case "view":
			return commandResult{Stdout: `{"statusCheckRollup":[{"name":"verify","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://checks.invalid/verify"}]}`}, nil
		case "ready":
			c.readyCalls++
			c.pr.IsDraft = false
			return commandResult{Stdout: "ready\n"}, nil
		case "merge":
			c.mergeCalls++
			return commandResult{}, errors.New("merge is forbidden")
		}
	}
	return commandResult{}, errors.New("unexpected gh command: " + strings.Join(args, " "))
}

func (c *pinningCommander) Run(ctx context.Context, dir, executable string, args ...string) (commandResult, error) {
	if executable == c.gh && len(args) >= 3 && args[0] == "repo" && args[1] == "view" {
		return commandResult{Stdout: c.identity + "\n"}, nil
	}
	if executable == c.git && len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" {
		return commandResult{Stdout: "https://github.com/" + c.identity + ".git\n"}, nil
	}
	if executable == c.git && len(args) >= 2 && args[0] == "push" {
		c.pushes++
	}
	return c.delegate.Run(ctx, dir, executable, args...)
}

func testRunRequest(t *testing.T, repositoryRoot string, stage factoryv1.Stage) factoryv1.RunRequest {
	t.Helper()
	order := factoryv1.FactoryOrder{
		DocID: "FO-DEMO-TEST", Version: "1.0.0", Status: "approved", Title: "Bounded demo evidence",
		Channel: factoryv1.ChannelCompletedOrder, TargetRepository: "transpara-ai/demo",
		SourceReferences:   []factoryv1.SourceReference{{Kind: "test", Identity: "source:test", URI: "test://source", SHA256: strings.Repeat("a", 64)}},
		Requirements:       []factoryv1.Requirement{{ID: "R1", Statement: "Add one bounded evidence file.", Rationale: "Demonstrate the factory."}},
		AcceptanceCriteria: []factoryv1.AcceptanceCriterion{{ID: "AC1", Statement: "Only the evidence file changes.", VerificationMethod: "git diff and named test", RiskClass: "low"}},
		TestPlan:           []string{"git diff --check"}, Constraints: []string{"non-production"}, NonGoals: []string{"merge"}, ExpectedOutputs: []string{"ready PR"},
		Authority: factoryv1.AuthorityScope{ActorID: "human-actor", AllowedActions: []string{"repo.branch.create", "repo.commit.create", "repo.pull_request.create", "repo.pull_request.mark_ready", "governance.review.record"}, TargetRepositories: []string{"transpara-ai/demo"}, NonProductionOnly: true},
		Budget:    factoryv1.BudgetLimit{MaxAttempts: 30, MaxTokens: 1000, MaxCostMicros: 1000},
	}
	document, err := factoryv1.Canonicalize(order)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := factoryv1.AttemptID(document.SHA256, stage, 1)
	if err != nil {
		t.Fatal(err)
	}
	return factoryv1.RunRequest{
		Operation: "execute", Order: order, OrderMarkdown: document.Markdown, DocumentSHA256: document.SHA256,
		Stage: stage, AttemptID: attempt, Ordinal: 1, RepositoryRoot: repositoryRoot,
		AuthorityScope: order.Authority, BudgetRemaining: factoryv1.BudgetProjection{RemainingAttempts: 30, RemainingTokens: 1000, RemainingCostMicros: 1000},
		Peers: factoryv1.PeersForStage(stage), Provider: factoryv1.ProviderBinding{ProviderID: "reviewer", Family: "Claude/Anthropic", ExecutableRealpath: "/private/reviewer", ExecutableSHA256: strings.Repeat("b", 64), ModelID: "reviewer-model", CredentialSourceID: "credential:test"},
	}
}

func stageRequest(t *testing.T, base factoryv1.RunRequest, stage factoryv1.Stage, prior []factoryv1.Evidence) factoryv1.RunRequest {
	t.Helper()
	base.Operation = "execute"
	base.Stage = stage
	base.Ordinal = 1
	base.Peers = factoryv1.PeersForStage(stage)
	base.PriorEvidence = append([]factoryv1.Evidence(nil), prior...)
	base.AttemptID, _ = factoryv1.AttemptID(base.DocumentSHA256, stage, 1)
	return base
}

func testConfig(t *testing.T, repositoryRoot string) config {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		ghPath = gitPath
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	reviewerRoot := filepath.Join(privateTempDir(t), "reviewer-artifacts")
	if err := ensurePrivateDir(reviewerRoot); err != nil {
		t.Fatal(err)
	}
	return config{
		SchemaVersion: runnerSchemaVersion, GitExecutable: gitPath, GHExecutable: ghPath, GitRemote: "origin",
		AuthorFamily: "Codex/OpenAI", CommitUserName: "Factory v1 Demo", CommitUserEmail: "factory-v1@example.invalid",
		Repositories:      map[string]repositoryConfig{"transpara-ai/demo": {Root: root, Identity: "transpara-ai/demo", RemoteURL: "https://github.com/transpara-ai/demo.git", BaseBranch: "main", TestCommands: [][]string{{"git", "diff", "--check"}}}},
		StandingApprovals: map[string]factoryv1.HumanApprovalReceipt{}, ReviewerEvidenceDir: reviewerRoot,
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDesignContractPinsDeterministicEffectAuthorityValidationAndGates(t *testing.T) {
	repositoryRoot := t.TempDir()
	request := testRunRequest(t, repositoryRoot, factoryv1.StageDesign)
	request.AttemptID = "volatile-attempt-marker"
	request.PriorEvidence = []factoryv1.Evidence{{Reference: "volatile-event-marker", Metadata: map[string]string{"work_artifact_id": "volatile-work-marker"}}}
	runner := &demoRunner{config: testConfig(t, repositoryRoot)}
	design := runner.renderDesign(request)

	for _, want := range []string{
		"- Intake channel: `completed_factory_order`",
		"- Deterministic branch: `" + branchName(request) + "`",
		"- Bounded output: `" + evidenceRelativePath(request) + "`",
		"- Exact output SHA-256: `" + factoryv1.HashText(renderDemoEvidence(request)) + "`",
		"- Human actor: `human-actor`",
		"- Source `test`: identity `source:test`, URI `test://source`, SHA-256 `" + strings.Repeat("a", 64) + "`",
		"- Non-production only: `true`",
		"- Allowed action: `repo.pull_request.mark_ready`",
		"- Authorized target: `transpara-ai/demo`",
		"- Bounded budget: attempts `30`, tokens `1000`, cost micros `1000`",
		"- `AC1` (`low`): Only the evidence file changes.",
		"- Named validation argv: `[\"git\",\"diff\",\"--check\"]`",
		"- GitHub check policy: read the exact non-empty required context list",
		"- Diff policy: the worktree and committed diff may contain only the exact bounded output path",
		"CFADA consumes an independent-family artifact binding this exact design Git blob with zero blockers.",
		"CFAR consumes an independent-family artifact binding that same exact PR head with zero blockers.",
		"Human Review is terminal: the runner will not merge, deploy, publish, or mutate the protected/default branch.",
		"Operation #86 path/state/evidence",
		"~~~~markdown\n" + renderDemoEvidence(request) + "~~~~",
	} {
		if !strings.Contains(design, want) {
			t.Fatalf("design lacks exact contract fragment %q\n%s", want, design)
		}
	}
	for _, forbidden := range []string{"attempt_id", "event_id", "occurred_at", "elapsed_ms", "work_artifact_id", "volatile-attempt-marker", "volatile-event-marker", "volatile-work-marker"} {
		if strings.Contains(design, forbidden) || strings.Contains(renderDemoEvidence(request), forbidden) {
			t.Fatalf("deterministic design/output contains volatile field %q", forbidden)
		}
	}
	if strings.HasPrefix(evidenceRelativePath(request), "docs/") {
		t.Fatalf("evidence path %q is inside a repository publication root", evidenceRelativePath(request))
	}
}

func TestRequiredChecksRequireEveryConfiguredContext(t *testing.T) {
	commands := &requiredPolicyCommander{
		policy: `{"contexts":["verify","cross-family-adversarial-review"]}`,
		rollup: `{"statusCheckRollup":[{"name":"verify","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://checks.invalid/verify"}]}`,
	}
	runner := &demoRunner{config: config{GHExecutable: "gh"}, commands: commands}
	checks, passing, err := runner.requiredChecks(context.Background(), repositoryConfig{Root: t.TempDir(), Identity: "transpara-ai/docs", BaseBranch: "main"}, 283)
	if err != nil {
		t.Fatalf("requiredChecks: %v", err)
	}
	if passing || len(checks) != 2 || checks[0].Bucket != "pass" || checks[1].Name != "cross-family-adversarial-review" || checks[1].Bucket != "pending" {
		t.Fatalf("checks=%+v passing=%v, want missing required context to fail closed", checks, passing)
	}
	if commands.calls != 2 {
		t.Fatalf("gh calls=%d, want policy and rollup queries", commands.calls)
	}
}

func TestRequiredChecksPassExactConfiguredContexts(t *testing.T) {
	commands := &requiredPolicyCommander{
		policy: `{"contexts":["verify","cross-family-adversarial-review"]}`,
		rollup: `{"statusCheckRollup":[{"name":"verify","status":"COMPLETED","conclusion":"SUCCESS"},{"context":"cross-family-adversarial-review","state":"SUCCESS"}]}`,
	}
	runner := &demoRunner{config: config{GHExecutable: "gh"}, commands: commands}
	checks, passing, err := runner.requiredChecks(context.Background(), repositoryConfig{Root: t.TempDir(), Identity: "transpara-ai/docs", BaseBranch: "main"}, 283)
	if err != nil || !passing || len(checks) != 2 {
		t.Fatalf("checks=%+v passing=%v err=%v, want exact required contexts passing", checks, passing, err)
	}
}

func TestRequiredChecksRejectEmptyPolicy(t *testing.T) {
	commands := &requiredPolicyCommander{policy: `{"contexts":[]}`}
	runner := &demoRunner{config: config{GHExecutable: "gh"}, commands: commands}
	checks, passing, err := runner.requiredChecks(context.Background(), repositoryConfig{Root: t.TempDir(), Identity: "transpara-ai/docs", BaseBranch: "main"}, 283)
	if err == nil || passing || len(checks) != 0 {
		t.Fatalf("checks=%+v passing=%v err=%v, want empty policy rejected", checks, passing, err)
	}
}

func runGitTest(t *testing.T, dir, executable string, args ...string) string {
	t.Helper()
	command := exec.Command(executable, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func TestPrivateConfigRejectsBroadPermissions(t *testing.T) {
	state := privateTempDir(t)
	path := filepath.Join(state, "config.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"factory-v1-demo-runner-v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(state, path); err == nil {
		t.Fatal("broadly readable config was accepted")
	}
}

func TestMainStrictJSONEmitsOnlyResult(t *testing.T) {
	state := privateTempDir(t)
	repositoryRoot := t.TempDir()
	cfg := testConfig(t, repositoryRoot)
	cfg.ReviewerEvidenceDir = "reviewer-artifacts"
	configPath := filepath.Join(state, "config.json")
	if err := writePrivateJSON(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	request := testRunRequest(t, repositoryRoot, factoryv1.StageIngestWork)
	raw, _ := json.Marshal(request)
	var stdout, stderr bytes.Buffer
	exit := runMain(context.Background(), []string{"--state-dir", state}, bytes.NewReader(raw), &stdout, &stderr, execCommander{})
	if exit != 0 {
		t.Fatalf("runMain exit=%d stderr=%s", exit, stderr.String())
	}
	var result factoryv1.RunResult
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not one result JSON: %v; %s", err, stdout.String())
	}
	if result.Status != factoryv1.RunnerPassed || stderr.Len() != 0 {
		t.Fatalf("result=%+v stderr=%s", result, stderr.String())
	}
}

var _ commander = (*pinningCommander)(nil)
var _ commander = (*githubFlowCommander)(nil)
