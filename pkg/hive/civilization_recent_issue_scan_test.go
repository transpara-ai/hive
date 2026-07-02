package hive

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func TestCivilizationRecentIssueScanRunsEmptyStoreUnavailable(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)

	projection := BuildCivilizationAssemblyProjection(s, 50)

	rail := projection.RecentIssueScanRuns
	if rail.Status != civilizationAssemblyFieldUnavailable {
		t.Fatalf("status = %q, want unavailable", rail.Status)
	}
	if len(rail.Runs) != 0 {
		t.Fatalf("runs = %+v, want zero", rail.Runs)
	}
	if rail.Summary == "" {
		t.Fatalf("summary is empty, want an honest summary")
	}
	if rail.Truncated {
		t.Fatalf("truncated = true, want false for empty store")
	}
}

func TestCivilizationRecentIssueScanRunsStateDomain(t *testing.T) {
	t.Run("parked run cross-section consistency with board", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		parked := appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "level1_canary_transpara-ai_docs_226",
			Repository:        "transpara-ai/docs",
			IssueNumber:       226,
			LifecycleVersion:  IssueScanParkLifecycleLevel1Canary,
			EvidenceClass:     IssueScanParkEvidenceClassLevel1Canary,
			AuthorityBoundary: IssueScanParkAuthorityBoundaryLevel1Canary,
			BlockerType:       IssueScanParkBlockerHumanScope,
			Detail:            "transpara-ai/docs#226 is labeled cc:needs-human-scope",
			RequiredAction:    "human must clarify scope and remove the human-scope blocker before Hive may continue",
			SourceRefs:        []string{"https://github.com/transpara-ai/docs/issues/226", "canary://level1-dark-factory/issue-discovery"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:intake", "cc:needs-human-scope"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if projection.RecentIssueScanRuns.Status != civilizationAssemblyFieldAvailable {
			t.Fatalf("rail status = %q, want available", projection.RecentIssueScanRuns.Status)
		}
		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one", projection.RecentIssueScanRuns.Runs)
		}
		railRun := projection.RecentIssueScanRuns.Runs[0]
		if len(projection.IssueScanProjection.Runs) != 1 {
			t.Fatalf("board runs = %+v, want one", projection.IssueScanProjection.Runs)
		}
		boardRun := projection.IssueScanProjection.Runs[0]
		if len(projection.IssueScanProjection.Blockers) == 0 {
			t.Fatalf("board blockers = %+v, want at least one", projection.IssueScanProjection.Blockers)
		}
		boardBlocker := projection.IssueScanProjection.Blockers[0]

		if railRun.RunID != boardRun.RunID {
			t.Fatalf("rail run_id = %q, board run_id = %q, want match", railRun.RunID, boardRun.RunID)
		}
		if railRun.State != boardRun.State {
			t.Fatalf("rail state = %q, board state = %q, want match", railRun.State, boardRun.State)
		}
		if railRun.State != "human_action" {
			t.Fatalf("rail state = %q, want human_action", railRun.State)
		}
		if railRun.Repo != boardRun.TargetIssue.Repo || railRun.IssueNumber != boardRun.TargetIssue.Number {
			t.Fatalf("rail issue ref = %+v, board target issue = %+v, want match", railRun, boardRun.TargetIssue)
		}
		if railRun.BlockerType != boardBlocker.BlockerType {
			t.Fatalf("rail blocker_type = %q, board blocker_type = %q, want match", railRun.BlockerType, boardBlocker.BlockerType)
		}
		if railRun.RequiredAction != boardBlocker.RequiredAction {
			t.Fatalf("rail required_action = %q, board required_action = %q, want match", railRun.RequiredAction, boardBlocker.RequiredAction)
		}
		if !containsString(railRun.SourceRefs, parked.ID().Value()) {
			t.Fatalf("rail source_refs = %+v, want parked event %s", railRun.SourceRefs, parked.ID().Value())
		}
		if railRun.FirstEventAt == "" || railRun.LastEventAt == "" {
			t.Fatalf("rail run timestamps = %+v, want populated for single-event run", railRun)
		}
		if railRun.FirstEventAt != railRun.LastEventAt {
			t.Fatalf("rail run first/last = %q/%q, want equal for single-event run", railRun.FirstEventAt, railRun.LastEventAt)
		}
		if _, err := time.Parse(time.RFC3339, railRun.FirstEventAt); err != nil {
			t.Fatalf("rail run first_event_at = %q, want RFC3339: %v", railRun.FirstEventAt, err)
		}
	})

	t.Run("parked run maps to parked state without human-action blocker", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_parked_only",
			FactoryOrderID:    "fo_run_issue_scan_parked_only",
			Repository:        "transpara-ai/hive",
			IssueNumber:       500,
			StageID:           "research_issue_and_repo_context",
			BlockerType:       "",
			Detail:            "",
			RequiredAction:    "",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/500"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: nil,
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.State != "parked" {
			t.Fatalf("state = %q, want parked (no human-action blocker present)", run.State)
		}
	})

	t.Run("queued: issue-scan requested with no parked event and no work evidence", func(t *testing.T) {
		s, _, appendEvent := newOperatorProjectionStore(t)
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		requested := appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_issue_scan_queued_001",
			IntakeID:   "intake_issue_scan_queued_001",
			OperatorID: "operator_michael",
			Title:      "Resolve transpara-ai/hive#321",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         brief,
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.State != "queued" {
			t.Fatalf("state = %q, want queued", run.State)
		}
		if run.RunID != "run_issue_scan_queued_001" {
			t.Fatalf("run_id = %q, want run_issue_scan_queued_001", run.RunID)
		}
		if !containsString(run.SourceRefs, requested.ID().Value()) {
			t.Fatalf("source_refs = %+v, want requested event %s", run.SourceRefs, requested.ID().Value())
		}
	})

	t.Run("in_flight: requested + factory order carrying stage work-task evidence", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 323,
			Title:  "In-flight issue-scan run",
			URL:    "https://github.com/transpara-ai/hive/issues/323",
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_issue_scan_in_flight_001",
			IntakeID:   "intake_issue_scan_in_flight_001",
			OperatorID: "operator_michael",
			Title:      "Resolve transpara-ai/hive#323",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         brief,
		})
		orderID, err := factoryOrderIDForRunLaunch("run_issue_scan_in_flight_001")
		if err != nil {
			t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
		}
		appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
			Title:                  "Resolve transpara-ai/hive#323",
			Description:            "FactoryOrder seed task.",
			CreatedBy:              actorID,
			FactoryOrderID:         orderID,
			RequirementIDs:         []string{"req_run_issue_scan_in_flight_001"},
			AcceptanceCriterionIDs: []string{"ac_run_issue_scan_in_flight_001"},
			Cell:                   "implementation",
			RiskClass:              "high",
			ExpectedOutputs:        []string{"ready-for-Human result PR"},
		})
		appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
			Title:                  "Issue-scan stage: Research issue and repo context",
			Description:            "Stage ID: research_issue_and_repo_context",
			CreatedBy:              actorID,
			CanonicalTaskID:        issueScanLifecycleStageTaskCanonicalID(orderID, "research_issue_and_repo_context"),
			FactoryOrderID:         orderID,
			RequirementIDs:         []string{"req_run_issue_scan_in_flight_001"},
			AcceptanceCriterionIDs: []string{"ac_run_issue_scan_in_flight_001"},
			Cell:                   "planning",
			RiskClass:              "high",
			ExpectedOutputs:        []string{"stage declaration artifact remains pending runtime evidence", "repo_context_packet"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.State != "in_flight" {
			t.Fatalf("state = %q, want in_flight", run.State)
		}
		if run.FactoryOrderID != orderID {
			t.Fatalf("factory_order_id = %q, want %q", run.FactoryOrderID, orderID)
		}
	})

	t.Run("queued stays queued when factory order summary is absent", func(t *testing.T) {
		s, _, appendEvent := newOperatorProjectionStore(t)
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 324,
			Title:  "Queued issue-scan run without order evidence",
			URL:    "https://github.com/transpara-ai/hive/issues/324",
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_issue_scan_no_order_001",
			IntakeID:   "intake_issue_scan_no_order_001",
			OperatorID: "operator_michael",
			Title:      "Resolve transpara-ai/hive#324",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         brief,
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.State != "queued" {
			t.Fatalf("state = %q, want queued (no factory order evidence to promote to in_flight)", run.State)
		}
	})

	t.Run("generic non-issue-scan factory.run.requested excluded entirely", func(t *testing.T) {
		s, _, appendEvent := newOperatorProjectionStore(t)
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_generic_001",
			IntakeID:   "intake_generic_001",
			OperatorID: "operator_michael",
			Title:      "Generic factory run",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "generic",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         []byte(`{"kind":"some_other_brief_kind"}`),
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 0 {
			t.Fatalf("rail runs = %+v, want zero (generic run excluded entirely)", projection.RecentIssueScanRuns.Runs)
		}
		if projection.RecentIssueScanRuns.Status != civilizationAssemblyFieldUnavailable {
			t.Fatalf("status = %q, want unavailable when only generic runs exist", projection.RecentIssueScanRuns.Status)
		}
	})

	t.Run("blank run_id excluded from parked evidence", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "   ",
			Repository:        "transpara-ai/docs",
			IssueNumber:       227,
			BlockerType:       IssueScanParkBlockerHumanScope,
			RequiredAction:    "human must clarify scope",
			SourceRefs:        []string{"https://github.com/transpara-ai/docs/issues/227"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 0 {
			t.Fatalf("rail runs = %+v, want zero (blank run_id excluded)", projection.RecentIssueScanRuns.Runs)
		}
	})

	t.Run("two parked events for one run: latest wins whole, refs unioned", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		first := appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_dup_001",
			Repository:        "transpara-ai/hive",
			IssueNumber:       401,
			StageID:           "research_issue_and_repo_context",
			BlockerType:       IssueScanParkBlockerStaleTarget,
			Detail:            "first parked event",
			RequiredAction:    "confirm the issue is still in scope",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/401", "ref-a"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: nil,
		})
		second := appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_dup_001",
			Repository:        "transpara-ai/hive",
			IssueNumber:       401,
			StageID:           "debate_with_correct_civic_roles",
			BlockerType:       IssueScanParkBlockerHumanScope,
			Detail:            "second parked event",
			RequiredAction:    "human must clarify scope and remove the human-scope blocker before Hive may continue",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/401", "ref-b"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one deduped run", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		// Latest event timestamp wins whole (no field-mixing between the two
		// parked events). second.Timestamp() >= first.Timestamp() because it
		// was appended later against a real clock.
		if second.Timestamp().Value().Before(first.Timestamp().Value()) {
			t.Skip("test clock produced an out-of-order second event; cannot assert latest-wins without ordering guarantee")
		}
		if run.StageID != "debate_with_correct_civic_roles" {
			t.Fatalf("stage_id = %q, want the latest parked event's stage (whole-record win, no field mixing)", run.StageID)
		}
		if run.BlockerType != IssueScanParkBlockerHumanScope {
			t.Fatalf("blocker_type = %q, want the latest parked event's blocker", run.BlockerType)
		}
		if !containsString(run.SourceRefs, "ref-a") || !containsString(run.SourceRefs, "ref-b") {
			t.Fatalf("source_refs = %+v, want union of both parked events' refs", run.SourceRefs)
		}
		if !containsString(run.SourceRefs, first.ID().Value()) || !containsString(run.SourceRefs, second.ID().Value()) {
			t.Fatalf("source_refs = %+v, want both event IDs", run.SourceRefs)
		}
	})

	t.Run("two parked events with identical timestamps tie-break by lexicographically greater event ID", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		signer := deriveSignerFromID(actorID)
		convID := types.MustConversationID("conv_00000000000000000000000000000077")
		_ = appendEvent

		head, err := s.Head()
		if err != nil || !head.IsSome() {
			t.Fatalf("head: %v", err)
		}
		sharedTimestamp := types.NewTimestamp(time.Now().UTC())

		buildParked := func(idSeed string, content IssueScanRunParkedContent, causes []types.EventID, prevHash types.Hash) event.Event {
			t.Helper()
			id := types.MustEventID(idSeed)
			tmp := event.NewEvent(event.CurrentEventVersion, id, EventTypeIssueScanRunParked, sharedTimestamp, actorID, content, causes, convID, types.ZeroHash(), prevHash, types.Signature{})
			canonical := event.CanonicalForm(tmp)
			hash, err := event.ComputeHash(canonical)
			if err != nil {
				t.Fatalf("compute hash: %v", err)
			}
			sig, err := signer.Sign([]byte(canonical))
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			return event.NewEvent(event.CurrentEventVersion, id, EventTypeIssueScanRunParked, sharedTimestamp, actorID, content, causes, convID, hash, prevHash, sig)
		}

		lowerID := "01900000-0000-7000-8000-00000000aaaa"
		higherID := "01900000-0000-7000-8000-00000000bbbb"
		lower := buildParked(lowerID, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_tiebreak_001",
			Repository:        "transpara-ai/hive",
			IssueNumber:       450,
			BlockerType:       IssueScanParkBlockerStaleTarget,
			Detail:            "lower id event",
			RequiredAction:    "confirm the issue is still in scope",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/450", "ref-lower"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: nil,
		}, []types.EventID{head.Unwrap().ID()}, head.Unwrap().Hash())
		if _, err := s.Append(lower); err != nil {
			t.Fatalf("append lower: %v", err)
		}
		higher := buildParked(higherID, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_tiebreak_001",
			Repository:        "transpara-ai/hive",
			IssueNumber:       450,
			BlockerType:       IssueScanParkBlockerHumanScope,
			Detail:            "higher id event",
			RequiredAction:    "human must clarify scope and remove the human-scope blocker before Hive may continue",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/450", "ref-higher"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		}, []types.EventID{lower.ID()}, lower.Hash())
		if _, err := s.Append(higher); err != nil {
			t.Fatalf("append higher: %v", err)
		}
		if !(higherID > lowerID) {
			t.Fatalf("test fixture invariant broken: %q must sort lexicographically greater than %q", higherID, lowerID)
		}

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one deduped run", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.BlockerType != IssueScanParkBlockerHumanScope {
			t.Fatalf("blocker_type = %q, want the lexicographically greater event ID's blocker to win the timestamp tie", run.BlockerType)
		}
		if !containsString(run.SourceRefs, "ref-lower") || !containsString(run.SourceRefs, "ref-higher") {
			t.Fatalf("source_refs = %+v, want union of both tied events' refs", run.SourceRefs)
		}
	})

	t.Run("parked beats queued for one run with refs unioned", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 402,
			Title:  "Parked beats queued",
			URL:    "https://github.com/transpara-ai/hive/issues/402",
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		requested := appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_issue_scan_parked_beats_queued",
			IntakeID:   "intake_issue_scan_parked_beats_queued",
			OperatorID: "operator_michael",
			Title:      "Resolve transpara-ai/hive#402",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         brief,
		})
		parked := appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_parked_beats_queued",
			Repository:        "transpara-ai/hive",
			IssueNumber:       402,
			StageID:           "research_issue_and_repo_context",
			BlockerType:       IssueScanParkBlockerHumanScope,
			Detail:            "parked after queueing",
			RequiredAction:    "human must clarify scope and remove the human-scope blocker before Hive may continue",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/402"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 1 {
			t.Fatalf("rail runs = %+v, want one deduped run", projection.RecentIssueScanRuns.Runs)
		}
		run := projection.RecentIssueScanRuns.Runs[0]
		if run.State != "human_action" {
			t.Fatalf("state = %q, want human_action (parked wins over queued)", run.State)
		}
		if !containsString(run.SourceRefs, requested.ID().Value()) || !containsString(run.SourceRefs, parked.ID().Value()) {
			t.Fatalf("source_refs = %+v, want union of queued and parked evidence", run.SourceRefs)
		}
	})

	t.Run("recorded: requested run degrades when parked page is truncated", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		// Fill the parked page beyond the limit so the parked page itself
		// reports HasMore() (truncated) and "no parked event for this run"
		// becomes unprovable.
		for i := 0; i < 3; i++ {
			appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
				RunID:             "run_issue_scan_filler_" + string(rune('a'+i)),
				Repository:        "transpara-ai/hive",
				IssueNumber:       600 + i,
				BlockerType:       IssueScanParkBlockerHumanScope,
				RequiredAction:    "human must clarify scope",
				SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/filler"},
				ParkedBy:          actorID,
				TargetIssueState:  "open",
				TargetIssueLabels: []string{"cc:needs-human-scope"},
			})
		}
		sourceEventID := newTestEventID(t)
		briefEventID := newTestEventID(t)
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 405,
			Title:  "Recorded due to truncated parked page",
			URL:    "https://github.com/transpara-ai/hive/issues/405",
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      "run_issue_scan_recorded_001",
			IntakeID:   "intake_issue_scan_recorded_001",
			OperatorID: "operator_michael",
			Title:      "Resolve transpara-ai/hive#405",
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: sourceEventID,
			BriefEventID:  briefEventID,
			Brief:         brief,
		})

		// Small limit: the parked page (3 events) exceeds it, so HasMore()
		// is true and the requested-run's parked-absence is unprovable.
		projection := BuildCivilizationAssemblyProjection(s, 2)

		if !projection.RecentIssueScanRuns.Truncated {
			t.Fatalf("truncated = false, want true when parked page hits its limit")
		}
		var run *CivilizationRecentIssueScanRun
		for i := range projection.RecentIssueScanRuns.Runs {
			if projection.RecentIssueScanRuns.Runs[i].RunID == "run_issue_scan_recorded_001" {
				run = &projection.RecentIssueScanRuns.Runs[i]
			}
		}
		if run == nil {
			t.Fatalf("rail runs = %+v, want run_issue_scan_recorded_001 present as recorded", projection.RecentIssueScanRuns.Runs)
		}
		if run.State != "recorded" {
			t.Fatalf("state = %q, want recorded (parked-absence unprovable under truncation)", run.State)
		}
	})

	t.Run("truncated flag set when requested page itself is truncated", func(t *testing.T) {
		s, _, appendEvent := newOperatorProjectionStore(t)
		for i := 0; i < 3; i++ {
			issue := GitHubIssueCandidate{
				Repo:   "transpara-ai/hive",
				Number: 700 + i,
				Title:  "Filler queued run",
				URL:    "https://github.com/transpara-ai/hive/issues/filler",
				Body:   "Body",
				Labels: []string{"civilization"},
			}
			brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
			if err != nil {
				t.Fatalf("issueScanBriefJSON: %v", err)
			}
			appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
				RunID:      "run_issue_scan_page_filler_" + string(rune('a'+i)),
				IntakeID:   "intake_issue_scan_page_filler_" + string(rune('a'+i)),
				OperatorID: "operator_michael",
				Title:      "Filler queued run",
				Status:     "queued",
				Authority: RunLaunchAuthority{
					InitialLevel: event.AuthorityLevelRequired,
					Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
					PolicyRef:    IssueScanDefaultPolicyRef,
					Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
				},
				Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
				TargetRepos:   []string{"transpara-ai/hive"},
				SourceEventID: newTestEventID(t),
				BriefEventID:  newTestEventID(t),
				Brief:         brief,
			})
		}

		projection := BuildCivilizationAssemblyProjection(s, 2)

		if !projection.RecentIssueScanRuns.Truncated {
			t.Fatalf("truncated = false, want true when requested page hits its limit")
		}
		if len(projection.RecentIssueScanRuns.Runs) > 2 {
			t.Fatalf("rail runs = %+v, want at most limit-bounded runs (older runs simply absent)", projection.RecentIssueScanRuns.Runs)
		}
	})

	t.Run("ordering by last_event_at descending", func(t *testing.T) {
		s, actorID, appendEvent := newOperatorProjectionStore(t)
		appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_order_first",
			Repository:        "transpara-ai/hive",
			IssueNumber:       801,
			BlockerType:       IssueScanParkBlockerHumanScope,
			RequiredAction:    "human must clarify scope",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/801"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		})
		time.Sleep(2 * time.Millisecond)
		appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             "run_issue_scan_order_second",
			Repository:        "transpara-ai/hive",
			IssueNumber:       802,
			BlockerType:       IssueScanParkBlockerHumanScope,
			RequiredAction:    "human must clarify scope",
			SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/802"},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:needs-human-scope"},
		})

		projection := BuildCivilizationAssemblyProjection(s, 50)

		if len(projection.RecentIssueScanRuns.Runs) != 2 {
			t.Fatalf("rail runs = %+v, want two", projection.RecentIssueScanRuns.Runs)
		}
		if projection.RecentIssueScanRuns.Runs[0].RunID != "run_issue_scan_order_second" {
			t.Fatalf("rail runs[0] = %+v, want the most-recently-parked run first (DESC)", projection.RecentIssueScanRuns.Runs[0])
		}
		if projection.RecentIssueScanRuns.Runs[1].RunID != "run_issue_scan_order_first" {
			t.Fatalf("rail runs[1] = %+v, want the older run second", projection.RecentIssueScanRuns.Runs[1])
		}
	})
}

