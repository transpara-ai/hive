package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/safety"
	"github.com/transpara-ai/work"
)

// ─── factory ────────────────────────────────────────────────────────────────
//
// The Factory is ALWAYS-ON: `factory daemon` runs a governing loop forever
// (Keepalive=true → roles never exit on quiescence), and each Order is a
// bounded sub-flow submitted into that loop. The four subcommands are thin glue
// over seams built earlier in the Dark Factory reunification:
//
//   daemon       → the civilization daemon path with Keepalive forced on.
//   order        → work.SeedFactoryOrder (W1): one Order → one Work task.
//   scan-issues  → scan Transpara-AI GitHub issues, then queue one governed
//                  run request for the existing FactoryOrder dispatcher.
//   advance-issue-scan
//               → release the next issue-scan stage dependency barrier after
//                  its lifecycle contracts are present.
//   record-issue-scan-role-output
//               → record one civic role's durable output evidence for an
//                  issue-scan lifecycle stage without completing the stage.
//   complete-issue-scan-stage
//               → record governed runtime evidence for one issue-scan stage,
//                  complete that stage task, and optionally release the next
//                  stage barrier.
//   request-pr   → (*hive.Runtime).RaiseDraftPRAuthorityRequest (H2): raise the
//                  guardian authority request; the gate HOLDS by design.
//   create-pr    → hive.LoadApprovedDraftPRTarget (the governance gate: load the
//                  recorded approval, refuse if denied/undecided) +
//                  hive.CreateDraftPRFromApprovedDecision (H4) + the live GitHub
//                  creator work.NewEpic11GitHubPullRequestCreator (W2).

func cmdFactory(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive factory <daemon|order|scan-issues|advance-issue-scan|record-issue-scan-role-output|complete-issue-scan-stage|request-pr|create-pr> [flags]", errUsage)
	}
	subverb := args[0]
	rest := args[1:]
	switch subverb {
	case "daemon":
		return cmdFactoryDaemon(rest)
	case "order":
		return cmdFactoryOrder(rest)
	case "scan-issues":
		return cmdFactoryScanIssues(rest)
	case "advance-issue-scan":
		return cmdFactoryAdvanceIssueScan(rest)
	case "record-issue-scan-role-output":
		return cmdFactoryRecordIssueScanRoleOutput(rest)
	case "complete-issue-scan-stage":
		return cmdFactoryCompleteIssueScanStage(rest)
	case "request-pr":
		return cmdFactoryRequestPR(rest)
	case "create-pr":
		return cmdFactoryCreatePR(rest)
	case "-h", "--help":
		fmt.Println("usage: hive factory <daemon|order|scan-issues|advance-issue-scan|record-issue-scan-role-output|complete-issue-scan-stage|request-pr|create-pr> [flags]")
		fmt.Println("\nRun 'hive factory <sub> --help' for subcommand flags.")
		return nil
	default:
		return fmt.Errorf("unknown factory subverb %q (want daemon|order|scan-issues|advance-issue-scan|record-issue-scan-role-output|complete-issue-scan-stage|request-pr|create-pr)", subverb)
	}
}

// cmdFactoryDaemon runs the always-on governing loop. It mirrors
// cmdCivilizationDaemon but forces the long-running Keepalive path: runLegacy's
// loop=true flows to hive.Config.Loop, which runtime.go sets as
// loop.Config.Keepalive — so governance roles block on the bus forever instead
// of exiting at quiescence. This is the terminating-path's opposite by design.
func cmdFactoryDaemon(args []string) error {
	fs := flag.NewFlagSet("factory daemon", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	seedSpec := fs.String("seed-spec", "", "Optional initial spec to seed the loop before it starts")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repo := fs.String("repo", "", "Path to repo for Operate (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for issue-scan implementation targets")
	catalog := fs.String("catalog", "", "Custom YAML model catalog (merged with built-in defaults)")
	catalogReloadInterval := fs.Duration("catalog-reload-interval", 0, "Reload --catalog on this interval for future model resolution; 0 disables")
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "transpara.ai space slug")
	apiBase := fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
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
	// loop=true → Keepalive=true: the governing loop never exits on quiescence.
	return runLegacy(*human, "", *storeDSN, *approveRequests, *approveRoles, *repo, *repoWorkspaceRoot, *catalog, *catalogReloadInterval, true, *space, *apiBase)
}

