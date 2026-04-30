package social

import (
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// Social Graph event types — Layer 3 of the thirteen-product roadmap.
var (
	EventTypePostCreated     = types.MustEventType("social.post.created")
	EventTypeFollowCreated   = types.MustEventType("social.follow.created")
	EventTypeReactionCreated = types.MustEventType("social.reaction.created")
)

// allSocialEventTypes returns all social event types for registration.
func allSocialEventTypes() []types.EventType {
	return []types.EventType{
		EventTypePostCreated,
		EventTypeFollowCreated,
		EventTypeReactionCreated,
	}
}

// socialContent is embedded in all social content types. Social events use
// no-op Accept (same pattern as work and market content) since they are hive-specific.
type socialContent struct{}

func (socialContent) Accept(event.EventContentVisitor) {}

// --- Content structs ---

// PostCreatedContent is emitted when an actor creates a new post.
type PostCreatedContent struct {
	socialContent
	Author    types.ActorID `json:"Author"`
	Body      string        `json:"Body"`
	Tags      []string      `json:"Tags,omitempty"`
	Workspace string        `json:"Workspace,omitempty"`
}

func (c PostCreatedContent) EventTypeName() string { return "social.post.created" }

// FollowCreatedContent is emitted when an actor follows another actor.
type FollowCreatedContent struct {
	socialContent
	Follower  types.ActorID `json:"Follower"`
	Subject   types.ActorID `json:"Subject"`
	Workspace string        `json:"Workspace,omitempty"`
}

func (c FollowCreatedContent) EventTypeName() string { return "social.follow.created" }

// ReactionCreatedContent is emitted when an actor reacts to a post.
type ReactionCreatedContent struct {
	socialContent
	Actor     types.ActorID `json:"Actor"`
	PostID    types.EventID `json:"PostID"`
	Reaction  string        `json:"Reaction"`
	Workspace string        `json:"Workspace,omitempty"`
}

func (c ReactionCreatedContent) EventTypeName() string { return "social.reaction.created" }

// RegisterEventTypes registers social content unmarshalers for Postgres
// deserialization. Call this before querying social events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("social.post.created", event.Unmarshal[PostCreatedContent])
	event.RegisterContentUnmarshaler("social.follow.created", event.Unmarshal[FollowCreatedContent])
	event.RegisterContentUnmarshaler("social.reaction.created", event.Unmarshal[ReactionCreatedContent])
}

// RegisterWithRegistry registers all social event types with the given registry
// and registers content unmarshalers for Postgres deserialization.
func RegisterWithRegistry(registry *event.EventTypeRegistry) {
	for _, et := range allSocialEventTypes() {
		registry.Register(et, nil)
	}
	RegisterEventTypes()
}