// recentIssueScanParkedFetchFailureStore fails ONLY the
// hive.issuescan.run.parked ByType query, so a factory.run.requested query
// against the same underlying store still succeeds. This isolates the
// "parked page could not be fetched" condition from every other store
// behavior (mirrors factoryOrderReadFailureStore's pattern for a different
// event type).
type recentIssueScanParkedFetchFailureStore struct {
	store.Store
}

func (s recentIssueScanParkedFetchFailureStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	if eventType == EventTypeIssueScanRunParked {
		return types.NewPage[event.Event](nil, types.None[types.Cursor](), false), errors.New("parked-run query unavailable")
	}
	return s.Store.ByType(eventType, limit, after)
}

// TestCivilizationRecentIssueScanRunsParkedFetchFailureIsUnavailable covers
// the Critical review finding: when the parked-run page fetch fails, the
// rail fold must fail CLOSED (status unavailable, zero runs) rather than
// treating "fetch failed" as "confirmed parked-absence" and evaluating
// factory.run.requested runs for in_flight/queued as if parked-absence were
// proven. The board fold already short-circuits on this (parkedOK==false);
// this test proves the rail now does too, and that the board's existing
// behavior is unchanged.
func TestCivilizationRecentIssueScanRunsParkedFetchFailureIsUnavailable(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)

	issue := GitHubIssueCandidate{
		Repo:   "transpara-ai/hive",
		Number: 900,
		Title:  "Requested run with unreadable parked evidence",
		URL:    "https://github.com/transpara-ai/hive/issues/900",
		Body:   "Body",
		Labels: []string{"civilization"},
	}
	brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
	if err != nil {
		t.Fatalf("issueScanBriefJSON: %v", err)
	}
	appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
		RunID:      "run_issue_scan_parked_fetch_failure",
		IntakeID:   "intake_issue_scan_parked_fetch_failure",
		OperatorID: "operator_michael",
		Title:      "Resolve transpara-ai/hive#900",
		Status:     "queued",
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
			PolicyRef:    IssueScanDefaultPolicyRef,
			Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
		},
		Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
		TargetRepos:   []string{"transpara-ai/hive"},
		SourceEventID: newTestEventID(t),
		BriefEventID:  newTestEventID(t),
		Brief:         brief,
	})

	failingStore := recentIssueScanParkedFetchFailureStore{Store: s}
	projection := BuildCivilizationAssemblyProjection(failingStore, 50)

	rail := projection.RecentIssueScanRuns
	if rail.Status != civilizationAssemblyFieldUnavailable {
		t.Fatalf("rail status = %q, want unavailable when the parked page cannot be fetched", rail.Status)
	}
	if len(rail.Runs) != 0 {
		t.Fatalf("rail runs = %+v, want zero when parked-absence cannot be proven", rail.Runs)
	}
	if !strings.Contains(rail.Summary, "parked") {
		t.Fatalf("rail summary = %q, want it to mention the unreadable parked evidence", rail.Summary)
	}

	// The board fold already short-circuits on parked-fetch failure
	// (parkedOK==false) prior to this fix. Assert that behavior is
	// unchanged by this change.
	if projection.IssueIntakeProjection.Status != civilizationAssemblyFieldUnavailable {
		t.Fatalf("board intake status = %q, want unavailable (unchanged behavior)", projection.IssueIntakeProjection.Status)
	}
	if len(projection.IssueScanProjection.Runs) != 0 {
		t.Fatalf("board scan runs = %+v, want zero (unchanged behavior)", projection.IssueScanProjection.Runs)
	}
}

