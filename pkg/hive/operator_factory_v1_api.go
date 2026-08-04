package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const maxFactoryV1BodyBytes = 2 * 1024 * 1024

// FactoryV1OperatorService is the Hive-owned command/projection boundary used
// by Site. The bearer credential is authenticated by the parent operator
// server; this service records the immutable configured Human actor identity.
type FactoryV1OperatorService struct {
	Intake          *factoryv1.Intake
	Projector       *factoryv1.Projector
	Store           factoryv1.Store
	Clock           factoryv1.Clock
	HumanActorID    string
	CredentialKeyID string
}

func NewFactoryV1OperatorService(intake *factoryv1.Intake, projector *factoryv1.Projector, store factoryv1.Store, clock factoryv1.Clock, actorID, credentialKeyID string) (*FactoryV1OperatorService, error) {
	if intake == nil || projector == nil || store == nil || strings.TrimSpace(actorID) == "" || strings.TrimSpace(credentialKeyID) == "" {
		return nil, errors.New("factory v1 operator service requires intake, projector, store, Human actor, and credential key")
	}
	if clock == nil {
		clock = factoryv1.WallClock{}
	}
	return &FactoryV1OperatorService{Intake: intake, Projector: projector, Store: store, Clock: clock, HumanActorID: actorID, CredentialKeyID: credentialKeyID}, nil
}

func WithOperatorFactoryV1(service *FactoryV1OperatorService) OperatorServerOption {
	return func(options *operatorServerOptions) { options.factoryV1 = service }
}

