package civilization

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

var ErrHumanOwnerConflict = errors.New("human responsibility changed; review the current assignment before saving")

// HumanOwnerAssignment is routing and accountability metadata. It grants no
// permission, confirms no brief, resolves no intervention, and executes no work.
// AssignedBy is asserted by the authenticated caller, like source identity.
type HumanOwnerAssignment struct {
	OwnerID              string `json:"owner_id"`
	AssignedBy           string `json:"assigned_by"`
	PreviousAssignmentID string `json:"previous_assignment_id"`
}

func (e *Engine) AssignHumanOwner(ctx context.Context, workID string, assignment HumanOwnerAssignment) (WorkProjection, error) {
	validID := func(id string) bool {
		return len(id) <= 256 && strings.TrimSpace(id) == id && !strings.ContainsFunc(id, unicode.IsControl)
	}
	if assignment.AssignedBy == "" || !validID(assignment.AssignedBy) || !validID(assignment.OwnerID) {
		return WorkProjection{}, errors.New("invalid responsible person or assigning operator")
	}
	// Human routing can change while a provider holds the execution lock. The
	// assignment version and store key serialize only assignment writers.
	unlock := e.lockWork("human-owner:" + workID)
	defer unlock()
	work, err := e.mustFind(ctx, workID)
	if err != nil {
		return WorkProjection{}, err
	}
	if work.HumanOwnerAssignmentID != assignment.PreviousAssignmentID {
		return work, ErrHumanOwnerConflict
	}
	if work.HumanOwnerID == assignment.OwnerID {
		return work, nil
	}
	// Competing writes from the same assignment version share one store key.
	// The store's existing idempotency conflict also protects separate callers.
	_, err = appendEvent(ctx, e.store, EventHumanOwnerAssigned, workID,
		"human-owner:"+workID+":"+assignment.PreviousAssignmentID,
		[]string{work.LatestEventID}, assignment)
	if errors.Is(err, ErrIdempotencyConflict) {
		return work, ErrHumanOwnerConflict
	}
	if err != nil {
		return WorkProjection{}, err
	}
	return e.mustFind(ctx, workID)
}
