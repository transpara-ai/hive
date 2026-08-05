package factoryv1

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FactoryOrderBlobSHA names the Human-reviewed source blob. Existing governed
// packets use Git SHA-1 object identities while newer callers may provide a
// SHA-256 digest. Both are content-addressed bindings; source_sha256 and the
// canonical document hash remain strictly SHA-256-only.
var factoryOrderBlobHashPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type ApprovalBasis string

const (
	ApprovalStandingScoped ApprovalBasis = "standing_scoped"
	ApprovalFreshScoped    ApprovalBasis = "fresh_scoped"
)

type HumanApprovalReceipt struct {
	Basis                 ApprovalBasis `json:"basis"`
	ActorID               string        `json:"actor_id"`
	CredentialKeyID       string        `json:"credential_key_id"`
	SourceSHA256          string        `json:"source_sha256"`
	FactoryOrderBlobSHA   string        `json:"factory_order_blob_sha"`
	OrderID               string        `json:"order_id"`
	OrderVersion          string        `json:"order_version"`
	DocumentSHA256        string        `json:"document_sha256"`
	ApprovalSentence      string        `json:"approval_sentence"`
	ApprovalSourceEventID string        `json:"approval_source_event_id"`
	IssuedAt              time.Time     `json:"issued_at"`
}

type StandingApprovalBinding struct {
	ActorID               string
	CredentialKeyID       string
	SourceSHA256          string
	FactoryOrderBlobSHA   string
	ApprovalSentence      string
	ApprovalSourceEventID string
}

func (binding StandingApprovalBinding) Bind(document CanonicalDocument, issuedAt time.Time) (HumanApprovalReceipt, error) {
	if !hexPattern.MatchString(binding.SourceSHA256) || !factoryOrderBlobHashPattern.MatchString(binding.FactoryOrderBlobSHA) {
		return HumanApprovalReceipt{}, errors.New("standing approval source and FactoryOrder blob hashes are required")
	}
	if strings.TrimSpace(binding.ActorID) == "" || strings.TrimSpace(binding.CredentialKeyID) == "" || strings.TrimSpace(binding.ApprovalSentence) == "" || strings.TrimSpace(binding.ApprovalSourceEventID) == "" {
		return HumanApprovalReceipt{}, errors.New("standing approval actor, credential key, sentence, and source event are required")
	}
	receipt := HumanApprovalReceipt{
		Basis:                 ApprovalStandingScoped,
		ActorID:               binding.ActorID,
		CredentialKeyID:       binding.CredentialKeyID,
		SourceSHA256:          binding.SourceSHA256,
		FactoryOrderBlobSHA:   binding.FactoryOrderBlobSHA,
		OrderID:               document.Order.DocID,
		OrderVersion:          document.Order.Version,
		DocumentSHA256:        document.SHA256,
		ApprovalSentence:      binding.ApprovalSentence,
		ApprovalSourceEventID: binding.ApprovalSourceEventID,
		IssuedAt:              issuedAt.UTC(),
	}
	if err := ValidateApprovalReceipt(document, receipt); err != nil {
		return HumanApprovalReceipt{}, err
	}
	return receipt, nil
}

func ValidateApprovalReceipt(document CanonicalDocument, receipt HumanApprovalReceipt) error {
	if receipt.Basis != ApprovalStandingScoped && receipt.Basis != ApprovalFreshScoped {
		return fmt.Errorf("unknown approval basis %q", receipt.Basis)
	}
	if receipt.ActorID == "" || receipt.CredentialKeyID == "" || receipt.ApprovalSourceEventID == "" || strings.TrimSpace(receipt.ApprovalSentence) == "" {
		return errors.New("approval receipt lacks Human actor, credential, source event, or sentence")
	}
	if receipt.OrderID != document.Order.DocID || receipt.OrderVersion != document.Order.Version || receipt.DocumentSHA256 != document.SHA256 {
		return errors.New("approval receipt does not bind the exact accepted order tuple")
	}
	if !hexPattern.MatchString(receipt.SourceSHA256) || !factoryOrderBlobHashPattern.MatchString(receipt.FactoryOrderBlobSHA) {
		return errors.New("approval receipt source or FactoryOrder blob hash is invalid")
	}
	if receipt.IssuedAt.IsZero() {
		return errors.New("approval receipt timestamp is required")
	}
	return nil
}