// cmdFactoryOrder submits one Order into the (separately running) daemon by
// seeding a work.task.created. The --human guard fires BEFORE any side effect
// (no spec read, no store open) so the offline router test stays inert.
func cmdFactoryOrder(args []string) error {
	fs := flag.NewFlagSet("factory order", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	spec := fs.String("spec", "", "Path to the order spec markdown file (required)")
	repo := fs.String("repo", "", "Path to repo for the order's workspace (default: current dir)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	id := fs.String("id", "", "Factory order ID (fo_ prefix; derived from spec name if empty)")
	title := fs.String("title", "", "Order title (defaults to the order ID)")
	catalog := fs.String("catalog", "", "Custom YAML model catalog used once to validate model override flags; does not reload a running daemon")
	modelOverrideFlags := factoryOrderModelOverrideFlags{}
	fs.Var(&modelOverrideFlags.models, "model", "Role-scoped model override role=model (repeatable)")
	fs.Var(&modelOverrideFlags.providers, "provider", "Role-scoped provider override role=provider (repeatable)")
	fs.Var(&modelOverrideFlags.profiles, "profile", "Role-scoped profile override role=profile (repeatable)")
	fs.Var(&modelOverrideFlags.authModes, "auth-mode", "Role-scoped auth-mode opt-in role=subscription|api-key|local (repeatable)")
	fs.Var(&modelOverrideFlags.preferredTiers, "preferred-tier", "Role-scoped preferred tier role=tier (repeatable)")
	fs.Var(&modelOverrideFlags.requiredCapabilities, "required-capability", "Role-scoped required capability role=capability[,capability...] (repeatable)")
	fs.Var(&modelOverrideFlags.maxCosts, "max-cost-per-call-usd", "Role-scoped per-call cost cap role=amount (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *spec == "" {
		return fmt.Errorf("--spec is required")
	}
	modelOverrides, err := parseFactoryOrderModelOverrideFlags(modelOverrideFlags)
	if err != nil {
		return err
	}
	validatedModelOverrides, err := validateFactoryOrderModelOverrides(*catalog, modelOverrides)
	if err != nil {
		return err
	}
	intent, err := os.ReadFile(*spec)
	if err != nil {
		return fmt.Errorf("read spec: %w", err)
	}

	orderID := *id
	if orderID == "" {
		orderID = "fo_" + specStem(*spec)
	}
	orderTitle := *title
	if orderTitle == "" {
		orderTitle = orderID
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fc, err := openFactoryContext(ctx, *storeDSN, *human)
	if err != nil {
		return err
	}
	defer fc.close()

	if *repo == "" {
		*repo = "."
	}

	ts := work.NewTaskStore(fc.store, fc.factory, fc.signer)
	// Each order gets its own conversation ID and (via the task) its own work
	// Workspace. Derive a stable conversation ID from the order ID.
	conv := factoryOrderConversation(orderID)
	causes := fc.headCauses()

	// Optional readiness gates carried by the spec. Absent sections stay empty;
	// the planner attaches them later and work's Readiness gate enforces a
	// non-empty body before the task is assignable.
	dod, ac, testPlan := parseOrderGateSections(string(intent))
	task, err := work.SeedFactoryOrder(ts, fc.humanID, work.FactoryOrder{
		Kind:               work.OrderSoftwarePR,
		ID:                 orderID,
		Title:              orderTitle,
		Intent:             string(intent),
		DefinitionOfDone:   dod,
		AcceptanceCriteria: ac,
		TestPlan:           testPlan,
		ModelOverrides:     workFactoryOrderModelOverrides(validatedModelOverrides),
	}, causes, conv)
	if err != nil {
		return fmt.Errorf("seed factory order: %w", err)
	}
	fmt.Printf("seeded factory order %s as work task %s (conversation %s)\n", orderID, task.ID, conv)
	return nil
}

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type factoryOrderModelOverrideFlags struct {
	models               repeatedStringFlag
	providers            repeatedStringFlag
	profiles             repeatedStringFlag
	authModes            repeatedStringFlag
	preferredTiers       repeatedStringFlag
	requiredCapabilities repeatedStringFlag
	maxCosts             repeatedStringFlag
}

func parseFactoryOrderModelOverrideFlags(flags factoryOrderModelOverrideFlags) ([]hive.ModelOverrideRequest, error) {
	byRole := map[string]*hive.ModelOverrideRequest{}
	var roles []string
	seenFields := map[string]map[string]struct{}{}
	ensure := func(role string) *hive.ModelOverrideRequest {
		if override, ok := byRole[role]; ok {
			return override
		}
		roles = append(roles, role)
		byRole[role] = &hive.ModelOverrideRequest{Role: role}
		return byRole[role]
	}
	markScalar := func(role, field string) error {
		if seenFields[role] == nil {
			seenFields[role] = map[string]struct{}{}
		}
		if _, exists := seenFields[role][field]; exists {
			return fmt.Errorf("--%s set more than once for role %q", field, role)
		}
		seenFields[role][field] = struct{}{}
		return nil
	}
	setScalar := func(flagName string, values repeatedStringFlag, set func(*hive.ModelOverrideRequest, string)) error {
		for _, raw := range values {
			role, value, err := parseFactoryOrderRoleValue(flagName, raw)
			if err != nil {
				return err
			}
			if err := markScalar(role, flagName); err != nil {
				return err
			}
			set(ensure(role), value)
		}
		return nil
	}
	if err := setScalar("model", flags.models, func(o *hive.ModelOverrideRequest, value string) { o.Model = value }); err != nil {
		return nil, err
	}
	if err := setScalar("provider", flags.providers, func(o *hive.ModelOverrideRequest, value string) { o.Provider = value }); err != nil {
		return nil, err
	}
	if err := setScalar("profile", flags.profiles, func(o *hive.ModelOverrideRequest, value string) { o.Profile = value }); err != nil {
		return nil, err
	}
	if err := setScalar("auth-mode", flags.authModes, func(o *hive.ModelOverrideRequest, value string) { o.AuthMode = value }); err != nil {
		return nil, err
	}
	if err := setScalar("preferred-tier", flags.preferredTiers, func(o *hive.ModelOverrideRequest, value string) { o.PreferredTier = value }); err != nil {
		return nil, err
	}
	for _, raw := range flags.requiredCapabilities {
		role, value, err := parseFactoryOrderRoleValue("required-capability", raw)
		if err != nil {
			return nil, err
		}
		caps := splitFactoryOrderCapabilities(value)
		if len(caps) == 0 {
			return nil, fmt.Errorf("--required-capability for role %q must include at least one capability", role)
		}
		override := ensure(role)
		override.RequiredCapabilities = append(override.RequiredCapabilities, caps...)
	}
	for _, raw := range flags.maxCosts {
		role, value, err := parseFactoryOrderRoleValue("max-cost-per-call-usd", raw)
		if err != nil {
			return nil, err
		}
		if err := markScalar(role, "max-cost-per-call-usd"); err != nil {
			return nil, err
		}
		amount, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("--max-cost-per-call-usd for role %q must be a number: %w", role, err)
		}
		override := ensure(role)
		override.MaxCostPerCallUSD = &amount
	}
	out := make([]hive.ModelOverrideRequest, 0, len(roles))
	for _, role := range roles {
		out = append(out, *byRole[role])
	}
	return out, nil
}

func parseFactoryOrderRoleValue(flagName, raw string) (string, string, error) {
	role, value, ok := strings.Cut(raw, "=")
	role = strings.TrimSpace(role)
	value = strings.TrimSpace(value)
	if !ok || role == "" || value == "" {
		return "", "", fmt.Errorf("--%s must use role=value", flagName)
	}
	return role, value, nil
}

func splitFactoryOrderCapabilities(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func validateFactoryOrderModelOverrides(catalogPath string, overrides []hive.ModelOverrideRequest) ([]hive.RunLaunchModelOverride, error) {
	if len(overrides) == 0 {
		return nil, nil
	}
	var source hive.OperatorModelSelectionSource
	if catalogPath != "" {
		config, err := hive.OperatorModelSelectionFromCatalogPath(catalogPath, time.Time{})
		if err != nil {
			return nil, fmt.Errorf("load catalog for model overrides: %w", err)
		}
		source = func() hive.OperatorModelSelectionConfig { return config }
	}
	validated, err := hive.ValidateModelOverrides(overrides, source)
	if err != nil {
		return nil, fmt.Errorf("validate model overrides: %w", err)
	}
	return validated, nil
}

func workFactoryOrderModelOverrides(overrides []hive.RunLaunchModelOverride) []work.FactoryOrderModelOverride {
	if len(overrides) == 0 {
		return nil
	}
	out := make([]work.FactoryOrderModelOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, work.FactoryOrderModelOverride{
			Role:                 override.Role,
			Model:                override.Model,
			Provider:             override.Provider,
			Profile:              override.Profile,
			RequestedAuthMode:    override.RequestedAuthMode,
			PreferredTier:        override.PreferredTier,
			RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
			MaxCostPerCallUSD:    cloneFloat64Ptr(override.MaxCostPerCallUSD),
			ResolvedModel:        override.ResolvedModel,
			ResolvedProvider:     override.ResolvedProvider,
			AuthMode:             override.AuthMode,
		})
	}
	return out
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// cmdFactoryRequestPR raises the draft-PR authority request carrying the target
// scope. All required flags are validated BEFORE any store/runtime/GitHub
// access. The gate HOLDS by design (AuthorityError) with --approve-requests
// off, so the request id is printed and the command exits 0.
func cmdFactoryRequestPR(args []string) error {
	fs := flag.NewFlagSet("factory request-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Guardian/operator name (required)")
	repo := fs.String("repo", "", "GitHub target Transpara-AI repo slug, e.g. transpara-ai/site (required)")
	baseRef := fs.String("base", "main", "Base branch ref")
	baseSHA := fs.String("base-sha", "", "Base commit SHA (required)")
	headRef := fs.String("head", "", "Head branch ref, e.g. codex/... (required)")
	headSHA := fs.String("head-sha", "", "Head commit SHA (required)")
	title := fs.String("title", "", "PR title (required)")
	bodyFile := fs.String("body-file", "", "Path to a file containing the PR body (required)")
	nonce := fs.String("nonce", "", "Single-use nonce (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{
		{"repo", *repo}, {"human", *human}, {"base-sha", *baseSHA}, {"head", *headRef},
		{"head-sha", *headSHA}, {"title", *title}, {"body-file", *bodyFile}, {"nonce", *nonce},
	}); err != nil {
		return err
	}
	body, err := os.ReadFile(*bodyFile)
	if err != nil {
		return fmt.Errorf("read body-file: %w", err)
	}

	// These scope hashes are informational for the Gate-E operator display. The
	// AUTHORITATIVE Epic 11 evidence hashes are recomputed from Title/Body by
	// BuildEpic11DocsDraftPROptions during `create-pr`; an exact-match here is
	// not required (work's epic7Hash helper is unexported, so we reproduce its
	// "sha256:"+hex(sha256(value)) format locally).
	repoSlug := strings.ToLower(strings.TrimSpace(*repo))
	policyBundleID := work.Epic11PolicyBundleID
	policyBundleHash := work.Epic11DocsDraftPRPolicyBundleHash()
	if repoSlug != "transpara-ai/docs" {
		policyBundleID = hive.TransparaAIDraftPRPolicyBundleID
		policyBundleHash = hive.TransparaAIDraftPRPolicyBundleHash()
	}

	target := hive.DraftPRTarget{
		Repository:       repoSlug,
		BaseRef:          *baseRef,
		BaseSHA:          *baseSHA,
		HeadRef:          *headRef,
		HeadSHA:          *headSHA,
		TitleHash:        sha256Hash(*title),
		BodyHash:         sha256Hash(string(body)),
		PolicyBundleID:   policyBundleID,
		PolicyBundleHash: policyBundleHash,
		SingleUseNonce:   *nonce,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, ".", "")
	if err != nil {
		return err
	}
	defer fc.close()

	requestID, err := rt.RaiseDraftPRAuthorityRequest(target, fc.humanID, "Guardian-initiated draft PR for "+*headRef)
	if err != nil {
		// The hold is the expected, successful outcome: an AuthorityError means
		// the request was recorded and awaits operator approval. Surface the id
		// and exit 0. Any non-authority error is a real failure.
		if isExpectedAuthorityHold(err) {
			fmt.Printf("draft-PR authority request raised (HELD pending approval): %s\n", requestID)
			return nil
		}
		return fmt.Errorf("raise draft-PR authority request: %w", err)
	}
	// --approve-requests was on: no hold. Still report the request id.
	fmt.Printf("draft-PR authority request raised (auto-approved): %s\n", requestID)
	return nil
}

// cmdFactoryCreatePR runs the gated, real-GitHub draft-PR creation from an
// approved decision. The target (repo, base/head refs+SHAs, policy bundle,
// nonce, and the approved title/body hashes) is the AUTHORITATIVE one loaded
// from the recorded authority decision for --request — NOT fresh CLI input. The
// only CLI inputs that flow into the PR are --title/--body-file (verified to
// hash-match the approved decision) and --changed-files (outside the authority
// chain). A request that is denied, never decided, or never raised has no
// approved decision and is refused before any GitHub access.
func cmdFactoryCreatePR(args []string) error {
	fs := flag.NewFlagSet("factory create-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	request := fs.String("request", "", "Approved authority request id (required)")
	title := fs.String("title", "", "PR title; must hash-match the approved decision (required)")
	bodyFile := fs.String("body-file", "", "Path to a file containing the PR body; must hash-match the approved decision (required)")
	changed := fs.String("changed-files", "", "Comma-separated changed file paths")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{
		{"request", *request}, {"human", *human}, {"title", *title}, {"body-file", *bodyFile},
	}); err != nil {
		return err
	}
	body, err := os.ReadFile(*bodyFile)
	if err != nil {
		return fmt.Errorf("read body-file: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fc, err := openFactoryContext(ctx, *storeDSN, *human)
	if err != nil {
		return err
	}
	defer fc.close()

	// GOVERNANCE gate: load the AUTHORITATIVE target from the recorded authority
	// decision. Refuses (no PR) if the request was denied, never decided, or
	// never raised. Then confirm the supplied title/body hash to exactly what the
	// human approved — a human can only authorize the content they reviewed.
	target, err := hive.LoadApprovedDraftPRTarget(fc.store, *request)
	if err != nil {
		return fmt.Errorf("create-pr refused for request %s: %w", *request, err)
	}
	if err := hive.VerifyDraftPRContent(target, *title, string(body)); err != nil {
		return fmt.Errorf("create-pr refused for request %s: %w", *request, err)
	}

	ts := work.NewTaskStore(fc.store, fc.factory, fc.signer)
	conv := factoryRequestConversation(*request)
	causes := fc.headCauses()

	art := hive.DraftPRArtifact{
		Target:         target,
		Title:          *title,
		Body:           string(body),
		ChangedFiles:   splitCSV(*changed),
		ActorRole:      "implementer",
		DeciderActorID: fc.humanID.Value(),
		DeciderRole:    "human",
	}

	// The real GitHub creator (W2). Token comes from the environment with no
	// fallback; an empty token will fail at the GitHub call, not here.
	client := work.NewEpic11GitHubPullRequestCreator(os.Getenv("GITHUB_TOKEN"))

	if strings.EqualFold(strings.TrimSpace(target.Repository), "transpara-ai/docs") && strings.TrimSpace(target.PolicyBundleID) == work.Epic11PolicyBundleID {
		run, err := hive.CreateDraftPRFromApprovedDecision(ctx, ts, fc.humanID, conv, client, art, causes...)
		if err != nil {
			return fmt.Errorf("create draft PR from approved decision %s: %w", *request, err)
		}
		fmt.Printf("created draft PR #%d for %s: %s\n", run.MutationResult.Number, run.MutationResult.Repository, run.MutationResult.URL)
		return nil
	}
	run, err := hive.CreateTransparaAIDraftPRFromApprovedDecision(ctx, ts, fc.humanID, conv, client, art, causes...)
	if err != nil {
		return fmt.Errorf("create Transpara-AI draft PR from approved decision %s: %w", *request, err)
	}
	fmt.Printf("created draft PR #%d for %s: %s\n", run.MutationResult.Number, run.MutationResult.Repository, run.MutationResult.URL)
	return nil
}

// ─── factory store/runtime context ──────────────────────────────────────────

// factoryContext bundles the store-layer handles the standalone factory
// subcommands need. It mirrors runLegacy's store-opening prologue so request-pr
// and create-pr open the SAME store the daemon uses (DATABASE_URL/Postgres or
// in-memory), with hive + work event types registered.
type factoryContext struct {
	pool    *pgxpool.Pool
	store   store.Store
	actors  actor.IActorStore
	factory *event.EventFactory
	signer  event.Signer
	humanID types.ActorID
}

// openFactoryContext opens the store, registers event types, registers the
// human actor, bootstraps the graph if empty, and derives the deterministic
// human signer. dsn falls back to DATABASE_URL; empty → in-memory.
func openFactoryContext(ctx context.Context, dsn, humanName string) (*factoryContext, error) {
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	var pool *pgxpool.Pool
	if dsn != "" {
		p, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("postgres: %w", err)
		}
		pool = p
	}

	// Register hive + work event types before opening the store so persisted
	// events (hive.*, work.*) deserialize on Head().
	hive.RegisterEventTypes()
	work.RegisterEventTypes()

	s, err := openStore(ctx, pool)
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		return nil, fmt.Errorf("store: %w", err)
	}

	actors, err := openActorStore(ctx, pool)
	if err != nil {
		s.Close()
		if pool != nil {
			pool.Close()
		}
		return nil, fmt.Errorf("actor store: %w", err)
	}

	humanID, err := registerHuman(actors, humanName)
	if err != nil {
		s.Close()
		if pool != nil {
			pool.Close()
		}
		return nil, fmt.Errorf("register human: %w", err)
	}

	if err := bootstrapGraph(s, humanID); err != nil {
		s.Close()
		if pool != nil {
			pool.Close()
		}
		return nil, fmt.Errorf("bootstrap graph: %w", err)
	}

	registry := event.DefaultRegistry()
	hive.RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)

	return &factoryContext{
		pool:    pool,
		store:   s,
		actors:  actors,
		factory: event.NewEventFactory(registry),
		signer:  &bootstrapSigner{humanID: humanID},
		humanID: humanID,
	}, nil
}

func (fc *factoryContext) close() {
	if fc == nil {
		return
	}
	if fc.store != nil {
		fc.store.Close()
	}
	if fc.pool != nil {
		fc.pool.Close()
	}
}

// headCauses returns the current chain head as the single causal parent, or nil
// when the store is empty. The eventgraph requires at least one cause for
// non-genesis events; bootstrapGraph guarantees a head exists.
func (fc *factoryContext) headCauses() []types.EventID {
	head, err := fc.store.Head()
	if err != nil || head.IsNone() {
		return nil
	}
	return []types.EventID{head.Unwrap().ID()}
}

// openFactoryRuntime builds a full hive.Runtime over the factory store context,
// for the request-pr authority seam. repoPath defaults to current dir.
func openFactoryRuntime(ctx context.Context, dsn, humanName, repoPath, repoWorkspaceRoot string) (*hive.Runtime, *factoryContext, error) {
	fc, err := openFactoryContext(ctx, dsn, humanName)
	if err != nil {
		return nil, nil, err
	}
	if repoPath == "" {
		repoPath = "."
	}
	rt, err := hive.New(ctx, hive.Config{
		Store:             fc.store,
		Actors:            fc.actors, // hive.New verifies the human and registers the system actor.
		HumanID:           fc.humanID,
		RepoPath:          repoPath,
		RepoWorkspaceRoot: repoWorkspaceRoot,
	})
	if err != nil {
		fc.close()
		return nil, nil, fmt.Errorf("runtime: %w", err)
	}
	return rt, fc, nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

// requiredFlag pairs a flag name with its parsed value for ordered validation.
type requiredFlag struct {
	name string
	val  string
}

// requireFlags returns an error naming the FIRST empty required flag, in the
// given order. Ordered (not map-based) so the error is deterministic.
func requireFlags(flags []requiredFlag) error {
	for _, f := range flags {
		if f.val == "" {
			return fmt.Errorf("--%s is required", f.name)
		}
	}
	return nil
}

// sha256Hash reproduces work.epic7Hash's "sha256:"+hex format (that helper is
// unexported). Used only for the informational scope hashes on the authority
// request; authoritative hashes are recomputed in create-pr.
func sha256Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// isExpectedAuthorityHold reports whether err is the benign "approval required"
// hold that RaiseDraftPRAuthorityRequest returns when --approve-requests is off.
func isExpectedAuthorityHold(err error) bool {
	if err == nil {
		return false
	}
	var authErr safety.AuthorityError
	if errors.As(err, &authErr) {
		return authErr.Outcome == safety.ApprovalRequired
	}
	return false
}

// specStem returns a filename-derived order suffix: base name without extension.
func specStem(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "" || base == "." {
		return "order"
	}
	return base
}

// parseOrderGateSections extracts the optional Definition of Done / Acceptance
// Criteria / Test Plan sections from an order spec's markdown. A section's body
// is the text under its "## <name>" heading up to the next heading, trimmed.
// Missing sections return "" so the planner attaches them later — work's
// Readiness gate enforces non-empty before the task becomes assignable.
func parseOrderGateSections(spec string) (definitionOfDone, acceptanceCriteria, testPlan string) {
	sections := map[string][]string{}
	current := ""
	for _, line := range strings.Split(spec, "\n") {
		if heading, ok := markdownHeading(line); ok {
			current = strings.ToLower(heading)
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}
	body := func(name string) string {
		return strings.TrimSpace(strings.Join(sections[name], "\n"))
	}
	return body("definition of done"), body("acceptance criteria"), body("test plan")
}

// markdownHeading reports whether line is an ATX heading (one or more leading
// '#') and returns the heading text with the '#'s and surrounding space removed.
func markdownHeading(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(trimmed, "#")), true
}

// factoryOrderConversation derives a stable, unique conversation ID for an
// order so each Order gets its own conversation (and thus its own work
// Workspace). A 16-byte hex digest of the order id keeps it deterministic.
func factoryOrderConversation(orderID string) types.ConversationID {
	sum := sha256.Sum256([]byte("order:" + orderID))
	return types.MustConversationID("conv_" + hex.EncodeToString(sum[:16]))
}

func factoryRequestConversation(requestID string) types.ConversationID {
	sum := sha256.Sum256([]byte("request:" + requestID))
	return types.MustConversationID("conv_" + hex.EncodeToString(sum[:16]))
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if seg := s[start:i]; seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	if seg := s[start:]; seg != "" {
		out = append(out, seg)
	}
	return out
}
