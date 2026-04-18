# Hive CLI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `cmd/hive`'s flat-flag dispatch with a verb-based subcommand tree (`hive civilization run|daemon`, `hive pipeline run|daemon`, `hive role <name> run|daemon`, `hive ingest`, `hive council`) per `docs/superpowers/specs/2026-04-18-hive-cli-redesign-design.md`.

**Architecture:** Hand-rolled subcommand router in `cmd/hive/router.go`. `main()` calls `routeAndDispatch(os.Args[1:])`, which switches on `args[0]` and delegates to per-verb command functions. Each command function owns its own `flag.FlagSet`, parses its scoped flags, and calls the existing `run*` function (`runLegacy`, `runPipeline`, `runDaemon`, `runRunner`, `runIngest`, `runCouncilCmd`). The internal `pkg/hive.Config.Loop` field and `pkg/loop.Config.Keepalive` field stay unchanged — only the user-facing flag is removed and encoded by the `daemon` sub-verb.

**Tech Stack:** Go 1.22+, standard library `flag` package (no new dependencies). Existing `pkg/hive`, `pkg/loop`, `pkg/runner`, `pkg/api`, `pkg/reconciliation` packages are untouched.

---

## File Structure

**Create:**
- `cmd/hive/router.go` — `routeAndDispatch()` plus per-verb `cmd*()` functions.
- `cmd/hive/router_test.go` — Routing tests (no-args, unknown verb, each verb's flag parsing).

**Modify:**
- `cmd/hive/main.go` — Strip `run()` of flag parsing and dispatch; delegate to `routeAndDispatch()`. Update top docstring. Keep all `run*` business-logic functions and helpers.
- `cmd/hive/dispatch_test.go` — Update error-message assertions for the new help text.
- `.claude/skills/hive-lifecycle/SKILL.md` — Update every `go run ./cmd/hive` example and the help summary.
- `docs/ARCHITECTURE.md`, `docs/OPERATOR.md`, `docs/ollama-integration.md`, `README.md`, `CLAUDE.md` — Update invocation examples.

**Untouched:**
- `pkg/hive/runtime.go` (Loop field stays).
- `pkg/loop/loop.go` (Keepalive field stays).
- `pkg/runner/*`, `pkg/api/*`, `pkg/reconciliation/*`.
- `runIngest`, `runRunner`, `runPipeline`, `runDaemon`, `runCouncilCmd`, `runLegacy` signatures and bodies.

## Verb-to-Function Mapping (reference)

| New invocation | Existing function called | Notes |
|---|---|---|
| `hive civilization run --human X --idea "..."` | `runLegacy(human, idea, store, approveRequests, approveRoles, repo, false, space, api)` | `loop=false` |
| `hive civilization run --human X --spec PATH` | `runIngest(spec, space, api, "high")` then `runLegacy(human, "", ...)` | spec ingested via API |
| `hive civilization daemon --human X` | `runLegacy(human, "", store, approveRequests, approveRoles, repo, true, space, api)` | `loop=true` |
| `hive civilization daemon --human X --seed-spec PATH` | `runIngest(seed-spec, space, api, "high")` then `runLegacy(human, "", ..., true, ...)` | spec ingested before runtime starts |
| `hive pipeline run` | `runPipeline(space, api, repo, budget, agentID, repoMap, prMode, worktrees, autoClone, store)` | one cycle |
| `hive pipeline daemon` | `runDaemon(space, api, repo, budget, agentID, repoMap, interval, prMode, worktrees, autoClone, store)` | loops |
| `hive role <name> run` | `runRunner(role, space, api, repo, budget, agentID, true, prMode)` | `oneShot=true` |
| `hive role <name> daemon` | `runRunner(role, space, api, repo, budget, agentID, false, prMode)` | `oneShot=false` |
| `hive ingest <file>` | `runIngest(file, space, api, priority)` | unchanged |
| `hive council [--topic ...]` | `runCouncilCmd(space, api, repo, budget, topic)` | unchanged |

---

## Task 1: Scaffold router with no-args test

**Files:**
- Create: `cmd/hive/router.go`
- Create: `cmd/hive/router_test.go`

- [ ] **Step 1: Write the failing test for no-args dispatch**

Append to `cmd/hive/router_test.go`:

```go
package main

import (
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
}
```

- [ ] **Step 2: Run test, expect compile failure**

```bash
cd /Transpara/transpara-ai/data/repos/lovyou-ai-hive
go test ./cmd/hive/ -run TestRouteAndDispatchNoArgs -v
```
Expected: `undefined: routeAndDispatch`.

- [ ] **Step 3: Create router skeleton**

Create `cmd/hive/router.go`:

```go
package main

import (
	"fmt"
	"strings"
)

// routeAndDispatch is the entry point for the subcommand router.
// It switches on args[0] and delegates to the matching cmd* function.
// Returns an error listing the available verbs when no verb is given.
func routeAndDispatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive <verb> [subverb] [flags]\n\n%s", helpText())
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "civilization":
		return cmdCivilization(rest)
	case "pipeline":
		return cmdPipeline(rest)
	case "role":
		return cmdRole(rest)
	case "ingest":
		return cmdIngest(rest)
	case "council":
		return cmdCouncil(rest)
	case "-h", "--help", "help":
		fmt.Println(helpText())
		return nil
	default:
		return fmt.Errorf("unknown verb %q\n\n%s", verb, helpText())
	}
}

// helpText returns the top-level help string. Each subcommand prints its own
// flag-level help via its FlagSet's PrintDefaults().
func helpText() string {
	var b strings.Builder
	b.WriteString("Verbs:\n")
	b.WriteString("  civilization run         Multi-agent runtime, one-shot (seed and exit at quiescence)\n")
	b.WriteString("  civilization daemon      Multi-agent runtime, long-running (block on bus)\n")
	b.WriteString("  pipeline run             Scout→Builder→Critic, one cycle\n")
	b.WriteString("  pipeline daemon          Scout→Builder→Critic, loops at --interval\n")
	b.WriteString("  role <name> run          Single agent (builder|scout|critic|monitor), one task\n")
	b.WriteString("  role <name> daemon       Single agent, continuous with throttling\n")
	b.WriteString("  ingest <file>            Post a markdown spec as a task\n")
	b.WriteString("  council [--topic ...]    Convene one deliberation\n")
	b.WriteString("\nRun 'hive <verb> --help' for verb-specific flags.\n")
	return b.String()
}

// Stubs — replaced by real implementations in later tasks.
func cmdCivilization(args []string) error { return fmt.Errorf("civilization: not implemented") }
func cmdPipeline(args []string) error     { return fmt.Errorf("pipeline: not implemented") }
func cmdRole(args []string) error         { return fmt.Errorf("role: not implemented") }
func cmdIngest(args []string) error       { return fmt.Errorf("ingest: not implemented") }
func cmdCouncil(args []string) error      { return fmt.Errorf("council: not implemented") }
```

- [ ] **Step 4: Run the no-args test, expect pass**

```bash
go test ./cmd/hive/ -run TestRouteAndDispatchNoArgs -v
```
Expected: PASS.

- [ ] **Step 5: Add unknown-verb test**

Append to `cmd/hive/router_test.go`:

```go
func TestRouteAndDispatchUnknownVerb(t *testing.T) {
	err := routeAndDispatch([]string{"banana"})
	if err == nil {
		t.Fatal("expected error for unknown verb")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should name the unknown verb, got: %s", err.Error())
	}
}
```

- [ ] **Step 6: Run; expect pass**

```bash
go test ./cmd/hive/ -run TestRouteAndDispatch -v
```
Expected: both tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/hive/router.go cmd/hive/router_test.go
git commit -m "feat(cli): scaffold subcommand router with help text and stubs"
```

---

## Task 2: Implement `cmd civilization` (run + daemon)

**Files:**
- Modify: `cmd/hive/router.go` (replace `cmdCivilization` stub)
- Modify: `cmd/hive/router_test.go`

- [ ] **Step 1: Write flag-parsing tests**

Append to `cmd/hive/router_test.go`:

```go
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

func TestCmdCivilizationUnknownSubverb(t *testing.T) {
	err := cmdCivilization([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run; expect failure (stub returns wrong error)**

```bash
go test ./cmd/hive/ -run TestCmdCivilization -v
```
Expected: all three FAIL (stub returns "not implemented").

- [ ] **Step 3: Replace `cmdCivilization` stub with real implementation**

In `cmd/hive/router.go`, delete the stub line for `cmdCivilization` and replace with:

```go
func cmdCivilization(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive civilization <run|daemon> [flags]")
	}
	subverb := args[0]
	rest := args[1:]
	switch subverb {
	case "run":
		return cmdCivilizationRun(rest)
	case "daemon":
		return cmdCivilizationDaemon(rest)
	case "-h", "--help":
		fmt.Println("usage: hive civilization <run|daemon> [flags]")
		fmt.Println("\nRun 'hive civilization run --help' or 'hive civilization daemon --help' for flags.")
		return nil
	default:
		return fmt.Errorf("unknown civilization subverb %q (want run|daemon)", subverb)
	}
}

func cmdCivilizationRun(args []string) error {
	fs := flag.NewFlagSet("civilization run", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	idea := fs.String("idea", "", "Freeform seed for the runtime")
	spec := fs.String("spec", "", "Path to markdown spec file (POSTed via hive ingest channel)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repo := fs.String("repo", "", "Path to repo for Operate (default: current dir)")
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *spec != "" {
		if err := runIngest(*spec, *space, *apiBase, "high"); err != nil {
			return fmt.Errorf("ingest spec: %w", err)
		}
	}
	return runLegacy(*human, *idea, *storeDSN, *approveRequests, *approveRoles, *repo, false, *space, *apiBase)
}

func cmdCivilizationDaemon(args []string) error {
	fs := flag.NewFlagSet("civilization daemon", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	seedSpec := fs.String("seed-spec", "", "Optional initial spec to ingest before daemon starts")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repo := fs.String("repo", "", "Path to repo for Operate (default: current dir)")
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *seedSpec != "" {
		if err := runIngest(*seedSpec, *space, *apiBase, "high"); err != nil {
			return fmt.Errorf("ingest seed-spec: %w", err)
		}
	}
	return runLegacy(*human, "", *storeDSN, *approveRequests, *approveRoles, *repo, true, *space, *apiBase)
}
```

Add `"flag"` to the imports at the top of `router.go`:

```go
import (
	"flag"
	"fmt"
	"strings"
)
```

- [ ] **Step 4: Run civilization tests; expect pass**

```bash
go test ./cmd/hive/ -run TestCmdCivilization -v
```
Expected: all three PASS.

- [ ] **Step 5: Verify the package still compiles**

```bash
go build ./cmd/hive/
```
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add cmd/hive/router.go cmd/hive/router_test.go
git commit -m "feat(cli): implement civilization run/daemon subcommands"
```

---

## Task 3: Implement `cmd pipeline` (run + daemon)

**Files:**
- Modify: `cmd/hive/router.go` (replace `cmdPipeline` stub)
- Modify: `cmd/hive/router_test.go`

- [ ] **Step 1: Write tests**

Append to `cmd/hive/router_test.go`:

```go
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
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./cmd/hive/ -run TestCmdPipeline -v
```
Expected: both FAIL (stub returns "not implemented").

- [ ] **Step 3: Replace stub with implementation**

In `cmd/hive/router.go`, replace the `cmdPipeline` stub with:

```go
func cmdPipeline(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive pipeline <run|daemon> [flags]")
	}
	subverb := args[0]
	rest := args[1:]
	switch subverb {
	case "run":
		return cmdPipelineRun(rest)
	case "daemon":
		return cmdPipelineDaemon(rest)
	case "-h", "--help":
		fmt.Println("usage: hive pipeline <run|daemon> [flags]")
		return nil
	default:
		return fmt.Errorf("unknown pipeline subverb %q (want run|daemon)", subverb)
	}
}

func pipelineFlags(fs *flag.FlagSet) (space, apiBase, repo, agentID, storeDSN, repos *string, budget *float64, prMode, worktrees, autoClone *bool) {
	space = fs.String("space", "hive", "lovyou.ai space slug")
	apiBase = fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	repo = fs.String("repo", "", "Path to repo (default: current dir)")
	agentID = fs.String("agent-id", "", "Agent's lovyou.ai user ID (filters task assignment)")
	storeDSN = fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repos = fs.String("repos", "", "Named repos: name=path,name=path")
	budget = fs.Float64("budget", 10.0, "Daily budget in USD")
	prMode = fs.Bool("pr", false, "Create feature branch and open PR instead of pushing to main")
	worktrees = fs.Bool("worktrees", false, "Each Builder task gets its own git worktree")
	autoClone = fs.Bool("auto-clone", false, "Clone missing repos from registry URLs before each cycle")
	return
}

func cmdPipelineRun(args []string) error {
	fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	space, apiBase, repo, agentID, storeDSN, repos, budget, prMode, worktrees, autoClone := pipelineFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoMap := parseRepos(*repos, *repo)
	return runPipeline(*space, *apiBase, *repo, *budget, *agentID, repoMap, *prMode, *worktrees, *autoClone, *storeDSN)
}

func cmdPipelineDaemon(args []string) error {
	fs := flag.NewFlagSet("pipeline daemon", flag.ContinueOnError)
	space, apiBase, repo, agentID, storeDSN, repos, budget, prMode, worktrees, autoClone := pipelineFlags(fs)
	interval := fs.Duration("interval", 30*time.Minute, "Pipeline cycle interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoMap := parseRepos(*repos, *repo)
	return runDaemon(*space, *apiBase, *repo, *budget, *agentID, repoMap, *interval, *prMode, *worktrees, *autoClone, *storeDSN)
}
```

Add `"time"` to imports if not already present:

```go
import (
	"flag"
	"fmt"
	"strings"
	"time"
)
```

- [ ] **Step 4: Run pipeline tests; expect pass**

```bash
go test ./cmd/hive/ -run TestCmdPipeline -v
```
Expected: both PASS.

- [ ] **Step 5: Compile check**

```bash
go build ./cmd/hive/
```
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add cmd/hive/router.go cmd/hive/router_test.go
git commit -m "feat(cli): implement pipeline run/daemon subcommands"
```

---

## Task 4: Implement `cmd role` (run + daemon)

**Files:**
- Modify: `cmd/hive/router.go` (replace `cmdRole` stub)
- Modify: `cmd/hive/router_test.go`

- [ ] **Step 1: Write tests**

Append to `cmd/hive/router_test.go`:

```go
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
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./cmd/hive/ -run TestCmdRole -v
```
Expected: all FAIL.

- [ ] **Step 3: Replace stub with implementation**

In `cmd/hive/router.go`, replace the `cmdRole` stub with:

```go
func cmdRole(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive role <name> <run|daemon> [flags] (role name required)")
	}
	roleName := args[0]
	if len(args) < 2 {
		return fmt.Errorf("usage: hive role %s <run|daemon> [flags]", roleName)
	}
	subverb := args[1]
	rest := args[2:]
	switch subverb {
	case "run":
		return cmdRoleRun(roleName, rest)
	case "daemon":
		return cmdRoleDaemon(roleName, rest)
	case "-h", "--help":
		fmt.Printf("usage: hive role %s <run|daemon> [flags]\n", roleName)
		return nil
	default:
		return fmt.Errorf("unknown role subverb %q (want run|daemon)", subverb)
	}
}

func roleFlags(fs *flag.FlagSet) (space, apiBase, repo, agentID *string, budget *float64, prMode *bool) {
	space = fs.String("space", "hive", "lovyou.ai space slug")
	apiBase = fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	repo = fs.String("repo", "", "Path to repo (default: current dir)")
	agentID = fs.String("agent-id", "", "Agent's lovyou.ai user ID (filters task assignment)")
	budget = fs.Float64("budget", 10.0, "Daily budget in USD")
	prMode = fs.Bool("pr", false, "Create feature branch and open PR instead of pushing to main")
	return
}

func cmdRoleRun(role string, args []string) error {
	fs := flag.NewFlagSet("role "+role+" run", flag.ContinueOnError)
	space, apiBase, repo, agentID, budget, prMode := roleFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runRunner(role, *space, *apiBase, *repo, *budget, *agentID, true, *prMode)
}

func cmdRoleDaemon(role string, args []string) error {
	fs := flag.NewFlagSet("role "+role+" daemon", flag.ContinueOnError)
	space, apiBase, repo, agentID, budget, prMode := roleFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runRunner(role, *space, *apiBase, *repo, *budget, *agentID, false, *prMode)
}
```

- [ ] **Step 4: Run role tests; expect pass**

```bash
go test ./cmd/hive/ -run TestCmdRole -v
```
Expected: all PASS.

- [ ] **Step 5: Compile check**

```bash
go build ./cmd/hive/
```
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add cmd/hive/router.go cmd/hive/router_test.go
git commit -m "feat(cli): implement role <name> run/daemon subcommands"
```

---

## Task 5: Implement `cmd ingest` and `cmd council`

**Files:**
- Modify: `cmd/hive/router.go` (replace `cmdIngest` and `cmdCouncil` stubs)
- Modify: `cmd/hive/router_test.go`

- [ ] **Step 1: Write tests**

Append to `cmd/hive/router_test.go`:

```go
func TestCmdIngestRequiresFile(t *testing.T) {
	err := cmdIngest(nil)
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected file-required error, got: %v", err)
	}
}

func TestCmdCouncilNoArgsOK(t *testing.T) {
	// council with no flags should attempt to dispatch (may fail on API,
	// but should not fail on flag parsing). We assert no flag-parsing error.
	err := cmdCouncil([]string{"--topic", "x", "--api", "http://invalid.local"})
	// We tolerate any error EXCEPT a flag-parsing error.
	if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("flag-parsing error: %v", err)
	}
}
```

- [ ] **Step 2: Run; expect first to FAIL, second may PASS or FAIL**

```bash
go test ./cmd/hive/ -run "TestCmdIngest|TestCmdCouncil" -v
```
Expected: TestCmdIngestRequiresFile FAILs (stub).

- [ ] **Step 3: Replace stubs with implementations**

In `cmd/hive/router.go`, replace the `cmdIngest` and `cmdCouncil` stubs with:

```go
func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	priority := fs.String("priority", "high", "Task priority: low|medium|high|critical")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: hive ingest <file.md> [flags] (spec file required)")
	}
	if len(rest) > 1 {
		return fmt.Errorf("ingest takes exactly one positional argument (spec file path)")
	}
	return runIngest(rest[0], *space, *apiBase, *priority)
}

