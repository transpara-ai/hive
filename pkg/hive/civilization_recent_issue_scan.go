package hive

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// CivilizationRecentIssueScanRuns is always populated (no omitempty on the
// projection field) so consumers can rely on status even when the store is
// empty (status becomes "unavailable" with zero runs and an honest summary).
type CivilizationRecentIssueScanRuns struct {
	Status    string                           `json:"status"` // "available" | "unavailable"
	Summary   string                           `json:"summary,omitempty"`
	Truncated bool                             `json:"truncated,omitempty"`
	Runs      []CivilizationRecentIssueScanRun `json:"runs,omitempty"`
}

// CivilizationRecentIssueScanRun is one deduped, state-proven issue-scan run
// row. State is bound to explicit event evidence per the D1 allowlist
// (parked, human_action, queued, in_flight, recorded) — never a default that
// assigns a healthy state.
type CivilizationRecentIssueScanRun struct {
	RunID          string   `json:"run_id"`
	FactoryOrderID string   `json:"factory_order_id,omitempty"`
	Repo           string   `json:"repo"`
	IssueNumber    int      `json:"issue_number"`
	IssueURL       string   `json:"issue_url,omitempty"`
	IssueTitle     string   `json:"issue_title,omitempty"`
	State          string   `json:"state"`                    // parked|human_action|queued|in_flight|recorded
	FirstEventAt   string   `json:"first_event_at,omitempty"` // RFC3339 UTC; omitted when unproven
	LastEventAt    string   `json:"last_event_at,omitempty"`
	BlockerType    string   `json:"blocker_type,omitempty"`
	RequiredAction string   `json:"required_action,omitempty"`
	StageID        string   `json:"stage_id,omitempty"`
	SourceRefs     []string `json:"source_refs,omitempty"`
}

const (
	civilizationRecentIssueScanStateParked      = "parked"
	civilizationRecentIssueScanStateHumanAction = "human_action"
	civilizationRecentIssueScanStateQueued      = "queued"
	civilizationRecentIssueScanStateInFlight    = "in_flight"
	civilizationRecentIssueScanStateRecorded    = "recorded"
)

// civilizationAssemblyNormalizedParkedRun is the SHARED per-event
// normalization of one hive.issuescan.run.parked event, feeding BOTH the
// board fold (civilizationAssemblyIssueScanProjections) and the recent-runs
// rail fold (civilizationRecentIssueScanRuns). Extracting this once
// guarantees the two sections can never diverge on state, blocker fields, or
// evidence for the same run.
type civilizationAssemblyNormalizedParkedRun struct {
	RunID              string
	FactoryOrderID     string
	StageID            string
	Issue              CivilizationIssueRef
	LifecycleVersion   string
	AuthorityBoundary  string
	PrimaryBlockerType string
	RiskClass          string
	Readiness          string
	RiskClasses        []string
	ReadinessStates    []string
	Blockers           []CivilizationIssueScanBlockerProjected
	State              string // "parked" | "human_action"
	EventID            string
	EventTimestamp     time.Time
	Refs               []string
	// RawRequiredAction is the parked event content's top-level
	// RequiredAction field, unprocessed by blocker derivation. The board
	// fold's intake row PRReadyWhen field has always used this raw value
	// (not a blocker-list lookup), so it is preserved verbatim here to keep
	// the refactored board fold byte-identical.
	RawRequiredAction string
}

