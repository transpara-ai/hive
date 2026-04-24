package localapi

// Node state values stored in the `state` column.
const (
	NodeStateOpen      = "open"
	NodeStateDone      = "done"
	NodeStateClosed    = "closed"
	NodeStateEscalated = "escalated"
)

// Op values accepted by POST /app/{slug}/op.
const (
	OpIntend   = "intend"
	OpComplete = "complete"
	OpOpen     = "open"
	OpEdit     = "edit"
	OpClaim    = "claim"
	OpAssign   = "assign"
	OpRespond  = "respond"
	OpExpress  = "express"
	OpAssert   = "assert"
	OpDiscuss  = "discuss"
)
