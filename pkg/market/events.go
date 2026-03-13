package market

import (
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Market Graph event types — Layer 2 of the thirteen-product roadmap.
var (
	EventTypeEndorsement = types.MustEventType("market.reputation.endorsement")
	EventTypeReview      = types.MustEventType("market.reputation.review")
)

// allMarketEventTypes returns all market event types for registration.
func allMarketEventTypes() []types.EventType {
	return []types.EventType{
		EventTypeEndorsement,
		EventTypeReview,
	}
}

// marketContent is embedded in all market content types. Market events use
// no-op Accept (same pattern as work content) since they are hive-specific.
type marketContent struct{}

func (marketContent) Accept(event.EventContentVisitor) {}

// --- Content structs ---

// EndorsementContent is emitted when an actor endorses another actor for a skill.
type EndorsementContent struct {
	marketContent
	EndorserID types.ActorID `json:"EndorserID"`
	SubjectID  types.ActorID `json:"SubjectID"`
	Skill      string        `json:"Skill"`
}

func (c EndorsementContent) EventTypeName() string { return "market.reputation.endorsement" }

// ReviewContent is emitted when an actor reviews another actor after completing a task.
type ReviewContent struct {
	marketContent
	ReviewerID types.ActorID `json:"ReviewerID"`
	SubjectID  types.ActorID `json:"SubjectID"`
	TaskID     types.EventID `json:"TaskID"`
	Rating     int           `json:"Rating"`
	Note       string        `json:"Note,omitempty"`
}

func (c ReviewContent) EventTypeName() string { return "market.reputation.review" }

// RegisterEventTypes registers market content unmarshalers for Postgres
// deserialization. Call this before querying market events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("market.reputation.endorsement", event.Unmarshal[EndorsementContent])
	event.RegisterContentUnmarshaler("market.reputation.review", event.Unmarshal[ReviewContent])
}

// RegisterWithRegistry registers all market event types with the given registry
// and registers content unmarshalers for Postgres deserialization.
func RegisterWithRegistry(registry *event.EventTypeRegistry) {
	for _, et := range allMarketEventTypes() {
		registry.Register(et, nil)
	}
	RegisterEventTypes()
}