// civilizationRecentIssueScanNormalizeParkedEvent normalizes one parked event
// into the shared record. It returns ok=false for events that cannot be
// identified as belonging to a specific issue (mirrors the board fold's own
// admission rule: repo and a positive issue number must be present).
func civilizationRecentIssueScanNormalizeParkedEvent(ev event.Event, content IssueScanRunParkedContent) (civilizationAssemblyNormalizedParkedRun, bool) {
	issue := civilizationAssemblyIssueRefFromParked(content)
	if issue.Repo == "" || issue.Number <= 0 {
		return civilizationAssemblyNormalizedParkedRun{}, false
	}
	refs := compactStrings(append([]string{ev.ID().Value()}, content.SourceRefs...))
	primaryBlockerType := civilizationAssemblyIssuePrimaryBlockerType(content)
	blockers := civilizationAssemblyIssueScanBlockersFromParked(content, ev.ID().Value(), refs)
	state := civilizationRecentIssueScanStateParked
	if civilizationAssemblyIssueHasHumanActionBlocker(blockers) {
		state = civilizationRecentIssueScanStateHumanAction
	}
	return civilizationAssemblyNormalizedParkedRun{
		RunID:              strings.TrimSpace(content.RunID),
		FactoryOrderID:     strings.TrimSpace(content.FactoryOrderID),
		StageID:            strings.TrimSpace(content.StageID),
		Issue:              issue,
		LifecycleVersion:   civilizationAssemblyIssueScanLifecycleVersion(content),
		AuthorityBoundary:  civilizationAssemblyIssueScanAuthorityBoundary(content),
		PrimaryBlockerType: primaryBlockerType,
		RiskClass:          civilizationAssemblyIssueRiskClass(primaryBlockerType),
		Readiness:          civilizationAssemblyIssueReadiness(primaryBlockerType),
		RiskClasses:        civilizationAssemblyIssueRiskClasses(content),
		ReadinessStates:    civilizationAssemblyIssueReadinessStates(content),
		Blockers:           blockers,
		State:              state,
		EventID:            ev.ID().Value(),
		EventTimestamp:     ev.Timestamp().Value(),
		Refs:               refs,
		RawRequiredAction:  content.RequiredAction,
	}, true
}

// civilizationAssemblyNormalizedParkedRuns performs the ONE
// hive.issuescan.run.parked store fetch shared by both the board fold and
// the recent-runs rail fold. It returns normalized runs plus the page's
// truncation flag; ok=false means the caller should return the returned
// evidence as-is (I/O error, nil store, or empty page).
func civilizationAssemblyNormalizedParkedRuns(s store.Store, limit int) ([]civilizationAssemblyNormalizedParkedRun, bool, civilizationAssemblyIssueScanProjectionEvidence, bool) {
	intake := CivilizationIssueIntakeProjection{
		Status:  civilizationAssemblyFieldUnavailable,
		Summary: "No scanner-visible issue intake records are projected.",
		ScannerBoundaries: []string{
			"read-only GitHub issue discovery",
			"human-scope/protected/deferred issues park without execution",
			"projection is not PR readiness, runtime readiness, issue closure, merge, deploy, or Test 001 GREEN evidence",
			"updated_at is the EventGraph parked-evidence timestamp, not the GitHub issue last-modified time",
			"touched_substrate is derived only from the repository slug; no code or substrate analysis is performed",
			"issue intake rows are point-in-time parked snapshots and may be superseded by later FactoryOrder lifecycle evidence",
		},
	}
	scan := CivilizationIssueScanProjection{}
	if s == nil {
		return nil, false, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Errors: []string{"project issue-scan records: store is required"}}, false
	}
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	page, err := s.ByType(EventTypeIssueScanRunParked, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Errors: []string{"project issue-scan parked records: " + err.Error()}}, false
	}
	events := page.Items()
	truncated := page.HasMore()
	if len(events) == 0 {
		return nil, truncated, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Truncated: truncated}, false
	}
	normalized := make([]civilizationAssemblyNormalizedParkedRun, 0, len(events))
	for _, ev := range events {
		content, ok := ev.Content().(IssueScanRunParkedContent)
		if !ok {
			continue
		}
		run, ok := civilizationRecentIssueScanNormalizeParkedEvent(ev, content)
		if !ok {
			continue
		}
		normalized = append(normalized, run)
	}
	return normalized, truncated, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Truncated: truncated}, true
}

// civilizationAssemblyFactoryRunRequestedEvents is the ONE new store query
// authorized by the design packet (D1/CFADA1-1): a factory.run.requested
// page mirroring the existing truncation-detection pattern. Filtering by the
// issue-scan predicate happens in the pure fold, not here, so this stays a
// thin I/O boundary.
func civilizationAssemblyFactoryRunRequestedEvents(s store.Store, limit int) ([]event.Event, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("store is required")
	}
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	page, err := s.ByType(EventTypeFactoryRunRequested, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, err
	}
	return page.Items(), page.HasMore(), nil
}