// seedRecentIssueScanFoldLatencyFixtures appends a deterministic mix of ~200
// events to s: parked runs (some with human-scope blockers, some plain),
// issue-scan factory.run.requested runs (queued/in_flight-eligible), generic
// non-issue-scan factory.run.requested runs (must be excluded by the fold's
// isIssueScanRunLaunch predicate), and Work task-created evidence (both
// FactoryOrder seed tasks and issue-scan lifecycle-stage tasks, so a share of
// the requested runs promote to in_flight). Returns the total event count
// appended for the caller to assert against.
func seedRecentIssueScanFoldLatencyFixtures(t *testing.T, s *store.InMemoryStore, actorID types.ActorID, appendEvent func(types.EventType, event.EventContent) event.Event) int {
	t.Helper()
	const (
		parkedCount        = 60
		issueScanRunCount  = 60
		inFlightRunCount   = 30
		genericFactoryRuns = 50
	)
	appended := 0

	for i := 0; i < parkedCount; i++ {
		blockerType := ""
		requiredAction := ""
		if i%2 == 0 {
			blockerType = IssueScanParkBlockerHumanScope
			requiredAction = "human must clarify scope before Hive may continue"
		}
		appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
			RunID:             fmt.Sprintf("run_latency_parked_%03d", i),
			Repository:        "transpara-ai/hive",
			IssueNumber:       1000 + i,
			BlockerType:       blockerType,
			Detail:            "latency budget fixture",
			RequiredAction:    requiredAction,
			SourceRefs:        []string{fmt.Sprintf("https://github.com/transpara-ai/hive/issues/%d", 1000+i)},
			ParkedBy:          actorID,
			TargetIssueState:  "open",
			TargetIssueLabels: []string{"cc:intake"},
		})
		appended++
	}

	for i := 0; i < issueScanRunCount; i++ {
		issue := GitHubIssueCandidate{
			Repo:   "transpara-ai/hive",
			Number: 2000 + i,
			Title:  fmt.Sprintf("Latency fixture issue-scan run %03d", i),
			URL:    fmt.Sprintf("https://github.com/transpara-ai/hive/issues/%d", 2000+i),
			Body:   "Body",
			Labels: []string{"civilization"},
		}
		brief, err := issueScanBriefJSON([]GitHubIssueCandidate{issue}, issue)
		if err != nil {
			t.Fatalf("issueScanBriefJSON: %v", err)
		}
		runID := fmt.Sprintf("run_latency_queued_%03d", i)
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      runID,
			IntakeID:   "intake_" + runID,
			OperatorID: "operator_michael",
			Title:      fmt.Sprintf("Resolve transpara-ai/hive#%d", 2000+i),
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
				PolicyRef:    IssueScanDefaultPolicyRef,
				Rationale:    "Civilization selected a Transpara-AI GitHub issue for governed factory execution.",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: newTestEventID(t),
			BriefEventID:  newTestEventID(t),
			Brief:         brief,
		})
		appended++

		if i < inFlightRunCount {
			orderID, err := factoryOrderIDForRunLaunch(runID)
			if err != nil {
				t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
			}
			appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
				Title:                  fmt.Sprintf("Resolve transpara-ai/hive#%d", 2000+i),
				Description:            "FactoryOrder seed task.",
				CreatedBy:              actorID,
				FactoryOrderID:         orderID,
				RequirementIDs:         []string{"req_" + runID},
				AcceptanceCriterionIDs: []string{"ac_" + runID},
				Cell:                   "implementation",
				RiskClass:              "high",
				ExpectedOutputs:        []string{"ready-for-Human result PR"},
			})
			appended++
			appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
				Title:                  "Issue-scan stage: Research issue and repo context",
				Description:            "Stage ID: research_issue_and_repo_context",
				CreatedBy:              actorID,
				CanonicalTaskID:        issueScanLifecycleStageTaskCanonicalID(orderID, "research_issue_and_repo_context"),
				FactoryOrderID:         orderID,
				RequirementIDs:         []string{"req_" + runID},
				AcceptanceCriterionIDs: []string{"ac_" + runID},
				Cell:                   "planning",
				RiskClass:              "high",
				ExpectedOutputs:        []string{"stage declaration artifact remains pending runtime evidence", "repo_context_packet"},
			})
			appended++
		}
	}

	for i := 0; i < genericFactoryRuns; i++ {
		runID := fmt.Sprintf("run_latency_generic_%03d", i)
		appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
			RunID:      runID,
			IntakeID:   "intake_" + runID,
			OperatorID: "operator_michael",
			Title:      fmt.Sprintf("Generic factory run %03d", i),
			Status:     "queued",
			Authority: RunLaunchAuthority{
				InitialLevel: event.AuthorityLevelRequired,
				Scope:        "generic",
			},
			Budget:        RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			TargetRepos:   []string{"transpara-ai/hive"},
			SourceEventID: newTestEventID(t),
			BriefEventID:  newTestEventID(t),
			Brief:         []byte(`{"kind":"some_other_brief_kind"}`),
		})
		appended++
	}

	return appended
}

