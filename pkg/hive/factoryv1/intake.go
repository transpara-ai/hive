package factoryv1

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrWorkNotFound          = errors.New("factory v1 Work linkage not found")
	ErrAcceptedTupleConflict = errors.New("factory v1 accepted tuple conflict")
	ErrIssueAmendmentBlocked = errors.New("factory v1 issue amendment requires Human resolution")
	ErrIdeaNotFound          = errors.New("factory v1 idea not found")
	ErrIdeaApprovalRequired  = errors.New("factory v1 idea requires explicit Human approval")
	ErrOrphanWorkQuarantined = errors.New("factory v1 Work task has no accepted event and was quarantined")
)

type WorkSeed struct {
	OrderID         string            `json:"order_id"`
	Version         string            `json:"version"`
	DocumentSHA256  string            `json:"document_sha256"`
	Markdown        string            `json:"markdown"`
	SourceSHA256    string            `json:"source_sha256"`
	AcceptedEventID string            `json:"accepted_event_id"`
	IdempotencyKey  string            `json:"idempotency_key"`
	Metadata        map[string]string `json:"metadata"`
}

type WorkLink struct {
	TaskID          string            `json:"task_id"`
	ArtifactID      string            `json:"artifact_id"`
	OrderID         string            `json:"order_id"`
	Version         string            `json:"version"`
	DocumentSHA256  string            `json:"document_sha256"`
	AcceptedEventID string            `json:"accepted_event_id"`
	Quarantined     bool              `json:"quarantined"`
	Metadata        map[string]string `json:"metadata"`
}

type WorkArtifact struct {
	ArtifactID   string     `json:"artifact_id"`
	OrderID      string     `json:"order_id"`
	Stage        Stage      `json:"stage"`
	AttemptID    string     `json:"attempt_id"`
	StageEventID string     `json:"stage_event_id"`
	Evidence     []Evidence `json:"evidence"`
}

type WorkStore interface {
	GetFactoryOrder(ctx context.Context, orderID, version string) (WorkLink, error)
	SeedFactoryOrder(ctx context.Context, seed WorkSeed) (WorkLink, error)
	ListFactoryOrders(ctx context.Context) ([]WorkLink, error)
	QuarantineFactoryOrder(ctx context.Context, link WorkLink, reason string) error
	AttachStageArtifact(ctx context.Context, artifact WorkArtifact) (string, error)
}

type InMemoryWorkStore struct {
	mu        sync.RWMutex
	links     map[string]WorkLink
	artifacts map[string]WorkArtifact
}

func NewInMemoryWorkStore() *InMemoryWorkStore {
	return &InMemoryWorkStore{links: make(map[string]WorkLink), artifacts: make(map[string]WorkArtifact)}
}

func workTuple(orderID, version string) string { return orderID + "@" + version }