// civilizationRecentIssueScanSourceRefs collects the rail's per-run source
// refs for the projection-wide SourceEventIDsOrQueryWindow/ProvenanceRefs
// aggregation.
func civilizationRecentIssueScanSourceRefs(rail CivilizationRecentIssueScanRuns) []string {
	refs := make([]string, 0, len(rail.Runs)*2)
	for _, run := range rail.Runs {
		refs = append(refs, run.SourceRefs...)
	}
	return refs
}

// civilizationRecentIssueScanBlockerForType returns the blocker record
// matching blockerType from a normalized parked run's blocker list, so the
// rail's singular BlockerType/RequiredAction fields stay identical to the
// board's per-blocker card for the same run.
func civilizationRecentIssueScanBlockerForType(blockers []CivilizationIssueScanBlockerProjected, blockerType string) (CivilizationIssueScanBlockerProjected, bool) {
	blockerType = strings.TrimSpace(blockerType)
	for _, blocker := range blockers {
		if strings.TrimSpace(blocker.BlockerType) == blockerType {
			return blocker, true
		}
	}
	return CivilizationIssueScanBlockerProjected{}, false
}

// civilizationRecentIssueScanRuns is the PURE fold deriving the
// recent_issue_scan_runs rail per design packet D1-D3. It performs no I/O —
// all store reads happen in the builder, which fetches the parked page ONCE
// and passes the normalized records here alongside the (also already
// fetched) factory-order/work-evidence outputs.
//
// Precedence: parked/human_action > in_flight > queued > recorded.
// Dedupe by run_id: blank run_id excluded; multiple parked events for one
// run collapse to the latest-timestamp event (whole record, no field
// mixing), tie-broken by lexicographically greater event ID, refs unioned.
func civilizationRecentIssueScanRuns(
	parkedRuns []civilizationAssemblyNormalizedParkedRun,
	parkedTruncated bool,
	requestedEvents []event.Event,
	requestedTruncated bool,
	factoryOrders []CivilizationAssemblyFactoryOrder,
	factoryOrderWorkEvidence civilizationAssemblyFactoryOrderWorkEvidence,
) CivilizationRecentIssueScanRuns {
	out := CivilizationRecentIssueScanRuns{
		Status:  civilizationAssemblyFieldUnavailable,
		Summary: "No recent issue-scan run activity is projected.",
	}

	parkedByRun := civilizationRecentIssueScanDedupeParked(parkedRuns)

	factoryOrderByID := make(map[string]bool, len(factoryOrders))
	for _, order := range factoryOrders {
		id := strings.TrimSpace(order.ID)
		if id != "" {
			factoryOrderByID[id] = true
		}
	}
	stageTaskFactoryOrders := civilizationRecentIssueScanFactoryOrdersWithStageEvidence(factoryOrderWorkEvidence)

	runs := make([]civilizationRecentIssueScanSortableRun, 0, len(parkedByRun)+len(requestedEvents))
	seenRun := map[string]bool{}

	for _, ev := range requestedEvents {
		content, ok := ev.Content().(FactoryRunRequestedContent)
		if !ok {
			continue
		}
		if !isIssueScanRunLaunch(content) {
			continue
		}
		runID := strings.TrimSpace(content.RunID)
		if runID == "" {
			continue
		}
		if seenRun[runID] {
			continue
		}
		seenRun[runID] = true

		if parked, ok := parkedByRun[runID]; ok {
			row := civilizationRecentIssueScanRunFromParked(parked, ev.ID().Value())
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: parked.EventTimestamp})
			continue
		}

		// No parked event matched this run. "No parked event" is only
		// provable when the parked page was NOT truncated (CFADA2-3).
		if parkedTruncated {
			row := civilizationRecentIssueScanRecordedRun(ev, content)
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: ev.Timestamp().Value()})
			continue
		}

		orderID, err := factoryOrderIDForRunLaunch(runID)
		inFlight := false
		if err == nil && factoryOrderByID[orderID] && stageTaskFactoryOrders[orderID] {
			inFlight = true
		}
		if inFlight {
			row := civilizationRecentIssueScanInFlightRun(ev, content, orderID)
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: ev.Timestamp().Value()})
			continue
		}
		row := civilizationRecentIssueScanQueuedRun(ev, content)
		runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: ev.Timestamp().Value()})
	}

	// Parked runs whose run_id never appeared in the requested-event page
	// (e.g. requested event outside the projection window, or run_id typo
	// upstream) still surface as parked/human_action — the parked event
	// itself is direct proof of that state regardless of queued evidence.
	for runID, parked := range parkedByRun {
		if seenRun[runID] {
			continue
		}
		seenRun[runID] = true
		row := civilizationRecentIssueScanRunFromParked(parked)
		runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: parked.EventTimestamp})
	}

	// Sort by the raw last-event timestamp (not the formatted RFC3339-second
	// string) so sub-second ordering is preserved; a run with no provable
	// timestamp sorts after every timestamped run, then by run_id.
	sort.Slice(runs, func(i, j int) bool {
		iZero, jZero := runs[i].lastEventAt.IsZero(), runs[j].lastEventAt.IsZero()
		if iZero != jZero {
			return !iZero
		}
		if !runs[i].lastEventAt.Equal(runs[j].lastEventAt) {
			return runs[i].lastEventAt.After(runs[j].lastEventAt)
		}
		return runs[i].run.RunID < runs[j].run.RunID
	})

	out.Truncated = parkedTruncated || requestedTruncated
	if len(runs) == 0 {
		if out.Truncated {
			out.Summary = "No recent issue-scan run activity is projected; the projection window is truncated and older runs may be omitted."
		}
		return out
	}

	out.Status = civilizationAssemblyFieldAvailable
	out.Runs = make([]CivilizationRecentIssueScanRun, 0, len(runs))
	for _, row := range runs {
		out.Runs = append(out.Runs, row.run)
	}
	out.Summary = fmt.Sprintf("%d recent issue-scan run(s) projected.", len(runs))
	if out.Truncated {
		out.Summary = fmt.Sprintf("%s Projection window is truncated; older runs may be omitted or degraded to recorded.", out.Summary)
	}
	return out
}

