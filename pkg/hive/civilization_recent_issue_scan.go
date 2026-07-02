package hive

import (
	"encoding/json"
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
// evidence as-is (I/O error, nil store, or empty page) — this is the board
// fold's existing short-circuit signal and is preserved unchanged.
//
// The fifth return value, fetched, is a STRICTLY NARROWER signal than ok:
// fetched=true iff the store fetch itself succeeded (s non-nil, ByType
// returned no error), REGARDLESS of whether the resulting page was empty.
// fetched=false means the page could not be read at all (nil store or
// ByType error) — parked-absence is UNKNOWN, not proven. This distinction
// matters to the rail fold, which (unlike the board fold) still has
// independent requested-run evidence to evaluate when the parked page is
// confirmably empty (fetched=true, ok=false, zero events): that is proof of
// parked-absence, not an unreadable page, so the rail must not be forced
// unavailable in that case. Only fetched=false forces the rail unavailable.
func civilizationAssemblyNormalizedParkedRuns(s store.Store, limit int) ([]civilizationAssemblyNormalizedParkedRun, bool, civilizationAssemblyIssueScanProjectionEvidence, bool, bool) {
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
		return nil, false, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Errors: []string{"project issue-scan records: store is required"}}, false, false
	}
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	page, err := s.ByType(EventTypeIssueScanRunParked, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Errors: []string{"project issue-scan parked records: " + err.Error()}}, false, false
	}
	events := page.Items()
	truncated := page.HasMore()
	if len(events) == 0 {
		return nil, truncated, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Truncated: truncated}, false, true
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
	return normalized, truncated, civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Truncated: truncated}, true, true
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

// civilizationRecentIssueScanBrief carries the fields the rail needs from a
// factory.run.requested event's Brief payload: the selected issue (repo,
// number, URL, title — CFAR finding 3) so queued/in_flight/recorded rows can
// populate real issue identity instead of leaving those fields at zero
// values. An empty Repo means the brief carried no selected issue (or wasn't
// an issue-scan brief), which the caller falls back to TargetRepos[0] for.
type civilizationRecentIssueScanBrief struct {
	Repo   string
	Number int
	URL    string
	Title  string
}

