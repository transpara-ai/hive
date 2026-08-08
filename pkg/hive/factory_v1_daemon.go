package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

// FactoryV1IssueNormalizer translates the legacy scanner's durable
// factory.run.requested events into the canonical v1 accepted-order queue. It
// does not invoke the legacy seven-stage dispatcher.
type FactoryV1IssueNormalizer struct {
	store   store.Store
	intake  *factoryv1.Intake
	actorID string
}

func NewFactoryV1IssueNormalizer(eventStore store.Store, intake *factoryv1.Intake, actorID string) (*FactoryV1IssueNormalizer, error) {
	if eventStore == nil || intake == nil || strings.TrimSpace(actorID) == "" {
		return nil, errors.New("factory v1 issue normalizer requires EventGraph store, intake, and actor")
	}
	return &FactoryV1IssueNormalizer{store: eventStore, intake: intake, actorID: actorID}, nil
}

// RunOnce scans the complete bounded/paginated request stream. Intake
// idempotency makes replay safe and repairs a missing Work projection.
func (n *FactoryV1IssueNormalizer) RunOnce(ctx context.Context) (int, error) {
	requests, err := factoryV1RequestedEvents(ctx, n.store)
	if err != nil {
		return 0, err
	}
	normalized := 0
	var normalizeErrors []error
	for _, request := range requests {
		content, ok := request.Content().(FactoryRunRequestedContent)
		if !ok {
			normalizeErrors = append(normalizeErrors, fmt.Errorf("factory.run.requested %s has content %T", request.ID().Value(), request.Content()))
			continue
		}
		admission, recognized, err := factoryV1IssueAdmission(request.ID(), content, n.actorID)
		if err != nil {
			normalizeErrors = append(normalizeErrors, fmt.Errorf("normalize request %s: %w", request.ID().Value(), err))
			continue
		}
		if !recognized {
			continue
		}
		if _, err := n.intake.NormalizeIssue(ctx, admission); err != nil {
			if errors.Is(err, factoryv1.ErrIssueAmendmentBlocked) {
				// The intake has durably recorded the amendment and intervention.
				normalizeErrors = append(normalizeErrors, err)
				continue
			}
			normalizeErrors = append(normalizeErrors, fmt.Errorf("accept issue request %s: %w", request.ID().Value(), err))
			continue
		}
		normalized++
	}
	return normalized, errors.Join(normalizeErrors...)
}

func factoryV1RequestedEvents(ctx context.Context, eventStore store.Store) ([]event.Event, error) {
	var result []event.Event
	cursor := types.None[types.Cursor]()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := eventStore.ByType(EventTypeFactoryRunRequested, 200, cursor)
		if err != nil {
			return nil, fmt.Errorf("list factory.run.requested for v1 normalization: %w", err)
		}
		for _, item := range page.Items() {
			result = append(result, item)
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].Timestamp().Value().Before(result[right].Timestamp().Value())
	})
	return result, nil
}

type factoryV1IssueBrief struct {
	Kind          string `json:"kind"`
	SelectedIssue struct {
		Repo   string   `json:"repo"`
		Number int      `json:"number"`
		Title  string   `json:"title"`
		URL    string   `json:"url"`
		Labels []string `json:"labels,omitempty"`
		Body   string   `json:"body_excerpt,omitempty"`
	} `json:"selected_issue"`
}