func cmdCouncil(args []string) error {
	fs := flag.NewFlagSet("council", flag.ContinueOnError)
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	repo := fs.String("repo", "", "Path to repo (default: current dir)")
	budget := fs.Float64("budget", 10.0, "Daily budget in USD")
	topic := fs.String("topic", "", "Focus the council on a specific question")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runCouncilCmd(*space, *apiBase, *repo, *budget, *topic)
}
```

- [ ] **Step 4: Run tests; expect pass**

```bash
go test ./cmd/hive/ -run "TestCmdIngest|TestCmdCouncil" -v
```
Expected: both PASS.

- [ ] **Step 5: Compile check**

```bash
go build ./cmd/hive/
```
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add cmd/hive/router.go cmd/hive/router_test.go
git commit -m "feat(cli): implement ingest and council subcommands"
```

---

## Task 6: Wire `main()` to use the router; remove old dispatch

**Files:**
- Modify: `cmd/hive/main.go` (lines 1–165 approx — `run()` and `printUsage()`)
- Modify: `cmd/hive/dispatch_test.go`

- [ ] **Step 1: Update `dispatch_test.go` to match the new error**

Replace the entire contents of `cmd/hive/dispatch_test.go` with:

```go
package main

import (
	"strings"
	"testing"
)

// TestRunNoFlags verifies that running with no verb produces a help-style
// error mentioning the available verbs.
func TestRunNoFlags(t *testing.T) {
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
```