// civilizationRecentIssueScanParseBrief parses content.Brief EXACTLY ONCE,
// extracting both the issue-scan kind predicate (previously
// isIssueScanRunLaunch's sole responsibility) and the selected-issue fields
// in the same decode, so the fold no longer parses each requested event's
// Brief JSON twice for two different purposes. ok=false means the event is
// not an issue-scan run launch (wrong/missing kind) and must be excluded
// from the rail entirely, exactly as isIssueScanRunLaunch's callers already
// require; a brief with the right kind but no selected issue still returns
// ok=true with a zero-value civilizationRecentIssueScanBrief (Repo=="" so
// the caller's TargetRepos[0] fallback applies).
func civilizationRecentIssueScanParseBrief(content FactoryRunRequestedContent) (civilizationRecentIssueScanBrief, bool) {
	var raw struct {
		Kind          string                     `json:"kind"`
		SelectedIssue issueScanBriefIssuePayload `json:"selected_issue"`
	}
	if err := json.Unmarshal(content.Brief, &raw); err != nil {
		return civilizationRecentIssueScanBrief{}, false
	}
	if strings.TrimSpace(raw.Kind) != issueScanBriefKind {
		return civilizationRecentIssueScanBrief{}, false
	}
	return civilizationRecentIssueScanBrief{
		Repo:   strings.TrimSpace(raw.SelectedIssue.Repo),
		Number: raw.SelectedIssue.Number,
		URL:    strings.TrimSpace(raw.SelectedIssue.URL),
		Title:  strings.TrimSpace(raw.SelectedIssue.Title),
	}, true
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
//
// parked-fetch failure vs parked-page truncation are DIFFERENT signals and
// must not be conflated:
//   - Truncation (parkedTruncated=true) means the parked page is REAL but
//     INCOMPLETE — a requested run with no matching parked row cannot be
//     proven parked-absent, so it degrades to recorded (see below).
//   - Fetch failure (parkedFetched=false) means there is NO parked evidence
//     at all — the page could not be read (nil store, ByType error). This is
//     narrower than "the page was empty": an empty-but-successfully-fetched
//     page (parkedFetched=true, zero parkedRuns) IS proof of parked-absence
//     and is handled the normal way below. Only an unreadable page makes
//     "no parked event matched this run_id" unknowable rather than merely
//     unproven — evaluating requested runs for in_flight/queued in that case
//     would silently assume parked-absence that was never demonstrated. This
//     fold therefore fails CLOSED on parkedFetched=false: the whole rail
//     becomes unavailable with zero runs, mirroring the board fold's
//     short-circuit on the same underlying fetch failure
//     (civilizationAssemblyIssueScanProjections).
//
// requestedFetched mirrors parkedFetched for the OTHER primary evidence
// source: requestedFetched=false means the factory.run.requested page itself
// could not be read (nil store, ByType error). Without it the fold has no
// signal for ANY requested run — not even "queued vs recorded" — so the
// whole rail fails CLOSED the same way as an unreadable parked page (CFAR
// finding 1).
//
// workEvidenceQueryFailed/workEvidenceTruncated propagate the THIRD evidence
// source's own uncertainty: the reused work.task.created
// query/factory-order-evidence computation (civilizationAssemblyFactoryOrders
// in the builder) can itself fail outright or truncate. Either condition
// makes "does this factory order carry stage work-task evidence" unprovable
// — the fold can prove neither work-evidence-absence (queued) nor
// work-evidence-presence-and-completeness (in_flight) for requested runs —
// so every requested run that would otherwise be evaluated for in_flight/
// queued degrades to recorded instead (existence proven via the issue-scan
// predicate match, position unproven). Truncated is set to true only when
// workEvidenceTruncated caused the degrade (truncation is itself evidence
// the projection window is incomplete); a pure query failure is reported
// through operatorProjection.Errors/FailureReasons by the builder already,
// so it does not additionally claim truncation. Parked rows are UNAFFECTED
// — their evidence (hive.issuescan.run.parked) is independent of work-task
// evidence.
//
// workEvidenceQueryFailed is NOT limited to the work.task.created page fetch
// itself. civilizationAssemblyFactoryOrders performs several Work reads
// while assembling factoryOrderWorkEvidence — the work.task.created page,
// the task-artifact/dependency/lifecycle-transition/verification ByType
// reads, and the per-task civilizationAssemblyProjectWorkTask projection
// (which itself reads lifecycle transitions) for every task-created event.
// workEvidenceQueryFailed is an ALLOWLIST over that whole set: it is true
// (query failed) unless EVERY one of those sub-reads is proven to have
// succeeded. A failure in any single sub-read (for example,
// work.task.lifecycle.transitioned erroring while the task-created page
// itself succeeds) silently omits the affected task from
// factoryOrderWorkEvidence.Tasks — "no stage work evidence" for that factory
// order is then unproven, not proven-absent, so it must not be read as
// grounds for a queued row (CFAR round 2 finding 1). This is why the flag is
// computed by OR-ing every sub-read's error signal at the source rather than
// by inspecting FailureReasons here: string-matching accumulated error text
// is not a reliable substitute for a fail-closed boolean threaded from the
// read site.
//
// The completed evidence-uncertainty matrix this fold enforces:
//   - parked page: unreadable -> whole rail unavailable (section
//     unavailable); truncated -> per-run degrade to recorded when no parked
//     row matches (absence unproven, not proven).
//   - requested page: unreadable -> whole rail unavailable (section
//     unavailable); truncated -> honest absence is impossible to claim, but
//     since requestedEvents only iterates what WAS fetched, a requested
//     page's own truncation only risks omitting rows entirely (they are
//     simply not present in requestedEvents), not misclassifying rows that
//     ARE present.
//   - work evidence (factory-order/stage work-task evidence, all sub-reads
//     enumerated above): ANY read failure or truncation across the whole
//     set -> every requested run that would otherwise be evaluated for
//     in_flight/queued degrades to recorded instead. No queued or in_flight
//     promotion is ever made from a work-evidence computation that is not
//     proven fully successful and complete.
func civilizationRecentIssueScanRuns(
	parkedRuns []civilizationAssemblyNormalizedParkedRun,
	parkedTruncated bool,
	parkedFetched bool,
	requestedEvents []event.Event,
	requestedTruncated bool,
	requestedFetched bool,
	factoryOrders []CivilizationAssemblyFactoryOrder,
	factoryOrderWorkEvidence civilizationAssemblyFactoryOrderWorkEvidence,
	workEvidenceQueryFailed bool,
	workEvidenceTruncated bool,
) CivilizationRecentIssueScanRuns {
	out := CivilizationRecentIssueScanRuns{
		Status:  civilizationAssemblyFieldUnavailable,
		Summary: "No recent issue-scan run activity is projected.",
	}

	if !parkedFetched {
		out.Summary = "Recent issue-scan runs are unavailable: the parked-run evidence page could not be read."
		return out
	}
	if !requestedFetched {
		out.Summary = "Recent issue-scan runs are unavailable: the requested-run evidence page could not be read."
		return out
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
	// workEvidenceUnprovable: neither "no work evidence" (queued) nor "work
	// evidence present" (in_flight) can be trusted from a failed or
	// truncated work-task-evidence source, so both promotions are withheld
	// (CFAR finding 1).
	workEvidenceUnprovable := workEvidenceQueryFailed || workEvidenceTruncated

	runs := make([]civilizationRecentIssueScanSortableRun, 0, len(parkedByRun)+len(requestedEvents))
	seenRun := map[string]bool{}

	for _, ev := range requestedEvents {
		content, ok := ev.Content().(FactoryRunRequestedContent)
		if !ok {
			continue
		}
		brief, ok := civilizationRecentIssueScanParseBrief(content)
		if !ok {
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
			row := civilizationRecentIssueScanRecordedRun(ev, content, brief)
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: ev.Timestamp().Value()})
			continue
		}

		// Work-task evidence itself is unprovable (query failed or
		// truncated): cannot prove absence (queued) nor presence+completeness
		// (in_flight) for this requested run, so it degrades to recorded
		// (CFAR finding 1).
		if workEvidenceUnprovable {
			row := civilizationRecentIssueScanRecordedRun(ev, content, brief)
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: ev.Timestamp().Value()})
			continue
		}

		orderID, err := factoryOrderIDForRunLaunch(runID)
		proof, hasProof := civilizationRecentIssueScanStageProof{}, false
		if err == nil && factoryOrderByID[orderID] {
			proof, hasProof = stageTaskFactoryOrders[orderID]
		}
		if hasProof {
			row := civilizationRecentIssueScanInFlightRun(ev, content, orderID, proof, brief)
			lastEventAt := ev.Timestamp().Value()
			if proof.TimestampMS > 0 {
				proofTime := time.UnixMilli(proof.TimestampMS).UTC()
				if proofTime.After(lastEventAt) {
					lastEventAt = proofTime
				}
			}
			runs = append(runs, civilizationRecentIssueScanSortableRun{run: row, lastEventAt: lastEventAt})
			continue
		}
		row := civilizationRecentIssueScanQueuedRun(ev, content, brief)
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

	out.Truncated = parkedTruncated || requestedTruncated || workEvidenceTruncated
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