// civilizationRecentIssueScanSortableRun carries the raw last-event
// timestamp alongside the output row so ordering (D3: last_event_at DESC)
// does not lose sub-second precision to the RFC3339-second output format.
type civilizationRecentIssueScanSortableRun struct {
	run         CivilizationRecentIssueScanRun
	lastEventAt time.Time
}

// civilizationRecentIssueScanDedupeParked applies the run_id dedupe rule:
// latest event timestamp wins whole (no field mixing); ties/zero timestamps
// break by lexicographically greater event ID; source_refs are unioned.
func civilizationRecentIssueScanDedupeParked(parkedRuns []civilizationAssemblyNormalizedParkedRun) map[string]civilizationAssemblyNormalizedParkedRun {
	byRun := make(map[string]civilizationAssemblyNormalizedParkedRun, len(parkedRuns))
	refsByRun := make(map[string][]string, len(parkedRuns))
	for _, candidate := range parkedRuns {
		runID := strings.TrimSpace(candidate.RunID)
		if runID == "" {
			continue
		}
		refsByRun[runID] = append(refsByRun[runID], candidate.Refs...)
		existing, ok := byRun[runID]
		if !ok || civilizationRecentIssueScanParkedWins(candidate, existing) {
			byRun[runID] = candidate
		}
	}
	for runID, winner := range byRun {
		winner.Refs = compactStrings(append(append([]string(nil), winner.Refs...), refsByRun[runID]...))
		byRun[runID] = winner
	}
	return byRun
}