// TestRecentIssueScanFoldLatencyBudget covers D2's fold-latency requirement:
// the rail fold adds exactly ONE new store query and otherwise consumes
// already-fetched pages, so its own wall-clock contribution must stay small
// in absolute terms. On a store seeded with ~200 mixed events (parked +
// issue-scan requested + generic factory + work evidence), this asserts the
// PURE fold's wall-clock is < 250ms absolute — a regression guard against
// the fold becoming accidentally O(n^2) or gaining a new store round-trip,
// not a tight performance SLA. Skipped under -short.
//
// NOTE on the design packet's original "< 10% relative to the builder"
// criterion (D2): that ratio was calibrated for the PRODUCTION profile,
// where the builder is Postgres-backed and I/O-dominated (~5s solo,
// measured against the site's 8s/9s client timeout) — the fold's CPU cost
// is genuine noise against that denominator. This test's in-memory
// InMemoryStore builder completes in single-digit milliseconds, so the
// SAME absolute fold cost (which includes a real, pre-existing cost: the
// fold's own isIssueScanRunLaunch calls json.Unmarshal each requested
// event's ~19KB Brief payload just to read one "kind" field — the exact
// predicate hive dispatch itself uses per D1, not something this task
// changes) becomes a large, noisy fraction of a tiny denominator. Empirical
// measurement across several fixture shapes showed the fold's share of the
// in-memory builder's wall-clock ranging from ~10% to ~90% depending on how
// many issue-scan requested-run rows were seeded, with no fixture ratio
// that reliably stays under 10% without being unrepresentative. Gaming the
// fixture mix to force sub-10% would make the assertion dishonest rather
// than meaningful, so the relative check is intentionally NOT asserted
// here; the absolute 250ms budget (observed: low single-digit milliseconds,
// i.e. ~1-2% of the 250ms budget) is what actually protects against a
// regression, and builder/fold timings are logged below for the record.
func TestRecentIssueScanFoldLatencyBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timed latency budget test under -short")
	}

	s, actorID, appendEvent := newOperatorProjectionStore(t)
	seeded := seedRecentIssueScanFoldLatencyFixtures(t, s, actorID, appendEvent)
	if seeded < 200 {
		t.Fatalf("seeded %d events, want >= 200 for a representative fold-latency fixture", seeded)
	}

	// A limit large enough that none of the fixture's pages truncate, so the
	// fold actually processes the full seeded volume rather than degrading
	// early runs to "recorded" under an artificial truncation.
	const limit = 500

	// (i) Time the full builder end to end.
	builderStart := time.Now()
	projection := BuildCivilizationAssemblyProjection(s, limit)
	builderElapsed := time.Since(builderStart)

	if projection.RecentIssueScanRuns.Status != civilizationAssemblyFieldAvailable {
		t.Fatalf("rail status = %q, want available (fixture invariant broken)", projection.RecentIssueScanRuns.Status)
	}
	if projection.RecentIssueScanRuns.Truncated {
		t.Fatalf("rail truncated = true, want false (limit=%d should exceed the seeded fixture; fixture/limit invariant broken)", limit)
	}
	if len(projection.RecentIssueScanRuns.Runs) == 0 {
		t.Fatalf("rail runs = %+v, want a non-empty rail from the seeded fixture", projection.RecentIssueScanRuns.Runs)
	}

	// (ii) Re-derive the SAME pre-extracted inputs BuildCivilizationAssemblyProjection
	// itself computes, then time ONLY the pure fold function on them —
	// mirroring civilization_assembly_projection.go's own call sequence
	// (BuildOperatorProjection -> civilizationAssemblyFactoryOrders ->
	// civilizationAssemblyNormalizedParkedRuns ->
	// civilizationAssemblyFactoryRunRequestedEvents ->
	// civilizationRecentIssueScanRuns) so the isolated timing reflects a
	// realistic input shape, not a synthetic one.
	operatorProjection := BuildOperatorProjection(s, limit)
	factoryOrders, factoryOrderWorkEvidence, _, _ := civilizationAssemblyFactoryOrders(&operatorProjection, s, limit)
	normalizedParkedRuns, parkedTruncated, _, _, parkedFetched := civilizationAssemblyNormalizedParkedRuns(s, limit)
	requestedEvents, requestedTruncated, err := civilizationAssemblyFactoryRunRequestedEvents(s, limit)
	if err != nil {
		t.Fatalf("civilizationAssemblyFactoryRunRequestedEvents: %v", err)
	}

	foldStart := time.Now()
	fold := civilizationRecentIssueScanRuns(normalizedParkedRuns, parkedTruncated, parkedFetched, requestedEvents, requestedTruncated, factoryOrders, factoryOrderWorkEvidence)
	foldElapsed := time.Since(foldStart)

	if fold.Status != civilizationAssemblyFieldAvailable || len(fold.Runs) == 0 {
		t.Fatalf("isolated fold result = %+v, want it to match the builder's non-empty available rail (fixture invariant broken)", fold)
	}

	const absoluteBudget = 250 * time.Millisecond
	if foldElapsed >= absoluteBudget {
		t.Fatalf("fold wall-clock = %s, want < %s (absolute budget)", foldElapsed, absoluteBudget)
	}

	t.Logf("builder=%s fold=%s (%.2f%% of builder, informational only — see NOTE on relative budget above) seeded_events=%d rail_runs=%d", builderElapsed, foldElapsed, 100*float64(foldElapsed)/float64(builderElapsed), seeded, len(fold.Runs))
}