func civilizationRecentIssueScanQueuedRun(ev event.Event, content FactoryRunRequestedContent, brief civilizationRecentIssueScanBrief) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, brief, civilizationRecentIssueScanStateQueued, "", civilizationRecentIssueScanStageProof{})
}

func civilizationRecentIssueScanInFlightRun(ev event.Event, content FactoryRunRequestedContent, orderID string, proof civilizationRecentIssueScanStageProof, brief civilizationRecentIssueScanBrief) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, brief, civilizationRecentIssueScanStateInFlight, orderID, proof)
}

func civilizationRecentIssueScanRecordedRun(ev event.Event, content FactoryRunRequestedContent, brief civilizationRecentIssueScanBrief) CivilizationRecentIssueScanRun {
	return civilizationRecentIssueScanRequestedRun(ev, content, brief, civilizationRecentIssueScanStateRecorded, "", civilizationRecentIssueScanStageProof{})
}

func civilizationRecentIssueScanRequestedRun(ev event.Event, content FactoryRunRequestedContent, brief civilizationRecentIssueScanBrief, state, factoryOrderID string, proof civilizationRecentIssueScanStageProof) CivilizationRecentIssueScanRun {
	if factoryOrderID == "" {
		if orderID, err := factoryOrderIDForRunLaunch(content.RunID); err == nil {
			factoryOrderID = orderID
		}
	}
	requestedTimestamp := ev.Timestamp().Value()
	timestamp := civilizationRecentIssueScanFormatTimestamp(requestedTimestamp)
	// TargetRepos[0] is guarded by the length check below: an empty
	// TargetRepos leaves Repo as "" (acceptable per contract — Repo is only
	// proven when the requested event actually names a target repo) rather
	// than indexing out of range. This is the single shared constructor for
	// the queued/in_flight/recorded rows, so the guard covers all three.
	// Brief-sourced issue fields (CFAR finding 3) take precedence when the
	// brief carries a selected issue; Repo falls back to TargetRepos[0] only
	// when the brief has none.
	repo := brief.Repo
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.TrimSpace(content.TargetRepos[0])
	}
	sourceRefs := []string{ev.ID().Value()}
	stageID := ""
	lastEventAtTimestamp := requestedTimestamp
	if state == civilizationRecentIssueScanStateInFlight {
		// CFAR finding 2: an in_flight row must carry the stage-evidence task
		// reference that PROVES the state, not just the requested event's own
		// ID — otherwise the row's evidence doesn't substantiate the state it
		// claims. last_event_at reflects the LATEST of the requested event and
		// the proving task evidence; first_event_at stays the requested event
		// (the run's own origin never moves).
		if proof.TaskRef != "" {
			sourceRefs = append(sourceRefs, proof.TaskRef)
		}
		stageID = proof.StageID
		if proof.TimestampMS > 0 {
			proofTime := time.UnixMilli(proof.TimestampMS).UTC()
			if proofTime.After(lastEventAtTimestamp) {
				lastEventAtTimestamp = proofTime
			}
		}
	}
	lastEventAt := civilizationRecentIssueScanFormatTimestamp(lastEventAtTimestamp)
	return CivilizationRecentIssueScanRun{
		RunID:          strings.TrimSpace(content.RunID),
		FactoryOrderID: factoryOrderID,
		Repo:           repo,
		IssueNumber:    brief.Number,
		IssueURL:       brief.URL,
		IssueTitle:     brief.Title,
		State:          state,
		FirstEventAt:   timestamp,
		LastEventAt:    lastEventAt,
		StageID:        stageID,
		SourceRefs:     compactStrings(sourceRefs),
	}
}

