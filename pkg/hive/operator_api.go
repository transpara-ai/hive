package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

// maxDecisionBodyBytes caps the POST /api/hive/operator-decision request body
// at 64 KiB to prevent large-body resource exhaustion.
const maxDecisionBodyBytes = 64 * 1024

// operatorServerOptions collects optional dependencies for the operator API.
// The zero value yields a strictly read-only server (today's behavior).
type operatorServerOptions struct {
	writer            *operatorDecisionWriter
	runWriter         *operatorRunLaunchWriter
	modelPolicyWriter *operatorModelRolePolicyWriter
	projectionOptions []OperatorProjectionOption
	modelSelection    OperatorModelSelectionSource
}

// OperatorServerOption configures NewOperatorProjectionServer.
type OperatorServerOption func(*operatorServerOptions)

// operatorDecisionWriter carries the EventGraph write primitives the operator
// decision endpoint needs. Hive remains the sole graph writer: Site only POSTs
// the human's choice, and this writer (provisioned by the hive ops-api process)
// performs the governed append. It is nil when WithOperatorDecisionWriter is NOT
// passed; in that case the POST route is not registered and the server stays
// read-only.
type operatorDecisionWriter struct {
	factory *event.EventFactory
	signer  event.Signer
	human   types.ActorID
	conv    types.ConversationID
}

// WithOperatorDecisionWriter enables the POST /api/hive/operator-decision route,
// supplying the factory/signer/human/conversation used to append the governed
// authority.decision.recorded event. Omit it to keep the server read-only.
func WithOperatorDecisionWriter(factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID) OperatorServerOption {
	return func(o *operatorServerOptions) {
		o.writer = &operatorDecisionWriter{factory: factory, signer: signer, human: human, conv: conv}
	}
}

// WithOperatorProjectionModelSelection configures the read-only model-selection
// slice returned by GET /api/hive/operator-projection.
func WithOperatorProjectionModelSelection(config OperatorModelSelectionConfig) OperatorServerOption {
	return func(o *operatorServerOptions) {
		o.projectionOptions = append(o.projectionOptions, WithOperatorModelSelection(config))
		o.modelSelection = func() OperatorModelSelectionConfig { return config }
	}
}

// WithOperatorProjectionModelSelectionSource configures the read-only
// model-selection projection from a dynamic Hive-owned source.
func WithOperatorProjectionModelSelectionSource(source OperatorModelSelectionSource) OperatorServerOption {
	return func(o *operatorServerOptions) {
		o.projectionOptions = append(o.projectionOptions, WithOperatorModelSelectionSource(source))
		o.modelSelection = source
	}
}

// operatorDecisionRequest is the Site -> hive payload for recording the human's
// authority decision on a pending request.
type operatorDecisionRequest struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
	Approver  string `json:"approver"`
	Reason    string `json:"reason"`
}

// operatorDecisionResponse echoes the recorded decision event id.
type operatorDecisionResponse struct {
	DecisionEventID string `json:"decision_event_id"`
	RequestID       string `json:"request_id"`
	Outcome         string `json:"outcome"`
}