func factoryV1IssueAdmission(requestID types.EventID, content FactoryRunRequestedContent, actorID string) (factoryv1.IssueAdmission, bool, error) {
	if strings.TrimSpace(content.Status) != "queued" {
		return factoryv1.IssueAdmission{}, false, nil
	}
	var brief factoryV1IssueBrief
	if err := json.Unmarshal(content.Brief, &brief); err != nil {
		return factoryv1.IssueAdmission{}, false, fmt.Errorf("decode issue-scan brief: %w", err)
	}
	if strings.TrimSpace(brief.Kind) != issueScanBriefKind {
		return factoryv1.IssueAdmission{}, false, nil
	}
	repository := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if !ValidTransparaAIRepo(repository) || brief.SelectedIssue.Number <= 0 || strings.TrimSpace(brief.SelectedIssue.Title) == "" {
		return factoryv1.IssueAdmission{}, true, errors.New("issue-scan brief selected issue is incomplete or outside transpara-ai")
	}
	if len(content.TargetRepos) != 1 || !strings.EqualFold(strings.TrimSpace(content.TargetRepos[0]), repository) {
		return factoryv1.IssueAdmission{}, true, errors.New("issue-scan brief repository does not match the exact queued target")
	}
	sourceIdentity := fmt.Sprintf("github:%s#%d", repository, brief.SelectedIssue.Number)
	maxAttempts := content.Budget.MaxIterations
	if maxAttempts < len(factoryv1.TLCStages) {
		maxAttempts = len(factoryv1.TLCStages) * 2
	}
	maxCostMicros := int64(0)
	if content.Budget.MaxCostUSD > 0 {
		if content.Budget.MaxCostUSD > float64(math.MaxInt64)/1_000_000 {
			return factoryv1.IssueAdmission{}, true, errors.New("issue-scan budget exceeds v1 integer range")
		}
		maxCostMicros = int64(content.Budget.MaxCostUSD * 1_000_000)
	}
	body := strings.TrimSpace(brief.SelectedIssue.Body)
	if body == "" {
		body = strings.TrimSpace(brief.SelectedIssue.Title)
	}
	order := factoryv1.FactoryOrder{
		DocID: "FO-ISSUE-" + strings.ToUpper(factoryv1.HashText(sourceIdentity)[:20]), Version: "1.0.0", Status: "approved",
		Title: strings.TrimSpace(brief.SelectedIssue.Title), Channel: factoryv1.ChannelIssueScan, TargetRepository: repository,
		Requirements: []factoryv1.Requirement{{
			ID: "R1", Statement: body,
			Rationale: "The interval issue scanner selected this explicitly labelled GitHub issue for bounded implementation.",
		}},
		AcceptanceCriteria: []factoryv1.AcceptanceCriterion{{
			ID: "AC1", Statement: "The selected issue is implemented and verified at an exact pull-request head.",
			VerificationMethod: "Run repository-prescribed tests, IAR/CFAR, required checks, and exact-head ready-state validation.", RiskClass: "high",
		}},
		TestPlan:        []string{"Run the target repository's prescribed verification and retain exact-head evidence."},
		Constraints:     []string{"Non-production only", "No merge or deploy", "Do not expand beyond the selected issue"},
		NonGoals:        []string{"Unrelated refactors", "Default-branch direct mutation", "Authority expansion"},
		ExpectedOutputs: []string{"Open non-draft exact-head pull request with passing required checks", "TLC evidence ledger"},
		Authority: factoryv1.AuthorityScope{
			ActorID:            actorID,
			AllowedActions:     []string{"repo.branch.create", "repo.commit.create", "repo.pull_request.create", "repo.pull_request.mark_ready", "governance.review.record"},
			TargetRepositories: []string{repository}, NonProductionOnly: true,
		},
		Budget: factoryv1.BudgetLimit{MaxAttempts: maxAttempts, MaxTokens: 2_000_000, MaxCostMicros: maxCostMicros},
	}
	return factoryv1.IssueAdmission{
		LaunchEventID: requestID.Value(), Repository: repository, IssueNumber: brief.SelectedIssue.Number,
		Title: brief.SelectedIssue.Title, Body: brief.SelectedIssue.Body, Order: order, ActorID: actorID,
	}, true, nil
}

type factoryV1SchedulerLoop interface {
	RunOnce(context.Context) error
}

type factoryV1IssueLoop interface {
	RunOnce(context.Context) (int, error)
}

type FactoryV1DaemonConfig struct {
	PollInterval time.Duration
	OnError      func(error)
	Runtime      *FactoryRuntimeMonitor
}

// FactoryV1Daemon continuously normalizes scanner events and advances the
// durable scheduler. A failed execute leaves a running transition; the next
// cycle therefore reaches Scheduler's reconcile-before-retry path instead of
// blindly repeating the effect.
type FactoryV1Daemon struct {
	normalizer factoryV1IssueLoop
	scheduler  factoryV1SchedulerLoop
	config     FactoryV1DaemonConfig
}

func NewFactoryV1Daemon(normalizer factoryV1IssueLoop, scheduler factoryV1SchedulerLoop, config FactoryV1DaemonConfig) (*FactoryV1Daemon, error) {
	if normalizer == nil || scheduler == nil {
		return nil, errors.New("factory v1 daemon requires issue normalizer and scheduler")
	}
	if config.PollInterval == 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.PollInterval < 100*time.Millisecond || config.PollInterval > 10*time.Minute {
		return nil, errors.New("factory v1 daemon poll interval must be between 100ms and 10m")
	}
	return &FactoryV1Daemon{normalizer: normalizer, scheduler: scheduler, config: config}, nil
}

func (d *FactoryV1Daemon) Run(ctx context.Context) error {
	if d.config.Runtime != nil {
		d.config.Runtime.SetState(FactoryRuntimePolling, nil)
		go d.config.Runtime.RunHeartbeat(ctx)
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			if d.config.Runtime != nil {
				d.config.Runtime.SetState(FactoryRuntimeStopping, nil)
			}
			return nil
		case <-timer.C:
			var cycleErrors []error
			if _, err := d.normalizer.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				cycleErr := fmt.Errorf("factory v1 issue normalization cycle: %w", err)
				cycleErrors = append(cycleErrors, cycleErr)
				d.report(cycleErr)
			}
			if d.config.Runtime != nil {
				d.config.Runtime.SetState(FactoryRuntimeExecuting, nil)
			}
			if err := d.scheduler.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				cycleErr := fmt.Errorf("factory v1 scheduler cycle: %w", err)
				cycleErrors = append(cycleErrors, cycleErr)
				d.report(cycleErr)
			}
			if d.config.Runtime != nil {
				if joined := errors.Join(cycleErrors...); joined != nil {
					d.config.Runtime.SetState(FactoryRuntimeDegraded, joined)
				} else {
					d.config.Runtime.SetState(FactoryRuntimePolling, nil)
				}
			}
			timer.Reset(d.config.PollInterval)
		}
	}
}

func (d *FactoryV1Daemon) report(err error) {
	if d.config.OnError != nil {
		d.config.OnError(err)
	}
}