func registerFactoryV1OperatorRoutes(mux *http.ServeMux, apiKey string, service *FactoryV1OperatorService) {
	protected := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !operatorBearerOK(apiKey, r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("GET /api/hive/factory/v1/projection", protected(service.handleProjection))
	mux.HandleFunc("POST /api/hive/factory/v1/ideas", protected(service.handleIdeaCreate))
	mux.HandleFunc("POST /api/hive/factory/v1/ideas/{id}/refine", protected(service.handleIdeaRefine))
	mux.HandleFunc("POST /api/hive/factory/v1/ideas/{id}/submit", protected(service.handleIdeaSubmit))
	mux.HandleFunc("POST /api/hive/factory/v1/orders", protected(service.handleCompletedOrder))
	mux.HandleFunc("POST /api/hive/factory/v1/interventions/{id}/resolve", protected(service.handleInterventionResolve))
}

func (s *FactoryV1OperatorService) handleProjection(w http.ResponseWriter, r *http.Request) {
	projection, err := s.Projector.Build(r.Context())
	if err != nil {
		http.Error(w, "factory v1 projection unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeFactoryV1JSON(w, http.StatusOK, projection)
}

type factoryV1IdeaCreateRequest struct {
	IdeaID           string                  `json:"idea_id,omitempty"`
	Title            string                  `json:"title"`
	Idea             string                  `json:"idea"`
	TargetRepository string                  `json:"target_repository"`
	Candidate        *factoryv1.FactoryOrder `json:"candidate,omitempty"`
}

func (s *FactoryV1OperatorService) handleIdeaCreate(w http.ResponseWriter, r *http.Request) {
	var request factoryV1IdeaCreateRequest
	if err := decodeFactoryV1JSON(w, r, &request); err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	if strings.TrimSpace(request.Title) == "" || strings.TrimSpace(request.Idea) == "" || strings.TrimSpace(request.TargetRepository) == "" {
		writeFactoryV1Error(w, errors.New("title, idea, and target_repository are required"))
		return
	}
	ideaID := strings.TrimSpace(request.IdeaID)
	if ideaID == "" {
		ideaID = "idea-" + factoryv1.HashText(request.TargetRepository + "\x00" + request.Title + "\x00" + request.Idea)[:24]
	}
	candidate := factoryV1IdeaOrder(ideaID, request.Title, request.Idea, request.TargetRepository, s.HumanActorID)
	if request.Candidate != nil {
		candidate = *request.Candidate
		candidate.Channel = factoryv1.ChannelHumanIdea
	}
	result, err := s.Intake.RecordIdea(r.Context(), factoryv1.IdeaInput{IdeaID: ideaID, Note: request.Idea, Candidate: candidate, ActorID: s.HumanActorID})
	if err != nil {
		// A byte-identical browser retry returns the durable current candidate.
		if strings.Contains(err.Error(), "idea already exists") {
			projection, projectErr := s.Projector.Build(r.Context())
			if projectErr == nil {
				for _, idea := range projection.Ideas {
					if idea.IdeaID == ideaID {
						writeFactoryV1JSON(w, http.StatusOK, idea)
						return
					}
				}
			}
		}
		writeFactoryV1Error(w, err)
		return
	}
	writeFactoryV1JSON(w, http.StatusCreated, result)
}

type factoryV1IdeaRefineRequest struct {
	Instruction string                  `json:"instruction"`
	Candidate   *factoryv1.FactoryOrder `json:"candidate,omitempty"`
}

func (s *FactoryV1OperatorService) handleIdeaRefine(w http.ResponseWriter, r *http.Request) {
	ideaID := strings.TrimSpace(r.PathValue("id"))
	var request factoryV1IdeaRefineRequest
	if ideaID == "" {
		writeFactoryV1Error(w, errors.New("idea id is required"))
		return
	}
	if err := decodeFactoryV1JSON(w, r, &request); err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	if strings.TrimSpace(request.Instruction) == "" {
		writeFactoryV1Error(w, errors.New("instruction is required"))
		return
	}
	projection, err := s.Projector.Build(r.Context())
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	var current *factoryv1.IdeaProjection
	for index := range projection.Ideas {
		if projection.Ideas[index].IdeaID == ideaID {
			current = &projection.Ideas[index]
			break
		}
	}
	if current == nil || len(current.Revisions) == 0 {
		writeFactoryV1Error(w, factoryv1.ErrIdeaNotFound)
		return
	}
	candidate := current.Revisions[len(current.Revisions)-1].Candidate
	if request.Candidate != nil {
		candidate = *request.Candidate
	} else {
		candidate.Requirements[0].Statement = strings.TrimSpace(candidate.Requirements[0].Statement) + "\n\nRefinement: " + strings.TrimSpace(request.Instruction)
		candidate.SourceReferences[0].SHA256 = factoryv1.HashText(candidate.SourceReferences[0].SHA256 + "\x00" + request.Instruction)
	}
	candidate.Channel = factoryv1.ChannelHumanIdea
	result, err := s.Intake.RefineIdea(r.Context(), factoryv1.IdeaInput{IdeaID: ideaID, Note: request.Instruction, Candidate: candidate, ActorID: s.HumanActorID})
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	writeFactoryV1JSON(w, http.StatusOK, result)
}

type factoryV1IdeaSubmitRequest struct {
	Approved        bool   `json:"approved"`
	Revision        int    `json:"revision"`
	CandidateSHA256 string `json:"candidate_sha256"`
}

func (s *FactoryV1OperatorService) handleIdeaSubmit(w http.ResponseWriter, r *http.Request) {
	ideaID := strings.TrimSpace(r.PathValue("id"))
	var request factoryV1IdeaSubmitRequest
	if err := decodeFactoryV1JSON(w, r, &request); err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	projection, err := s.Projector.Build(r.Context())
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	var current *factoryv1.IdeaProjection
	for index := range projection.Ideas {
		if projection.Ideas[index].IdeaID == ideaID {
			current = &projection.Ideas[index]
			break
		}
	}
	if current == nil || len(current.Revisions) == 0 {
		writeFactoryV1Error(w, factoryv1.ErrIdeaNotFound)
		return
	}
	last := current.Revisions[len(current.Revisions)-1]
	if request.Revision != last.Revision || request.CandidateSHA256 == "" || request.CandidateSHA256 != last.CandidateSHA256 {
		writeFactoryV1Error(w, errors.New("idea approval does not bind the current canonical revision"))
		return
	}
	document, err := factoryv1.Canonicalize(last.Candidate)
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	sourceSHA := document.SHA256
	if len(document.Order.SourceReferences) > 0 {
		sourceSHA = document.Order.SourceReferences[0].SHA256
	}
	approval := &factoryv1.HumanApprovalReceipt{
		Basis: factoryv1.ApprovalFreshScoped, ActorID: s.HumanActorID,
		CredentialKeyID: s.CredentialKeyID, SourceSHA256: sourceSHA,
		FactoryOrderBlobSHA: document.SHA256, OrderID: document.Order.DocID,
		OrderVersion: document.Order.Version, DocumentSHA256: document.SHA256,
		ApprovalSentence:      "Human approved idea candidate revision " + fmt.Sprint(last.Revision),
		ApprovalSourceEventID: last.EventID, IssuedAt: s.Clock.Now().UTC(),
	}
	receipt, err := s.Intake.SubmitIdeaExact(r.Context(), ideaID, request.Revision, request.CandidateSHA256, request.Approved, s.HumanActorID, s.CredentialKeyID, approval)
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	writeFactoryV1JSON(w, http.StatusCreated, receipt)
}

type factoryV1CompletedOrderRequest struct {
	FactoryOrder factoryv1.FactoryOrder `json:"factory_order"`
}

func (s *FactoryV1OperatorService) handleCompletedOrder(w http.ResponseWriter, r *http.Request) {
	var request factoryV1CompletedOrderRequest
	if err := decodeFactoryV1JSON(w, r, &request); err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	receipt, err := s.Intake.SubmitCompleted(r.Context(), request.FactoryOrder, s.HumanActorID, s.CredentialKeyID)
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	writeFactoryV1JSON(w, http.StatusCreated, receipt)
}

type factoryV1InterventionResolveRequest struct {
	Resolution          string `json:"resolution"`
	ActorID             string `json:"actor_id,omitempty"`
	OperatorPrincipalID string `json:"operator_principal_id,omitempty"`
}

func (s *FactoryV1OperatorService) handleInterventionResolve(w http.ResponseWriter, r *http.Request) {
	var request factoryV1InterventionResolveRequest
	if err := decodeFactoryV1JSON(w, r, &request); err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	actorID := strings.TrimSpace(request.ActorID)
	if actorID == "" {
		actorID = s.HumanActorID
	}
	if actorID != s.HumanActorID {
		http.Error(w, "configured Human actor mismatch", http.StatusForbidden)
		return
	}
	operatorPrincipalID := strings.TrimSpace(request.OperatorPrincipalID)
	if operatorPrincipalID == "" {
		operatorPrincipalID = actorID
	}
	event, err := factoryv1.ResolveIntervention(r.Context(), s.Store, s.Clock, factoryv1.InterventionResolution{
		InterventionID: r.PathValue("id"), Resolution: request.Resolution,
		ActorID: actorID, CredentialKeyID: s.CredentialKeyID,
		OperatorPrincipalID: operatorPrincipalID,
	})
	if err != nil {
		writeFactoryV1Error(w, err)
		return
	}
	writeFactoryV1JSON(w, http.StatusOK, map[string]string{"intervention_id": r.PathValue("id"), "resolution_event_id": event.ID})
}

func factoryV1IdeaOrder(ideaID, title, idea, repository, actorID string) factoryv1.FactoryOrder {
	sourceSHA := factoryv1.HashText(idea)
	return factoryv1.FactoryOrder{
		DocID: "FO-IDEA-" + factoryv1.HashText(ideaID)[:16], Version: "1.0.0", Status: "approved",
		Title: title, Channel: factoryv1.ChannelHumanIdea, TargetRepository: repository,
		SourceReferences:   []factoryv1.SourceReference{{Kind: "human_idea", Identity: "human-idea:" + ideaID, SHA256: sourceSHA}},
		Requirements:       []factoryv1.Requirement{{ID: "R1", Statement: idea, Rationale: "The Human supplied this outcome through the visible refinement channel."}},
		AcceptanceCriteria: []factoryv1.AcceptanceCriterion{{ID: "AC1", Statement: "The requested bounded change is present and verified.", VerificationMethod: "Repository-specific tests and exact-head PR evidence", RiskClass: "medium"}},
		TestPlan:           []string{"Run repository-prescribed verification and preserve exact output."},
		Constraints:        []string{"Non-production only", "No merge or deploy", "Preserve existing repository invariants"},
		NonGoals:           []string{"Unrelated refactors", "Authority expansion"},
		ExpectedOutputs:    []string{"Open non-draft exact-head pull request with passing checks"},
		Authority:          factoryv1.AuthorityScope{ActorID: actorID, AllowedActions: []string{"repo.branch.create", "repo.commit.create", "repo.pull_request.create", "repo.pull_request.mark_ready", "governance.review.record"}, TargetRepositories: []string{repository}, NonProductionOnly: true},
		Budget:             factoryv1.BudgetLimit{MaxAttempts: 24, MaxTokens: 2_000_000, MaxCostMicros: 100_000_000},
	}
}

func decodeFactoryV1JSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxFactoryV1BodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body too large")
		}
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("invalid JSON request: trailing value")
		}
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	return nil
}

func writeFactoryV1JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeFactoryV1Error(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, factoryv1.ErrIdempotencyConflict) || errors.Is(err, factoryv1.ErrAcceptedTupleConflict) || errors.Is(err, factoryv1.ErrIssueAmendmentBlocked) {
		status = http.StatusConflict
	}
	if errors.Is(err, factoryv1.ErrIdeaNotFound) || strings.Contains(err.Error(), "does not exist") {
		status = http.StatusNotFound
	}
	var validation *factoryv1.ValidationError
	if errors.As(err, &validation) {
		writeFactoryV1JSON(w, status, map[string]any{"error": "validation_failed", "validation_errors": validation.Fields})
		return
	}
	writeFactoryV1JSON(w, status, map[string]string{"error": err.Error()})
}