// NewOperatorProjectionServer returns the HTTP API for Site operator
// projections. By default it is read-only — it exposes derived EventGraph state
// only. Passing writer options additionally registers bearer-protected POST
// routes that record operator decisions or queued run launches; the graph itself
// is only ever written by hive (this process), never by Site.
func NewOperatorProjectionServer(s store.Store, apiKey string, limit int, opts ...OperatorServerOption) http.Handler {
	options := operatorServerOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/hive/operator-projection", func(w http.ResponseWriter, r *http.Request) {
		if !operatorBearerOK(apiKey, r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeOperatorProjectionJSON(w, BuildOperatorProjection(s, limit, options.projectionOptions...))
	})
	mux.HandleFunc("GET /api/hive/civilization/assembly-projection", func(w http.ResponseWriter, r *http.Request) {
		if !operatorBearerOK(apiKey, r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeOperatorProjectionJSON(w, BuildCivilizationAssemblyProjection(s, limit, options.projectionOptions...))
	})
	if options.writer != nil {
		writer := options.writer
		mux.HandleFunc("POST /api/hive/operator-decision", func(w http.ResponseWriter, r *http.Request) {
			if !operatorBearerOK(apiKey, r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			handleOperatorDecision(w, r, s, limit, writer)
		})
	}
	if options.runWriter != nil {
		writer := options.runWriter
		modelSelection := modelSelectionSourceWithRolePolicyUpdates(s, options.modelSelection, limit)
		mux.HandleFunc("POST /api/hive/runs", func(w http.ResponseWriter, r *http.Request) {
			if !operatorBearerOK(apiKey, r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			handleOperatorRunLaunch(w, r, s, writer, modelSelection)
		})
	}
	if options.modelPolicyWriter != nil {
		writer := options.modelPolicyWriter
		mux.HandleFunc("POST /api/hive/model-selection/role-policy", func(w http.ResponseWriter, r *http.Request) {
			if !operatorBearerOK(apiKey, r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			handleOperatorModelRolePolicyUpdate(w, r, s, writer, options.modelSelection)
		})
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// operatorBearerOK applies the same bearer scheme to every protected route: when
// an API key is configured, the request must present "Bearer <key>".
func operatorBearerOK(apiKey string, r *http.Request) bool {
	if apiKey == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+apiKey
}

// handleOperatorDecision records the human's authority decision against a pending
// request. It loads the referenced authority.request.recorded to carry the
// request's action/target/scope onto the decision (so the projection joins them
// and drops the request from PendingApprovals via the shared RequestID), then
// appends the governed authority.decision.recorded event.
func handleOperatorDecision(w http.ResponseWriter, r *http.Request, s store.Store, limit int, writer *operatorDecisionWriter) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDecisionBodyBytes)
	var body operatorDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusBadRequest)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.RequestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}
	outcome, ok := operatorDecisionOutcome(body.Decision)
	if !ok {
		http.Error(w, fmt.Sprintf("unsupported decision %q (want approved|denied)", body.Decision), http.StatusBadRequest)
		return
	}

	request, found, err := findAuthorityRequestByID(s, body.RequestID, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("load authority request: %v", err), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, fmt.Sprintf("no pending authority request %q", body.RequestID), http.StatusNotFound)
		return
	}

	// P1-a: this endpoint decides ONLY the draft-PR create action. Refuse any
	// other pending protected action — and any request whose scope is not a
	// valid draft-PR scope — so a decision recorded here can never approve, e.g.,
	// an agent-spawn or deploy request. This mirrors the Site render gate.
	if request.ActionName != string(safety.ActionRepoPullRequestCreate) {
		http.Error(w, fmt.Sprintf("request %q action %q is not %q: this endpoint only decides draft-PR creation", body.RequestID, request.ActionName, safety.ActionRepoPullRequestCreate), http.StatusForbidden)
		return
	}
	if _, err := ParseDraftPRScope(request.Scope); err != nil {
		http.Error(w, fmt.Sprintf("request %q scope is not a valid draft-PR scope: %v", body.RequestID, err), http.StatusForbidden)
		return
	}

	// P2-a: a request is decided exactly once. If a decision was already recorded
	// for it, refuse with 409 rather than appending a rival decision — otherwise a
	// later approval could silently overwrite (shadow) an earlier denial, because
	// the decision read path resolves duplicates latest-wins. The graph stays
	// append-only; we simply never record a second decision for the same request.
	if _, decided, err := findAuthorityDecisionByRequestID(s, body.RequestID, limit); err != nil {
		http.Error(w, fmt.Sprintf("check existing decision: %v", err), http.StatusInternalServerError)
		return
	} else if decided {
		http.Error(w, fmt.Sprintf("request %q already has a recorded decision", body.RequestID), http.StatusConflict)
		return
	}

	approver := writer.human
	if body.Approver != "" {
		// Require the actor_ prefix that all actor IDs in this system carry.
		// types.NewActorID only rejects empty; this check enforces the
		// observable format convention so a junk approver string is rejected
		// with a 400 rather than silently recorded on a governance event.
		if !strings.HasPrefix(body.Approver, "actor_") {
			http.Error(w, fmt.Sprintf("invalid approver %q: must begin with actor_", body.Approver), http.StatusBadRequest)
			return
		}
		parsed, perr := types.NewActorID(body.Approver)
		if perr != nil {
			http.Error(w, fmt.Sprintf("invalid approver %q: %v", body.Approver, perr), http.StatusBadRequest)
			return
		}
		approver = parsed
	}

	content := AuthorityDecisionRecordedContent{
		DecisionID:     request.RequestID.Value(),
		RequestID:      request.RequestID,
		ApproverActor:  approver,
		DeciderRole:    "human",
		Outcome:        outcome,
		ApprovedTarget: request.Target,
		ApprovedAction: request.ActionName,
		Scope:          append([]string(nil), request.Scope...),
		Rationale:      body.Reason,
	}
	decisionID, err := appendAuthorityDecisionRecorded(s, writer.factory, writer.signer, writer.human, writer.conv, request.RequestID, content)
	if err != nil {
		http.Error(w, fmt.Sprintf("record decision: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(operatorDecisionResponse{
		DecisionEventID: decisionID.Value(),
		RequestID:       request.RequestID.Value(),
		Outcome:         outcome,
	})
}

// operatorDecisionOutcome maps the Site decision verb to the authority outcome
// vocabulary the projection reports verbatim ("approved" / "denied").
func operatorDecisionOutcome(decision string) (string, bool) {
	switch decision {
	case "approved", "approve":
		return "approved", true
	case "denied", "deny", "rejected", "reject":
		return "denied", true
	default:
		return "", false
	}
}

// findAuthorityRequestByID scans authority.request.recorded events for one whose
// content RequestID matches id. That RequestID is the key BuildOperatorProjection
// uses to pair requests with decisions.
func findAuthorityRequestByID(s store.Store, id string, limit int) (AuthorityRequestRecordedContent, bool, error) {
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeAuthorityRequestRecorded, limit, cursor)
		if err != nil {
			return AuthorityRequestRecordedContent{}, false, err
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(AuthorityRequestRecordedContent)
			if ok && content.RequestID.Value() == id {
				return content, true, nil
			}
		}
		if !page.HasMore() {
			return AuthorityRequestRecordedContent{}, false, nil
		}
		cursor = page.Cursor()
	}
}

func writeOperatorProjectionJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(v)
}