type InterventionStatus string

const (
	InterventionOpen     InterventionStatus = "open"
	InterventionResolved InterventionStatus = "resolved"
)

type InterventionRequestedPayload struct {
	InterventionID string    `json:"intervention_id"`
	OrderID        string    `json:"order_id"`
	Kind           string    `json:"kind"`
	Prompt         string    `json:"prompt"`
	Stage          Stage     `json:"stage"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	RequestedAt    time.Time `json:"requested_at"`
}

type InterventionResolvedPayload struct {
	InterventionID      string    `json:"intervention_id"`
	OrderID             string    `json:"order_id"`
	Resolution          string    `json:"resolution"`
	ActorID             string    `json:"actor_id"`
	CredentialKeyID     string    `json:"credential_key_id"`
	OperatorPrincipalID string    `json:"operator_principal_id,omitempty"`
	ResolvedAt          time.Time `json:"resolved_at"`
}

type InterventionResolution struct {
	InterventionID      string
	Resolution          string
	ActorID             string
	CredentialKeyID     string
	OperatorPrincipalID string
}

func ResolveIntervention(ctx context.Context, store Store, clock Clock, resolution InterventionResolution) (Event, error) {
	if clock == nil {
		clock = WallClock{}
	}
	if strings.TrimSpace(resolution.OperatorPrincipalID) == "" {
		resolution.OperatorPrincipalID = resolution.ActorID
	}
	if strings.TrimSpace(resolution.InterventionID) == "" || strings.TrimSpace(resolution.Resolution) == "" || strings.TrimSpace(resolution.ActorID) == "" || strings.TrimSpace(resolution.CredentialKeyID) == "" || strings.TrimSpace(resolution.OperatorPrincipalID) == "" {
		return Event{}, errors.New("bounded intervention resolution fields are required")
	}
	events, err := store.List(ctx)
	if err != nil {
		return Event{}, err
	}
	var request Event
	var payload InterventionRequestedPayload
	for _, event := range events {
		if event.Type != EventInterventionRequested {
			continue
		}
		candidate, decodeErr := decodeEvent[InterventionRequestedPayload](event)
		if decodeErr != nil {
			return Event{}, decodeErr
		}
		if candidate.InterventionID == resolution.InterventionID {
			request, payload = event, candidate
		}
	}
	if request.ID == "" {
		return Event{}, errors.New("intervention does not exist")
	}
	for _, event := range events {
		if event.Type != EventInterventionResolved {
			continue
		}
		candidate, decodeErr := decodeEvent[InterventionResolvedPayload](event)
		if decodeErr != nil {
			return Event{}, decodeErr
		}
		if candidate.InterventionID == resolution.InterventionID {
			if candidate.Resolution == resolution.Resolution && candidate.ActorID == resolution.ActorID && candidate.CredentialKeyID == resolution.CredentialKeyID && candidate.OperatorPrincipalID == resolution.OperatorPrincipalID {
				return event, nil
			}
			return Event{}, fmt.Errorf("%w: intervention %s already has a different Human resolution", ErrIdempotencyConflict, resolution.InterventionID)
		}
	}
	result := InterventionResolvedPayload{
		InterventionID:      resolution.InterventionID,
		OrderID:             payload.OrderID,
		Resolution:          resolution.Resolution,
		ActorID:             resolution.ActorID,
		CredentialKeyID:     resolution.CredentialKeyID,
		OperatorPrincipalID: resolution.OperatorPrincipalID,
		ResolvedAt:          clock.Now().UTC(),
	}
	return AppendTyped(ctx, store, EventInterventionResolved, payload.OrderID, "intervention-resolved:"+resolution.InterventionID, []string{request.ID}, result)
}