func (s *InMemoryWorkStore) GetFactoryOrder(ctx context.Context, orderID, version string) (WorkLink, error) {
	if err := ctx.Err(); err != nil {
		return WorkLink{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	link, exists := s.links[workTuple(orderID, version)]
	if !exists {
		return WorkLink{}, ErrWorkNotFound
	}
	return cloneWorkLink(link), nil
}

func (s *InMemoryWorkStore) SeedFactoryOrder(ctx context.Context, seed WorkSeed) (WorkLink, error) {
	if err := ctx.Err(); err != nil {
		return WorkLink{}, err
	}
	if seed.OrderID == "" || seed.Version == "" || !hexPattern.MatchString(seed.DocumentSHA256) || seed.Markdown == "" || seed.AcceptedEventID == "" || seed.IdempotencyKey == "" {
		return WorkLink{}, errors.New("invalid factory v1 Work seed")
	}
	key := workTuple(seed.OrderID, seed.Version)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.links[key]; exists {
		if existing.DocumentSHA256 != seed.DocumentSHA256 || existing.AcceptedEventID != seed.AcceptedEventID {
			return WorkLink{}, fmt.Errorf("%w: Work tuple %s already has different accepted truth", ErrAcceptedTupleConflict, key)
		}
		return cloneWorkLink(existing), nil
	}
	link := WorkLink{
		TaskID:          "work-task-" + seed.DocumentSHA256[:16],
		ArtifactID:      "work-artifact-" + seed.DocumentSHA256[16:32],
		OrderID:         seed.OrderID,
		Version:         seed.Version,
		DocumentSHA256:  seed.DocumentSHA256,
		AcceptedEventID: seed.AcceptedEventID,
		Metadata:        cloneMap(seed.Metadata),
	}
	s.links[key] = link
	return cloneWorkLink(link), nil
}

func (s *InMemoryWorkStore) ListFactoryOrders(ctx context.Context) ([]WorkLink, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkLink, 0, len(s.links))
	for _, link := range s.links {
		result = append(result, cloneWorkLink(link))
	}
	sort.Slice(result, func(i, j int) bool {
		return workTuple(result[i].OrderID, result[i].Version) < workTuple(result[j].OrderID, result[j].Version)
	})
	return result, nil
}

func (s *InMemoryWorkStore) QuarantineFactoryOrder(ctx context.Context, link WorkLink, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("quarantine reason is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := workTuple(link.OrderID, link.Version)
	current, exists := s.links[key]
	if !exists {
		current = link
	}
	current.Quarantined = true
	current.Metadata = cloneMap(current.Metadata)
	if current.Metadata == nil {
		current.Metadata = make(map[string]string)
	}
	current.Metadata["quarantine_reason"] = reason
	s.links[key] = current
	return nil
}

func (s *InMemoryWorkStore) AttachStageArtifact(ctx context.Context, artifact WorkArtifact) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if artifact.OrderID == "" || StageIndex(artifact.Stage) < 0 || artifact.AttemptID == "" || artifact.StageEventID == "" || len(artifact.Evidence) == 0 {
		return "", errors.New("invalid Work stage artifact")
	}
	if artifact.ArtifactID == "" {
		artifact.ArtifactID = "work-stage-" + HashText(artifact.OrderID + "\x00" + string(artifact.Stage) + "\x00" + artifact.AttemptID)[:24]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.artifacts[artifact.ArtifactID]; ok {
		if existing.OrderID != artifact.OrderID || existing.Stage != artifact.Stage || existing.AttemptID != artifact.AttemptID {
			return "", ErrIdempotencyConflict
		}
		return artifact.ArtifactID, nil
	}
	s.artifacts[artifact.ArtifactID] = artifact
	return artifact.ArtifactID, nil
}

func cloneWorkLink(link WorkLink) WorkLink {
	link.Metadata = cloneMap(link.Metadata)
	return link
}

func cloneMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

type AcceptanceReceipt struct {
	OrderID         string                `json:"order_id"`
	Version         string                `json:"version"`
	Channel         Channel               `json:"channel"`
	DocumentSHA256  string                `json:"document_sha256"`
	AcceptedEventID string                `json:"accepted_event_id"`
	Work            WorkLink              `json:"work"`
	Document        CanonicalDocument     `json:"document"`
	Approval        *HumanApprovalReceipt `json:"approval,omitempty"`
}

type Intake struct {
	store Store
	work  WorkStore
	clock Clock
}

func NewIntake(store Store, work WorkStore, clock Clock) (*Intake, error) {
	if store == nil || work == nil {
		return nil, errors.New("factory v1 intake requires EventGraph and Work stores")
	}
	if clock == nil {
		clock = WallClock{}
	}
	return &Intake{store: store, work: work, clock: clock}, nil
}

type AcceptOptions struct {
	SourceIdentity  string
	SourceEventIDs  []string
	ActorID         string
	CredentialKeyID string
	Approval        *HumanApprovalReceipt
}

func (i *Intake) AcceptCompleted(ctx context.Context, order FactoryOrder, options AcceptOptions) (AcceptanceReceipt, error) {
	if order.Channel != ChannelCompletedOrder {
		return AcceptanceReceipt{}, errors.New("completed FactoryOrder channel must be completed_factory_order")
	}
	return i.accept(ctx, order, options)
}

// RecordCompletedSubmission writes the typed causal source event used by the
// direct completed-order endpoint. Validation happens before the append, so an
// invalid document never masquerades as an admitted submission.
func (i *Intake) RecordCompletedSubmission(ctx context.Context, order FactoryOrder, actorID, credentialKeyID string) (Event, error) {
	if order.Channel != ChannelCompletedOrder {
		return Event{}, errors.New("completed FactoryOrder channel must be completed_factory_order")
	}
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(credentialKeyID) == "" {
		return Event{}, errors.New("completed FactoryOrder submission requires Human actor and credential key IDs")
	}
	document, err := Canonicalize(order)
	if err != nil {
		return Event{}, err
	}
	payload := CompletedOrderSubmittedPayload{Document: document, ActorID: actorID, CredentialKeyID: credentialKeyID}
	return AppendTyped(ctx, i.store, EventOrderSubmitted, order.DocID,
		"completed-order-submitted:"+order.DocID+"@"+order.Version+":"+document.SHA256, nil, payload)
}

// SubmitCompleted records an honest direct-submission source event and then
// accepts the exact same canonical document causally from that event.
func (i *Intake) SubmitCompleted(ctx context.Context, order FactoryOrder, actorID, credentialKeyID string, approvals ...*HumanApprovalReceipt) (AcceptanceReceipt, error) {
	if len(approvals) > 1 {
		return AcceptanceReceipt{}, errors.New("completed FactoryOrder accepts at most one approval receipt")
	}
	var approval *HumanApprovalReceipt
	if len(approvals) == 1 {
		approval = approvals[0]
	}
	submitted, err := i.RecordCompletedSubmission(ctx, order, actorID, credentialKeyID)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	document, err := Canonicalize(order)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	return i.AcceptCompleted(ctx, order, AcceptOptions{
		SourceIdentity: "completed-order:" + order.DocID + "@" + order.Version + ":" + document.SHA256,
		SourceEventIDs: []string{submitted.ID}, ActorID: actorID, CredentialKeyID: credentialKeyID, Approval: approval,
	})
}

type IssueAdmission struct {
	LaunchEventID string
	Repository    string
	IssueNumber   int
	Title         string
	Body          string
	Order         FactoryOrder
	ActorID       string
}

func (i *Intake) NormalizeIssue(ctx context.Context, admission IssueAdmission) (AcceptanceReceipt, error) {
	if admission.LaunchEventID == "" || !repoPattern.MatchString(admission.Repository) || admission.IssueNumber <= 0 {
		return AcceptanceReceipt{}, errors.New("issue admission requires launch event, repository, and positive issue number")
	}
	sourceIdentity := fmt.Sprintf("github:%s#%d", admission.Repository, admission.IssueNumber)
	sourceSHA := HashText(strings.TrimSpace(admission.Title) + "\n\n" + strings.TrimSpace(admission.Body))
	if prior, ok, err := i.activeIssueClaim(ctx, sourceIdentity); err != nil {
		return AcceptanceReceipt{}, err
	} else if ok {
		priorSHA := prior.Document.Order.SourceReferences[0].SHA256
		if priorSHA == sourceSHA {
			return i.repairReceipt(ctx, prior)
		}
		amendment := IssueAmendmentPayload{
			SourceIdentity: sourceIdentity, ActiveOrderID: prior.Document.Order.DocID,
			PriorSourceSHA256: priorSHA, NewSourceSHA256: sourceSHA,
			Reason: "an active issue claim cannot be silently replaced by edited source",
		}
		amendmentEvent, appendErr := AppendTyped(ctx, i.store, EventIssueAmendmentRecorded, prior.Document.Order.DocID,
			"issue-amendment:"+sourceIdentity+":"+sourceSHA, []string{prior.Event.ID, admission.LaunchEventID}, amendment)
		if appendErr != nil {
			return AcceptanceReceipt{}, appendErr
		}
		_, requestErr := i.requestIntervention(ctx, prior.Document.Order.DocID, StageIngestWork, "issue_source_amendment",
			"Resolve the edited active issue source before admitting a new FactoryOrder version.", "", []string{amendmentEvent.ID})
		if requestErr != nil {
			return AcceptanceReceipt{}, requestErr
		}
		return AcceptanceReceipt{}, fmt.Errorf("%w: %s", ErrIssueAmendmentBlocked, sourceIdentity)
	}
	order := admission.Order
	order.Channel = ChannelIssueScan
	order.TargetRepository = admission.Repository
	order.SourceReferences = []SourceReference{{Kind: "github_issue", Identity: sourceIdentity, URI: "https://github.com/" + admission.Repository + "/issues/" + fmt.Sprint(admission.IssueNumber), SHA256: sourceSHA}}
	order.ResolvedIssues = []ResolvedIssue{{
		Repository: admission.Repository,
		Number:     admission.IssueNumber,
		Title:      strings.TrimSpace(admission.Title),
		URI:        "https://github.com/" + admission.Repository + "/issues/" + fmt.Sprint(admission.IssueNumber),
	}}
	return i.accept(ctx, order, AcceptOptions{SourceIdentity: sourceIdentity, SourceEventIDs: []string{admission.LaunchEventID}, ActorID: admission.ActorID})
}

type IdeaInput struct {
	IdeaID    string
	Note      string
	Candidate FactoryOrder
	ActorID   string
}

type IdeaCandidate struct {
	IdeaID           string         `json:"idea_id"`
	Title            string         `json:"title"`
	TargetRepository string         `json:"target_repository"`
	Status           string         `json:"status"`
	CurrentRevision  int            `json:"current_revision"`
	Revisions        []IdeaRevision `json:"revisions"`
	Candidate        FactoryOrder   `json:"candidate"`
	ValidationErrors []string       `json:"validation_errors"`
}

type IdeaRevision struct {
	Revision         int          `json:"revision"`
	Note             string       `json:"note"`
	Candidate        FactoryOrder `json:"candidate"`
	CandidateSHA256  string       `json:"candidate_sha256,omitempty"`
	ValidationErrors []string     `json:"validation_errors"`
	EventID          string       `json:"event_id"`
	RecordedAt       time.Time    `json:"recorded_at"`
}

func (i *Intake) RecordIdea(ctx context.Context, input IdeaInput) (IdeaCandidate, error) {
	if strings.TrimSpace(input.IdeaID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return IdeaCandidate{}, errors.New("idea ID and Human actor ID are required")
	}
	if _, found, err := i.idea(ctx, input.IdeaID); err != nil {
		return IdeaCandidate{}, err
	} else if found {
		return IdeaCandidate{}, errors.New("idea already exists; use refinement")
	}
	input.Candidate.Channel = ChannelHumanIdea
	return i.appendIdeaRevision(ctx, EventIdeaRecorded, input.IdeaID, 1, input.Note, input.Candidate, input.ActorID, nil)
}

func (i *Intake) RefineIdea(ctx context.Context, input IdeaInput) (IdeaCandidate, error) {
	idea, found, err := i.idea(ctx, input.IdeaID)
	if err != nil {
		return IdeaCandidate{}, err
	}
	if !found {
		return IdeaCandidate{}, ErrIdeaNotFound
	}
	if idea.Status == "submitted" {
		return IdeaCandidate{}, errors.New("submitted idea is immutable")
	}
	input.Candidate.Channel = ChannelHumanIdea
	return i.appendIdeaRevision(ctx, EventIdeaRefined, input.IdeaID, idea.CurrentRevision+1, input.Note, input.Candidate, input.ActorID, []string{idea.Revisions[len(idea.Revisions)-1].EventID})
}

func (i *Intake) SubmitIdea(ctx context.Context, ideaID string, approved bool, actorID, credentialKeyID string, approval *HumanApprovalReceipt) (AcceptanceReceipt, error) {
	idea, found, err := i.idea(ctx, ideaID)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	if !found || len(idea.Revisions) == 0 {
		return AcceptanceReceipt{}, ErrIdeaNotFound
	}
	last := idea.Revisions[len(idea.Revisions)-1]
	return i.SubmitIdeaExact(ctx, ideaID, last.Revision, last.CandidateSHA256, approved, actorID, credentialKeyID, approval)
}

// SubmitIdeaExact rejects a stale browser approval unless it names the exact
// current revision and canonical FactoryOrder document hash the Human saw.
func (i *Intake) SubmitIdeaExact(ctx context.Context, ideaID string, expectedRevision int, expectedDocumentSHA256 string, approved bool, actorID, credentialKeyID string, approval *HumanApprovalReceipt) (AcceptanceReceipt, error) {
	if !approved {
		return AcceptanceReceipt{}, ErrIdeaApprovalRequired
	}
	idea, found, err := i.idea(ctx, ideaID)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	if !found {
		return AcceptanceReceipt{}, ErrIdeaNotFound
	}
	if len(idea.ValidationErrors) != 0 {
		return AcceptanceReceipt{}, &ValidationError{Fields: append([]string(nil), idea.ValidationErrors...)}
	}
	last := idea.Revisions[len(idea.Revisions)-1]
	if expectedRevision != last.Revision || expectedDocumentSHA256 == "" || expectedDocumentSHA256 != last.CandidateSHA256 {
		return AcceptanceReceipt{}, errors.New("idea approval does not bind the current canonical revision")
	}
	return i.accept(ctx, idea.Candidate, AcceptOptions{
		SourceIdentity: "human-idea:" + ideaID, SourceEventIDs: []string{last.EventID},
		ActorID: actorID, CredentialKeyID: credentialKeyID, Approval: approval,
	})
}

func (i *Intake) appendIdeaRevision(ctx context.Context, eventType EventType, ideaID string, revision int, note string, candidate FactoryOrder, actorID string, causes []string) (IdeaCandidate, error) {
	validation := validationFields(ValidateFactoryOrder(candidate))
	candidateSHA256 := ""
	if len(validation) == 0 {
		document, err := Canonicalize(candidate)
		if err != nil {
			return IdeaCandidate{}, err
		}
		candidateSHA256 = document.SHA256
	}
	payload := IdeaRevisionPayload{IdeaID: ideaID, Revision: revision, Note: note, Candidate: candidate, CandidateSHA256: candidateSHA256, ValidationErrors: validation, ActorID: actorID}
	event, err := AppendTyped(ctx, i.store, eventType, "", fmt.Sprintf("idea:%s:revision:%d", ideaID, revision), causes, payload)
	if err != nil {
		return IdeaCandidate{}, err
	}
	result, _, err := i.idea(ctx, ideaID)
	if err != nil {
		return IdeaCandidate{}, err
	}
	if len(result.Revisions) == 0 || result.Revisions[len(result.Revisions)-1].EventID != event.ID {
		return IdeaCandidate{}, errors.New("idea revision replay did not observe appended event")
	}
	return result, nil
}

func validationFields(err error) []string {
	if err == nil {
		return nil
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		return append([]string(nil), validation.Fields...)
	}
	return []string{err.Error()}
}

func (i *Intake) idea(ctx context.Context, ideaID string) (IdeaCandidate, bool, error) {
	events, err := i.store.List(ctx)
	if err != nil {
		return IdeaCandidate{}, false, err
	}
	var revisions []IdeaRevision
	for _, event := range eventsByTime(events) {
		if event.Type != EventIdeaRecorded && event.Type != EventIdeaRefined {
			continue
		}
		payload, decodeErr := decodeEvent[IdeaRevisionPayload](event)
		if decodeErr != nil {
			return IdeaCandidate{}, false, decodeErr
		}
		if payload.IdeaID == ideaID {
			revisions = append(revisions, IdeaRevision{Revision: payload.Revision, Note: payload.Note, Candidate: payload.Candidate, CandidateSHA256: payload.CandidateSHA256, ValidationErrors: payload.ValidationErrors, EventID: event.ID, RecordedAt: event.OccurredAt})
		}
	}
	if len(revisions) == 0 {
		return IdeaCandidate{}, false, nil
	}
	sort.Slice(revisions, func(a, b int) bool { return revisions[a].Revision < revisions[b].Revision })
	current := revisions[len(revisions)-1]
	status := "refining"
	for _, event := range events {
		if event.Type != EventOrderAccepted {
			continue
		}
		payload, decodeErr := decodeEvent[OrderAcceptedPayload](event)
		if decodeErr != nil {
			return IdeaCandidate{}, false, decodeErr
		}
		if payload.SourceIdentity == "human-idea:"+ideaID {
			status = "submitted"
		}
	}
	return IdeaCandidate{IdeaID: ideaID, Title: current.Candidate.Title, TargetRepository: current.Candidate.TargetRepository, Status: status, CurrentRevision: current.Revision, Revisions: revisions, Candidate: current.Candidate, ValidationErrors: append([]string(nil), current.ValidationErrors...)}, true, nil
}

type acceptedRecord struct {
	Event    Event
	Payload  OrderAcceptedPayload
	Document CanonicalDocument
}

func (i *Intake) accept(ctx context.Context, order FactoryOrder, options AcceptOptions) (AcceptanceReceipt, error) {
	document, err := Canonicalize(order)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	if options.SourceIdentity == "" || len(options.SourceEventIDs) == 0 || options.ActorID == "" {
		return AcceptanceReceipt{}, errors.New("acceptance source identity, causal source event, and actor are required")
	}
	if options.Approval != nil {
		if err := ValidateApprovalReceipt(document, *options.Approval); err != nil {
			return AcceptanceReceipt{}, err
		}
	}
	existing, found, err := findAccepted(ctx, i.store, order.DocID, order.Version)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	if found {
		if existing.Document.SHA256 != document.SHA256 {
			return AcceptanceReceipt{}, fmt.Errorf("%w: %s@%s", ErrAcceptedTupleConflict, order.DocID, order.Version)
		}
		return i.repairReceipt(ctx, existing)
	}
	payload := OrderAcceptedPayload{
		Document: document, SourceIdentity: options.SourceIdentity,
		SourceEventIDs:    append([]string(nil), options.SourceEventIDs...),
		AcceptedByActorID: options.ActorID, CredentialKeyID: options.CredentialKeyID,
		WorkSeedIdempotencyID: "factory-v1-work:" + order.DocID + "@" + order.Version + ":" + document.SHA256,
	}
	if options.Approval != nil {
		payload.HumanApprovalBasis = options.Approval.Basis
		copy := *options.Approval
		payload.HumanApprovalReceipt = &copy
	}
	event, err := AppendTyped(ctx, i.store, EventOrderAccepted, order.DocID,
		"accepted:"+order.DocID+"@"+order.Version, options.SourceEventIDs, payload)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	// The accepted event is deliberately committed before this Work side effect.
	work, err := i.seedWork(ctx, event, payload)
	if err != nil {
		return AcceptanceReceipt{}, fmt.Errorf("accepted event %s committed before Work seed failed: %w", event.ID, err)
	}
	return AcceptanceReceipt{OrderID: order.DocID, Version: order.Version, Channel: order.Channel, DocumentSHA256: document.SHA256, AcceptedEventID: event.ID, Work: work, Document: document, Approval: payload.HumanApprovalReceipt}, nil
}

func (i *Intake) repairReceipt(ctx context.Context, record acceptedRecord) (AcceptanceReceipt, error) {
	work, err := i.seedWork(ctx, record.Event, record.Payload)
	if err != nil {
		return AcceptanceReceipt{}, err
	}
	return AcceptanceReceipt{OrderID: record.Document.Order.DocID, Version: record.Document.Order.Version, Channel: record.Document.Order.Channel, DocumentSHA256: record.Document.SHA256, AcceptedEventID: record.Event.ID, Work: work, Document: record.Document, Approval: record.Payload.HumanApprovalReceipt}, nil
}

func (i *Intake) seedWork(ctx context.Context, event Event, payload OrderAcceptedPayload) (WorkLink, error) {
	link, err := i.work.GetFactoryOrder(ctx, payload.Document.Order.DocID, payload.Document.Order.Version)
	if err == nil {
		if link.DocumentSHA256 != payload.Document.SHA256 || link.AcceptedEventID != event.ID || link.Quarantined {
			return WorkLink{}, ErrAcceptedTupleConflict
		}
		return link, nil
	}
	if !errors.Is(err, ErrWorkNotFound) {
		return WorkLink{}, err
	}
	sourceSHA := payload.Document.Order.SourceReferences[0].SHA256
	return i.work.SeedFactoryOrder(ctx, WorkSeed{
		OrderID: payload.Document.Order.DocID, Version: payload.Document.Order.Version,
		DocumentSHA256: payload.Document.SHA256, Markdown: payload.Document.Markdown,
		SourceSHA256: sourceSHA, AcceptedEventID: event.ID, IdempotencyKey: payload.WorkSeedIdempotencyID,
		Metadata: map[string]string{"channel": string(payload.Document.Order.Channel), "source_identity": payload.SourceIdentity},
	})
}

func findAccepted(ctx context.Context, store Store, orderID, version string) (acceptedRecord, bool, error) {
	events, err := store.List(ctx)
	if err != nil {
		return acceptedRecord{}, false, err
	}
	var found acceptedRecord
	for _, event := range events {
		if event.Type != EventOrderAccepted {
			continue
		}
		payload, decodeErr := decodeEvent[OrderAcceptedPayload](event)
		if decodeErr != nil {
			return acceptedRecord{}, false, decodeErr
		}
		if payload.Document.Order.DocID == orderID && payload.Document.Order.Version == version {
			if found.Event.ID != "" && found.Document.SHA256 != payload.Document.SHA256 {
				return acceptedRecord{}, false, ErrAcceptedTupleConflict
			}
			found = acceptedRecord{Event: event, Payload: payload, Document: payload.Document}
		}
	}
	return found, found.Event.ID != "", nil
}

func (i *Intake) ReplayAndRepair(ctx context.Context) error {
	events, err := i.store.List(ctx)
	if err != nil {
		return err
	}
	acceptedIDs := make(map[string]acceptedRecord)
	for _, event := range events {
		if event.Type != EventOrderAccepted {
			continue
		}
		payload, decodeErr := decodeEvent[OrderAcceptedPayload](event)
		if decodeErr != nil {
			return decodeErr
		}
		acceptedIDs[event.ID] = acceptedRecord{Event: event, Payload: payload, Document: payload.Document}
		if _, seedErr := i.seedWork(ctx, event, payload); seedErr != nil {
			if !errors.Is(seedErr, ErrAcceptedTupleConflict) {
				return seedErr
			}
			if err := i.containAcceptedTupleConflict(ctx, event, payload); err != nil {
				return err
			}
		}
	}
	links, err := i.work.ListFactoryOrders(ctx)
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.Quarantined {
			continue
		}
		if accepted, exists := acceptedIDs[link.AcceptedEventID]; exists &&
			accepted.Document.Order.DocID == link.OrderID && accepted.Document.Order.Version == link.Version &&
			accepted.Document.SHA256 == link.DocumentSHA256 && !link.Quarantined {
			continue
		}
		reason := "Work FactoryOrder has no matching accepted EventGraph event"
		if err := i.work.QuarantineFactoryOrder(ctx, link, reason); err != nil {
			return err
		}
		if _, err := i.requestIntervention(ctx, link.OrderID, StageIngestWork, "orphan_work", reason, "", nil); err != nil {
			return err
		}
	}
	return nil
}

func (i *Intake) containAcceptedTupleConflict(ctx context.Context, acceptedEvent Event, payload OrderAcceptedPayload) error {
	order := payload.Document.Order
	link, err := i.work.GetFactoryOrder(ctx, order.DocID, order.Version)
	if err != nil {
		return fmt.Errorf("load conflicting Work FactoryOrder %s@%s: %w", order.DocID, order.Version, err)
	}
	reason := "Work FactoryOrder conflicts with accepted EventGraph tuple"
	if err := i.work.QuarantineFactoryOrder(ctx, link, reason); err != nil {
		return fmt.Errorf("quarantine conflicting Work FactoryOrder %s@%s: %w", order.DocID, order.Version, err)
	}
	if _, err := i.requestIntervention(ctx, order.DocID, StageIngestWork, "accepted_tuple_conflict", reason, "", []string{acceptedEvent.ID}); err != nil {
		return fmt.Errorf("request intervention for conflicting Work FactoryOrder %s@%s: %w", order.DocID, order.Version, err)
	}
	return nil
}

func (i *Intake) activeIssueClaim(ctx context.Context, sourceIdentity string) (acceptedRecord, bool, error) {
	events, err := i.store.List(ctx)
	if err != nil {
		return acceptedRecord{}, false, err
	}
	for _, event := range events {
		if event.Type != EventOrderAccepted {
			continue
		}
		payload, decodeErr := decodeEvent[OrderAcceptedPayload](event)
		if decodeErr != nil {
			return acceptedRecord{}, false, decodeErr
		}
		if payload.SourceIdentity == sourceIdentity && !orderAtHumanReview(events, payload.Document.Order.DocID) {
			return acceptedRecord{Event: event, Payload: payload, Document: payload.Document}, true, nil
		}
	}
	return acceptedRecord{}, false, nil
}

func orderAtHumanReview(events []Event, orderID string) bool {
	for _, event := range events {
		if event.Type != EventStageTransitioned || event.OrderID != orderID {
			continue
		}
		transition, err := decodeEvent[StageTransitionPayload](event)
		if err == nil && transition.Stage == StageHumanReview && (transition.State == TransitionHumanRequired || transition.State == TransitionPassed) {
			return true
		}
	}
	return false
}

func (i *Intake) requestIntervention(ctx context.Context, orderID string, stage Stage, kind, prompt, attemptID string, causes []string) (Event, error) {
	if orderID == "" || StageIndex(stage) < 0 || kind == "" || prompt == "" {
		return Event{}, errors.New("bounded intervention request fields are required")
	}
	id := "intervention-" + HashText(orderID + "\x00" + string(stage) + "\x00" + kind + "\x00" + attemptID)[:24]
	events, err := i.store.List(ctx)
	if err != nil {
		return Event{}, err
	}
	for _, event := range events {
		if event.Type != EventInterventionRequested {
			continue
		}
		existing, decodeErr := decodeEvent[InterventionRequestedPayload](event)
		if decodeErr != nil {
			return Event{}, decodeErr
		}
		if existing.InterventionID != id {
			continue
		}
		if existing.OrderID == orderID && existing.Stage == stage && existing.Kind == kind && existing.Prompt == prompt && existing.AttemptID == attemptID {
			return event, nil
		}
		return Event{}, fmt.Errorf("%w: intervention identity %s already has different logical content", ErrIdempotencyConflict, id)
	}
	payload := InterventionRequestedPayload{InterventionID: id, OrderID: orderID, Kind: kind, Prompt: prompt, Stage: stage, AttemptID: attemptID, RequestedAt: i.clock.Now().UTC()}
	return AppendTyped(ctx, i.store, EventInterventionRequested, orderID, "intervention-requested:"+id, causes, payload)
}
