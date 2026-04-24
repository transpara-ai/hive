package localapi

import "errors"

// ErrNotFound is returned when a mutation targets a node ID that does not
// exist in the requested space. Cross-space mismatches are surfaced as
// ErrNotFound (not a distinct error) so callers cannot probe for the
// existence of nodes outside their space.
var ErrNotFound = errors.New("node not found")

// ErrInvalidState is returned when a state-specific mutation is rejected
// because the node is not in an eligible source state (for example,
// reopening a node that is already open).
var ErrInvalidState = errors.New("invalid state for operation")