// civilizationRecentIssueScanStageProof carries the PROVING work-task
// evidence for one factory order's in_flight promotion: the task reference
// (event ID or canonical task ID — whichever the task evidence exposes),
// the lifecycle stage ID, and that task's own event timestamp (derived from
// its UUIDv7 event ID, since CivilizationAssemblyTaskEvidence carries no
// timestamp field of its own). When a factory order has more than one
// qualifying stage task, the LATEST one wins (CFAR finding 2: last_event_at
// must reflect the latest work-task evidence, not merely the first match).
type civilizationRecentIssueScanStageProof struct {
	TaskRef     string
	StageID     string
	TimestampMS int64
}

// civilizationRecentIssueScanFactoryOrdersWithStageEvidence returns, per
// factory-order ID, the PROVING work-task evidence record for an issue-scan
// lifecycle stage (a task with a resolvable LifecycleStageID), not merely
// the FactoryOrder's own seed task. This is the "≥1 work-task evidence
// record" proof required to promote queued -> in_flight (CFAR finding 2:
// the in_flight row must carry this proof in its own source_refs/stage_id/
// last_event_at, not just the requested event's).
func civilizationRecentIssueScanFactoryOrdersWithStageEvidence(workEvidence civilizationAssemblyFactoryOrderWorkEvidence) map[string]civilizationRecentIssueScanStageProof {
	out := map[string]civilizationRecentIssueScanStageProof{}
	for _, task := range workEvidence.Tasks {
		orderID := strings.TrimSpace(task.FactoryOrderID)
		stageID := strings.TrimSpace(task.LifecycleStageID)
		if orderID == "" || stageID == "" {
			continue
		}
		taskRef := strings.TrimSpace(task.CanonicalTaskID)
		if taskRef == "" {
			taskRef = strings.TrimSpace(task.ID)
		}
		if taskRef == "" {
			continue
		}
		var taskTimestampMS int64
		if id, err := types.NewEventID(task.ID); err == nil {
			taskTimestampMS = id.TimestampMS()
		}
		candidate := civilizationRecentIssueScanStageProof{TaskRef: taskRef, StageID: stageID, TimestampMS: taskTimestampMS}
		existing, ok := out[orderID]
		if !ok || candidate.TimestampMS > existing.TimestampMS {
			out[orderID] = candidate
		}
	}
	return out
}

func civilizationRecentIssueScanFormatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