// civilizationRecentIssueScanParkedWins reports whether candidate should
// replace existing as the whole winning record: later timestamp wins; on a
// tie (including both zero), the lexicographically greater event ID wins.
func civilizationRecentIssueScanParkedWins(candidate, existing civilizationAssemblyNormalizedParkedRun) bool {
	if candidate.EventTimestamp.After(existing.EventTimestamp) {
		return true
	}
	if candidate.EventTimestamp.Before(existing.EventTimestamp) {
		return false
	}
	return candidate.EventID > existing.EventID
}

func civilizationRecentIssueScanRunFromParked(parked civilizationAssemblyNormalizedParkedRun, extraRefs ...string) CivilizationRecentIssueScanRun {
	blocker, _ := civilizationRecentIssueScanBlockerForType(parked.Blockers, parked.PrimaryBlockerType)
	timestamp := civilizationRecentIssueScanFormatTimestamp(parked.EventTimestamp)
	return CivilizationRecentIssueScanRun{
		RunID:          parked.RunID,
		FactoryOrderID: parked.FactoryOrderID,
		Repo:           parked.Issue.Repo,
		IssueNumber:    parked.Issue.Number,
		IssueURL:       parked.Issue.URL,
		IssueTitle:     parked.Issue.Title,
		State:          parked.State,
		FirstEventAt:   timestamp,
		LastEventAt:    timestamp,
		BlockerType:    blocker.BlockerType,
		RequiredAction: blocker.RequiredAction,
		StageID:        parked.StageID,
		// parked vs queued for one run: parked wins whole, refs unioned (D1)
		// — extraRefs carries the matching factory.run.requested event's ID
		// when this parked record also has queued evidence for the same run.
		SourceRefs: compactStrings(append(append([]string(nil), parked.Refs...), extraRefs...)),
	}
}

func civilizationRecentIssueScanQueuedRun(ev event.Event, content FactoryRunRequestedContent) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, civilizationRecentIssueScanStateQueued, "")
}

func civilizationRecentIssueScanInFlightRun(ev event.Event, content FactoryRunRequestedContent, orderID string) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, civilizationRecentIssueScanStateInFlight, orderID)
}

func civilizationRecentIssueScanRecordedRun(ev event.Event, content FactoryRunRequestedContent) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, civilizationRecentIssueScanStateRecorded, "")
}

func civilizationRecentIssueScanRequestedRun(ev event.Event, content FactoryRunRequestedContent, state, factoryOrderID string) CivilizationRecentIssueScanRun {
	if factoryOrderID == "" {
		if orderID, err := factoryOrderIDForRunLaunch(content.RunID); err == nil {
			factoryOrderID = orderID
		}
	}
	timestamp := civilizationRecentIssueScanFormatTimestamp(ev.Timestamp().Value())
	repo := ""
	if len(content.TargetRepos) > 0 {
		repo = strings.TrimSpace(content.TargetRepos[0])
	}
	return CivilizationRecentIssueScanRun{
		RunID:          strings.TrimSpace(content.RunID),
		FactoryOrderID: factoryOrderID,
		Repo:           repo,
		State:          state,
		FirstEventAt:   timestamp,
		LastEventAt:    timestamp,
		SourceRefs:     compactStrings([]string{ev.ID().Value()}),
	}
}

// civilizationRecentIssueScanFactoryOrdersWithStageEvidence returns the set
// of factory-order IDs carrying at least one work-task evidence record for
// an issue-scan lifecycle stage (a task with a resolvable LifecycleStageID),
// not merely the FactoryOrder's own seed task. This is the "≥1 work-task
// evidence record" proof required to promote queued -> in_flight.
func civilizationRecentIssueScanFactoryOrdersWithStageEvidence(workEvidence civilizationAssemblyFactoryOrderWorkEvidence) map[string]bool {
	out := map[string]bool{}
	for _, task := range workEvidence.Tasks {
		orderID := strings.TrimSpace(task.FactoryOrderID)
		stageID := strings.TrimSpace(task.LifecycleStageID)
		if orderID == "" || stageID == "" {
			continue
		}
		out[orderID] = true
	}
	return out
}

func civilizationRecentIssueScanFormatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