- [ ] **Step 2: Run; expect FAIL because main.go's `run()` still owns flag parsing**

```bash
go test ./cmd/hive/ -run TestRunNoFlags -v
```
Expected: PASS *if* router_test.go's TestRouteAndDispatchNoArgs already covers this — but main.go's old `run()` still exists and may interfere when other tests reset `flag.CommandLine`. We accept this; the next step replaces it.

- [ ] **Step 3: Strip flag parsing and dispatch from `cmd/hive/main.go`**

Replace `cmd/hive/main.go` lines 1–165 (the entire `run()` and `printUsage()` functions and the file's leading docstring) with the following. Keep everything from line 167 onward (`// ─── Ingest mode ───` and below) untouched.

Replace the top of the file (lines 1–18, the docstring) with:

```go
// Command hive runs hive agents.
//
// Usage:
//   hive <verb> [subverb] [flags]
//
// Verbs:
//   hive civilization run         Multi-agent runtime, one-shot
//   hive civilization daemon      Multi-agent runtime, long-running
//   hive pipeline run             Scout→Builder→Critic, one cycle
//   hive pipeline daemon          Scout→Builder→Critic, looping
//   hive role <name> run          Single agent (builder|scout|critic|monitor), one task
//   hive role <name> daemon       Single agent, continuous
//   hive ingest <file>            Post a markdown spec as a task
//   hive council [--topic ...]    Convene one deliberation
//
// Run 'hive <verb> --help' for verb-specific flags.
package main
```

Then replace lines 59–165 (the entire `run()` and `printUsage()` functions) with:

```go
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	return routeAndDispatch(os.Args[1:])
}
```

After this edit, the `flag` import in `main.go` will be unused. Remove it from the import block (find the `"flag"` line in the import group and delete it). Verify `os` is still imported (it is, for `os.Args` and `os.Exit`).

- [ ] **Step 4: Compile check — fix any unused imports**

```bash
go build ./cmd/hive/
```
Expected: success. If you see an unused-import error for `flag` or `time`, remove that import line. (`time` is still used elsewhere in main.go, e.g. `runDaemon` uses `time.Duration`. `flag` is no longer used in main.go and must be removed.)

- [ ] **Step 5: Run all cmd/hive tests**

```bash
go test ./cmd/hive/ -v
```
Expected: all PASS, including TestRunNoFlags, TestRouteAndDispatch*, TestCmdCivilization*, TestCmdPipeline*, TestCmdRole*, TestCmdIngest*, TestCmdCouncil*, TestRunIngest, TestDaemonResetToMain.

- [ ] **Step 6: Smoke-test the binary's help output**

```bash
go run ./cmd/hive 2>&1 | head -15
```
Expected: contains "Verbs:" and lists all five verbs.

```bash
go run ./cmd/hive --help 2>&1 | head -15
```
Expected: same help text.

```bash
go run ./cmd/hive banana 2>&1 | head -3
```
Expected: error mentioning "banana" and the verb list. Exit code 1.

- [ ] **Step 7: Commit**

```bash
git add cmd/hive/main.go cmd/hive/dispatch_test.go
git commit -m "refactor(cli): replace flag-soup dispatch with subcommand router"
```

---

## Task 7: Update `.claude/skills/hive-lifecycle/SKILL.md`

**Files:**
- Modify: `.claude/skills/hive-lifecycle/SKILL.md`

The file has many `go run ./cmd/hive` invocations. Update each one. The file uses heredoc-style code blocks; preserve indentation and surrounding context.

- [ ] **Step 1: Update the `hive help` summary block (around line 12–46)**

Find the code block that starts with `Hive Lifecycle Commands:` and update the lines that map to renamed flags. Specifically:

The summary block describes user-facing commands like `hive up`, `hive down` — these are skill macros, not CLI args, so leave them alone. Only the underlying `go run` invocations elsewhere in the file change.

No edit needed in this section. Move on.

- [ ] **Step 2: Update the "Hive Up" code blocks (around lines 127–185)**

In the file, find the code block that contains:

```bash
go run ./cmd/hive \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

Replace with:

```bash
go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

For the "background" variant (next code block starting `nohup go run ./cmd/hive`):

```bash
nohup go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here" \
    > /tmp/hive.log 2>&1 &
```

For the "Hive Up with Auto-Approve" block:

```bash
go run ./cmd/hive civilization run \
    --human Michael \
    --approve-requests \
    --approve-roles \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

For the "Hive Up with Loop" block — this becomes daemon and drops `--idea`:

```bash
go run ./cmd/hive civilization daemon \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive
```

For the "Full Autonomy" block:

```bash
go run ./cmd/hive civilization daemon \
    --human Michael \
    --approve-requests \
    --approve-roles \
    --store postgres://hive:hive@localhost:5432/hive
```

- [ ] **Step 3: Update the "Hive Run (Runner / Pipeline Mode)" section (around lines 220–235)**

Replace each of:

```bash
go run ./cmd/hive --role builder --space hive --one-shot
go run ./cmd/hive --pipeline --space hive
go run ./cmd/hive --pipeline --interval 15m --space hive
go run ./cmd/hive --pipeline --one-shot --space hive
go run ./cmd/hive --council --topic "Should we add a caching layer?"
```

with:

```bash
go run ./cmd/hive role builder run --space hive
go run ./cmd/hive pipeline daemon --space hive
go run ./cmd/hive pipeline daemon --interval 15m --space hive
go run ./cmd/hive pipeline run --space hive
go run ./cmd/hive council --topic "Should we add a caching layer?"
```

- [ ] **Step 4: Update the "Local Pipeline" block (around line 258)**

Find:

```bash
go run ./cmd/hive \
    --pipeline \
    --api http://localhost:8082 \
    --space hive \
    --store postgres://hive:hive@localhost:5432/hive \
    --human Michael \
    --pr
```

Replace with (note: removing `--human` since it's not a pipeline flag, and the prior version already ignored it):

```bash
go run ./cmd/hive pipeline daemon \
    --api http://localhost:8082 \
    --space hive \
    --store postgres://hive:hive@localhost:5432/hive \
    --pr
```

- [ ] **Step 5: Update the "Hive Restart" block (around line 337)**

Find:

```bash
go run ./cmd/hive \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

Replace with:

```bash
go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

- [ ] **Step 6: Update the "Flag Reference" section**

This is the biggest section. Find the "Legacy Runtime" table around the file's mid-bottom and replace it with a verb-organized version. Find this header:

```markdown
### Legacy Runtime (multi-agent coordination)

| Flag | Default | Purpose |
|------|---------|---------|
| `--human` | (required) | Human operator name — always `Michael` |
| `--idea` | (required) | Seed idea for the agent civilization |
| `--store` | in-memory | Postgres DSN for persistence |
| `--approve-requests` | false | Auto-approve authority requests (file writes, git ops) |
| `--approve-roles` | false | Auto-approve role proposals (skip Guardian approval) |
| `--loop` | false | Keep agents alive when idle (block on bus) |
| `--repo` | current dir | Path to repo for Implementer's Operate |
```

Replace with:

```markdown
### `civilization run` (multi-agent, one-shot)

| Flag | Default | Purpose |
|------|---------|---------|
| `--human` | (required) | Human operator name — always `Michael` |
| `--idea` | "" | Freeform seed |
| `--spec` | "" | Path to markdown spec file (POSTed via ingest channel) |
| `--store` | in-memory | Postgres DSN for persistence |
| `--repo` | current dir | Path to repo for Implementer's Operate |
| `--approve-requests` | false | Auto-approve authority requests |
| `--approve-roles` | false | Auto-approve role proposals |
| `--space`, `--api` | `hive`, `https://lovyou.ai` | lovyou.ai targeting |

### `civilization daemon` (multi-agent, long-running)

Same flags as `civilization run` except `--idea` becomes `--seed-spec PATH` (optional initial spec; daemon survives past quiescence regardless).
```

For the "Runner / Pipeline Mode" table just below it, find:

```markdown
| Flag | Default | Purpose |
|------|---------|---------|
| `--role` | | Single agent: builder, scout, critic, monitor |
| `--pipeline` | false | Scout + Builder + Critic pipeline (loops at `--interval`) |
| `--interval` | 30m | Pipeline cycle interval |
| `--council` | false | Convene all agents for deliberation |
| `--topic` | | Focus question for council |
| `--space` | hive | lovyou.ai space slug |
| `--budget` | 10.0 | Daily budget in USD |
| `--one-shot` | false | Run once then exit (single pipeline cycle or single task) |
| `--pr` | false | Create feature branch + PR instead of pushing to main |
| `--worktrees` | false | Each Builder task gets its own git worktree |
| `--repos` | | Named repos: `name=path,name=path` |
| `--auto-clone` | false | Clone missing repos from registry URLs |
```

Replace with:

```markdown
### `pipeline run` / `pipeline daemon`

| Flag | Default | Purpose | Where |
|------|---------|---------|-------|
| `--space` | `hive` | lovyou.ai space slug | both |
| `--api` | `https://lovyou.ai` | lovyou.ai API base URL | both |
| `--repo` | current dir | Path to repo | both |
| `--repos` | "" | Named repos: `name=path,...` | both |
| `--budget` | 10.0 | Daily budget in USD | both |
| `--agent-id` | "" | Agent's lovyou.ai user ID | both |
| `--store` | in-memory | Postgres DSN | both |
| `--pr` | false | Branch + PR instead of pushing to main | both |
| `--worktrees` | false | Per-task worktrees | both |
| `--auto-clone` | false | Clone missing repos | both |
| `--interval` | 30m | Cycle interval | `daemon` only |

### `role <name> run` / `role <name> daemon`

| Flag | Default | Purpose |
|------|---------|---------|
| `--space`, `--api`, `--repo`, `--agent-id`, `--budget`, `--pr` | (as above) | Same as pipeline |

`run` runs a single task with throttling disabled. `daemon` runs continuously with throttled phases.

### `council`

| Flag | Default | Purpose |
|------|---------|---------|
| `--topic` | "" | Focus question |
| `--space`, `--api`, `--repo`, `--budget` | (as pipeline) | |

### `ingest <file>`

| Flag | Default | Purpose |
|------|---------|---------|
| `--space`, `--api` | (as pipeline) | |
| `--priority` | `high` | Task priority: low\|medium\|high\|critical |
```

- [ ] **Step 7: Search for any remaining old-form invocations**

```bash
grep -nE "\-\-(pipeline|role|council|ingest|loop|one-shot|human)\b" /Transpara/transpara-ai/data/repos/lovyou-ai-hive/.claude/skills/hive-lifecycle/SKILL.md | grep -v "^[0-9]*:#" | head -20
```

Expected: only matches inside surrounding prose (e.g. "the `--human` parameter") or in the `pkill -f` zombie-kill commands that match by process name (those should keep `--human` because that's what processes were named by). If you see any `go run ./cmd/hive --` invocation form, fix it.

The `pkill -f "/hive --human"` lines should stay — those match against the OLD process command lines for any zombies from before the rename. Add a note above them: `# Matches both old --human form and new "civilization" form via cmd/hive`.

- [ ] **Step 8: Commit**

```bash
git add .claude/skills/hive-lifecycle/SKILL.md
git commit -m "docs(skill): update hive-lifecycle invocations to subcommand verbs"
```

---

## Task 8: Update repo docs (README, CLAUDE.md, OPERATOR, ARCHITECTURE, ollama-integration)

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/OPERATOR.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/ollama-integration.md`

- [ ] **Step 1: Update `README.md`**

Find lines containing `go run ./cmd/hive --human`. Replace each invocation:

`go run ./cmd/hive --human Matt --idea "Build a task management app"` → `go run ./cmd/hive civilization run --human Matt --idea "Build a task management app"`

`go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "..."` → `go run ./cmd/hive civilization run --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "..."`

- [ ] **Step 2: Update `CLAUDE.md`**

In the "Running" section (around lines 148–164), replace each invocation. The five lines to change:

```
go run ./cmd/hive --human Matt --idea "Build a task management app with kanban boards"
go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a CLI tool"
go run ./cmd/hive --human Matt --approve-requests --approve-roles --idea "Build a REST API"
go run ./cmd/hive --human Matt --approve-requests --approve-roles --loop --store "postgres://hive:hive@localhost:5432/hive"
go run ./cmd/hive --human Matt --approve-requests --repo /path/to/repo --idea "add error handling to the API"
```

Become (the `--loop` one becomes `daemon` and drops `--idea`; others become `civilization run`):

```
go run ./cmd/hive civilization run --human Matt --idea "Build a task management app with kanban boards"
go run ./cmd/hive civilization run --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a CLI tool"
go run ./cmd/hive civilization run --human Matt --approve-requests --approve-roles --idea "Build a REST API"
go run ./cmd/hive civilization daemon --human Matt --approve-requests --approve-roles --store "postgres://hive:hive@localhost:5432/hive"
go run ./cmd/hive civilization run --human Matt --approve-requests --repo /path/to/repo --idea "add error handling to the API"
```

Also update the "Flags" bulleted list immediately below those examples. Find:

```
- `--human` — Human operator name (required)
- `--idea` — Seed idea for agents to work on
- `--store` — Store DSN (`postgres://...` or empty for in-memory)
- `--approve-requests` — Auto-approve authority requests (file writes, git ops)
- `--approve-roles` — Auto-approve role proposals (skip Guardian approval)
- `--loop` — Keep agents alive when idle (block on bus for webhook work)
- `--repo` — Path to repo for Implementer's Operate (default: current dir)
```

Replace with:

```
Subcommands:
- `civilization run` / `civilization daemon` — multi-agent runtime (one-shot or long-running)
- `pipeline run` / `pipeline daemon` — Scout→Builder→Critic state machine
- `role <name> run` / `role <name> daemon` — single agent
- `ingest <file>` — post a markdown spec as a task
- `council [--topic STR]` — convene one deliberation

Civilization flags:
- `--human` — Human operator name (required)
- `--idea` — Freeform seed (run only)
- `--spec PATH` — Markdown spec file as seed (run only)
- `--seed-spec PATH` — Optional initial spec for daemon
- `--store DSN` — Postgres DSN or empty for in-memory
- `--repo PATH` — Path for agent file ops (default: current dir)
- `--approve-requests` — Auto-approve authority requests
- `--approve-roles` — Auto-approve role proposals

Run `go run ./cmd/hive <verb> --help` for verb-specific flags.
```

- [ ] **Step 3: Update `docs/OPERATOR.md`**

Find:

```
go run ./cmd/hive --human Matt --idea "Build a task management app"
go run ./cmd/hive --human Matt --store "postgres://..." --idea "..."
```

Replace with:

```
go run ./cmd/hive civilization run --human Matt --idea "Build a task management app"
go run ./cmd/hive civilization run --human Matt --store "postgres://..." --idea "..."
```

- [ ] **Step 4: Update `docs/ARCHITECTURE.md`**

Around line 128–135, find:

```bash
go run ./cmd/hive \
  --human Matt \
  --store postgres://hive:hive@localhost:5432/hive \
  --repo /path/to/target \
  --idea "Build feature X" \
  --yes
```

Replace with (note: `--yes` was a stale flag that does not exist in the current `cmd/hive`; drop it. The closest replacement is `--approve-requests --approve-roles`):

```bash
go run ./cmd/hive civilization run \
  --human Matt \
  --store postgres://hive:hive@localhost:5432/hive \
  --repo /path/to/target \
  --idea "Build feature X" \
  --approve-requests \
  --approve-roles
```

- [ ] **Step 5: Update `docs/ollama-integration.md`**

Find the two lines:

```
HIVE_PROVIDER=ollama HIVE_MODEL=gemma4:27b go run ./cmd/hive --human Matt --idea "Build a task manager"
OLLAMA_HOST=http://192.168.1.10:11434 HIVE_PROVIDER=ollama HIVE_MODEL=gemma4:27b go run ./cmd/hive --human Matt --idea "..."
```

Replace with:

```
HIVE_PROVIDER=ollama HIVE_MODEL=gemma4:27b go run ./cmd/hive civilization run --human Matt --idea "Build a task manager"
OLLAMA_HOST=http://192.168.1.10:11434 HIVE_PROVIDER=ollama HIVE_MODEL=gemma4:27b go run ./cmd/hive civilization run --human Matt --idea "..."
```

- [ ] **Step 6: Sweep for any remaining stragglers**

```bash
grep -rnE "go run \./cmd/hive --(human|pipeline|role|council|ingest|loop|one-shot)\b" /Transpara/transpara-ai/data/repos/lovyou-ai-hive --include="*.md" 2>&1
```

Expected: no matches. If any appear, update them with the same pattern (find the verb that matches the old flag, prepend it, drop the flag).

Note: the file `loop/council-full-civilization-2026-03-25.md` is a historical council transcript — leave it as-is (it's a record of what was said at that time, not a current invocation).

- [ ] **Step 7: Commit**

```bash
git add README.md CLAUDE.md docs/OPERATOR.md docs/ARCHITECTURE.md docs/ollama-integration.md
git commit -m "docs: update hive invocation examples to subcommand verbs"
```

---

## Task 9: Final verification

**Files:** none (read-only checks)

- [ ] **Step 1: Run the full test suite**

```bash
cd /Transpara/transpara-ai/data/repos/lovyou-ai-hive
go test ./...
```

Expected: all tests pass. Pay particular attention to `cmd/hive/` and `pkg/loop/` — they're the most likely to break.

- [ ] **Step 2: Run `go vet`**

```bash
go vet ./...
```

Expected: no warnings.

- [ ] **Step 3: Verify build succeeds**

```bash
go build ./...
```

Expected: success, no output.

- [ ] **Step 4: Smoke-test each verb's --help**

```bash
go run ./cmd/hive
go run ./cmd/hive --help
go run ./cmd/hive civilization
go run ./cmd/hive civilization run --help
go run ./cmd/hive civilization daemon --help
go run ./cmd/hive pipeline run --help
go run ./cmd/hive pipeline daemon --help
go run ./cmd/hive role builder run --help
go run ./cmd/hive role builder daemon --help
go run ./cmd/hive ingest --help
go run ./cmd/hive council --help
```

For each, expected: usage text or flag list, exit code 0 (or 1 with help text — the parser may treat `--help` as an error in `ContinueOnError` mode; that's acceptable as long as the help text is printed).

- [ ] **Step 5: Smoke-test the unknown-verb path**

```bash
go run ./cmd/hive banana 2>&1 | head -10
```

Expected: error mentioning `banana`, then verb list. Exit code 1.

- [ ] **Step 6: Smoke-test civilization run with a quick in-memory invocation**

```bash
timeout 30 go run ./cmd/hive civilization run --human TestUser --idea "say hello and exit" 2>&1 | head -30
```

Expected: hive boots, agents start, you see at least the "Human operator: TestUser" line and some agent activity. Timeout terminates it. The point of this smoke test is that the dispatch reaches `runLegacy` correctly with `loop=false` — not that the agents complete useful work.

If the build complains about missing API key for the agents, that's expected (Claude CLI auth) and proves the dispatch worked; abort the timeout early.

- [ ] **Step 7: Final search for old flag names in cmd/**

```bash
grep -nE "flag\.(Bool|String|Int|Float64|Duration)\(\"(pipeline|role|council|ingest|loop|one-shot|human|idea|approve-requests|approve-roles)\"" /Transpara/transpara-ai/data/repos/lovyou-ai-hive/cmd/hive/*.go
```

Expected: matches ONLY in `cmd/hive/router.go` (the new flag definitions, scoped to subcommands). NO matches in `cmd/hive/main.go`. If main.go still has any of these, the dispatch refactor is incomplete.

- [ ] **Step 8: Commit any small follow-up fixes from steps 1–7**

If any test failed or any check turned up stragglers, fix them and commit:

```bash
git add -p   # review and stage selectively
git commit -m "fix(cli): address verification issues from CLI redesign"
```

If everything passed cleanly, no commit needed. The plan is done.

---

## Self-Review Notes (for the executing engineer)

- **Spec coverage:** every row in the spec's "Migration Mapping" table is covered by tasks 1–6. The four documentation rows (skill + 5 docs) are covered by tasks 7–8. The "Open Questions" section in the spec is explicitly out of scope.
- **Type/name consistency:** the `cmd*` and `cmd*Run`/`cmd*Daemon` naming is uniform. `pipelineFlags` and `roleFlags` are the only helpers and live in `router.go` next to their callers.
- **Backward compatibility:** none. The spec rejected an alias layer. After task 6 lands, every old-form invocation will fail with the unknown-verb error. Task 7 updates the in-tree caller (the skill) and task 8 updates the docs. Any out-of-tree shell scripts are the user's to fix.
- **What this plan does NOT do:** does not collapse `--approve-requests`/`--approve-roles` into `--autonomy=`, does not add a config file, does not split daemon and client, does not change anything in `pkg/`. All four are deferred per the spec's "Out of Scope for Phase A" section.
