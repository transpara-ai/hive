package social

import (
	"fmt"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// Post represents a social post derived from a social.post.created event.
type Post struct {
	ID        types.EventID
	Author    types.ActorID
	Body      string
	Tags      []string
	Workspace string
	Timestamp time.Time
}

// Follow represents a follow relationship derived from a social.follow.created event.
type Follow struct {
	ID        types.EventID
	Follower  types.ActorID
	Subject   types.ActorID
	Workspace string
}

// Reaction represents a reaction to a post derived from a social.reaction.created event.
type Reaction struct {
	ID        types.EventID
	Actor     types.ActorID
	PostID    types.EventID
	Reaction  string
	Workspace string
}

// SocialStore creates and queries social events as auditable events on the shared graph.
type SocialStore struct {
	store   store.Store
	factory *event.EventFactory
	signer  event.Signer
}

// NewSocialStore creates a new SocialStore backed by the given event store.
func NewSocialStore(s store.Store, factory *event.EventFactory, signer event.Signer) *SocialStore {
	return &SocialStore{store: s, factory: factory, signer: signer}
}

// CreatePost records a social.post.created event on the graph and returns the post.
func (ss *SocialStore) CreatePost(
	author types.ActorID,
	body string,
	tags []string,
	causes []types.EventID,
	convID types.ConversationID,
) (Post, error) {
	if body == "" {
		return Post{}, fmt.Errorf("body is required")
	}
	content := PostCreatedContent{
		Author: author,
		Body:   body,
		Tags:   tags,
	}
	ev, err := ss.factory.Create(EventTypePostCreated, author, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Post{}, fmt.Errorf("create post event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Post{}, fmt.Errorf("append post event: %w", err)
	}
	return Post{
		ID:        stored.ID(),
		Author:    author,
		Body:      body,
		Tags:      tags,
		Timestamp: stored.Timestamp().Value(),
	}, nil
}

// CreatePostInWorkspace records a social.post.created event scoped to a workspace.
func (ss *SocialStore) CreatePostInWorkspace(
	author types.ActorID,
	body string,
	tags []string,
	workspace string,
	causes []types.EventID,
	convID types.ConversationID,
) (Post, error) {
	if body == "" {
		return Post{}, fmt.Errorf("body is required")
	}
	if workspace == "" {
		return Post{}, fmt.Errorf("workspace is required")
	}
	content := PostCreatedContent{
		Author:    author,
		Body:      body,
		Tags:      tags,
		Workspace: workspace,
	}
	ev, err := ss.factory.Create(EventTypePostCreated, author, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Post{}, fmt.Errorf("create post event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Post{}, fmt.Errorf("append post event: %w", err)
	}
	return Post{
		ID:        stored.ID(),
		Author:    author,
		Body:      body,
		Tags:      tags,
		Workspace: workspace,
		Timestamp: stored.Timestamp().Value(),
	}, nil
}

// GetPost returns the post with the given ID, or an error if not found.
func (ss *SocialStore) GetPost(id types.EventID) (Post, error) {
	ev, err := ss.store.Get(id)
	if err != nil {
		return Post{}, fmt.Errorf("get post event: %w", err)
	}
	c, ok := ev.Content().(PostCreatedContent)
	if !ok {
		return Post{}, fmt.Errorf("event %s is not a post", id.Value())
	}
	return Post{
		ID:        ev.ID(),
		Author:    c.Author,
		Body:      c.Body,
		Tags:      c.Tags,
		Workspace: c.Workspace,
		Timestamp: ev.Timestamp().Value(),
	}, nil
}

// ListPosts returns up to limit social.post.created events as Posts.
func (ss *SocialStore) ListPosts(limit int) ([]Post, error) {
	if limit <= 0 {
		limit = 20
	}
	page, err := ss.store.ByType(EventTypePostCreated, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	posts := make([]Post, 0, len(page.Items()))
	for _, ev := range page.Items() {
		c, ok := ev.Content().(PostCreatedContent)
		if !ok {
			continue
		}
		posts = append(posts, Post{
			ID:        ev.ID(),
			Author:    c.Author,
			Body:      c.Body,
			Tags:      c.Tags,
			Workspace: c.Workspace,
			Timestamp: ev.Timestamp().Value(),
		})
	}
	return posts, nil
}

// ListPostsByWorkspace returns up to limit posts whose Workspace field matches the given workspace.
func (ss *SocialStore) ListPostsByWorkspace(workspace string, limit int) ([]Post, error) {
	if limit <= 0 {
		limit = 20
	}
	page, err := ss.store.ByType(EventTypePostCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("list posts by workspace: %w", err)
	}
	posts := make([]Post, 0)
	for _, ev := range page.Items() {
		c, ok := ev.Content().(PostCreatedContent)
		if !ok || c.Workspace != workspace {
			continue
		}
		posts = append(posts, Post{
			ID:        ev.ID(),
			Author:    c.Author,
			Body:      c.Body,
			Tags:      c.Tags,
			Workspace: c.Workspace,
			Timestamp: ev.Timestamp().Value(),
		})
		if len(posts) >= limit {
			break
		}
	}
	return posts, nil
}

// Follow records a social.follow.created event on the graph.
func (ss *SocialStore) Follow(
	follower types.ActorID,
	subject types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) (Follow, error) {
	content := FollowCreatedContent{
		Follower: follower,
		Subject:  subject,
	}
	ev, err := ss.factory.Create(EventTypeFollowCreated, follower, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Follow{}, fmt.Errorf("create follow event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Follow{}, fmt.Errorf("append follow event: %w", err)
	}
	return Follow{
		ID:       stored.ID(),
		Follower: follower,
		Subject:  subject,
	}, nil
}

// FollowInWorkspace records a social.follow.created event scoped to a workspace.
func (ss *SocialStore) FollowInWorkspace(
	follower types.ActorID,
	subject types.ActorID,
	workspace string,
	causes []types.EventID,
	convID types.ConversationID,
) (Follow, error) {
	if workspace == "" {
		return Follow{}, fmt.Errorf("workspace is required")
	}
	content := FollowCreatedContent{
		Follower:  follower,
		Subject:   subject,
		Workspace: workspace,
	}
	ev, err := ss.factory.Create(EventTypeFollowCreated, follower, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Follow{}, fmt.Errorf("create follow event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Follow{}, fmt.Errorf("append follow event: %w", err)
	}
	return Follow{
		ID:        stored.ID(),
		Follower:  follower,
		Subject:   subject,
		Workspace: workspace,
	}, nil
}

// GetFollowers returns all actor IDs that follow the given subject.
func (ss *SocialStore) GetFollowers(subject types.ActorID) ([]types.ActorID, error) {
	page, err := ss.store.ByType(EventTypeFollowCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch follow events: %w", err)
	}
	seen := make(map[types.ActorID]bool)
	var followers []types.ActorID
	for _, ev := range page.Items() {
		c, ok := ev.Content().(FollowCreatedContent)
		if !ok || c.Subject != subject {
			continue
		}
		if !seen[c.Follower] {
			seen[c.Follower] = true
			followers = append(followers, c.Follower)
		}
	}
	return followers, nil
}

// GetFollowing returns all actor IDs that the given follower follows.
func (ss *SocialStore) GetFollowing(follower types.ActorID) ([]types.ActorID, error) {
	page, err := ss.store.ByType(EventTypeFollowCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch follow events: %w", err)
	}
	seen := make(map[types.ActorID]bool)
	var following []types.ActorID
	for _, ev := range page.Items() {
		c, ok := ev.Content().(FollowCreatedContent)
		if !ok || c.Follower != follower {
			continue
		}
		if !seen[c.Subject] {
			seen[c.Subject] = true
			following = append(following, c.Subject)
		}
	}
	return following, nil
}

// GetFollowersByWorkspace returns all actor IDs that follow the given subject in the workspace.
func (ss *SocialStore) GetFollowersByWorkspace(subject types.ActorID, workspace string) ([]types.ActorID, error) {
	page, err := ss.store.ByType(EventTypeFollowCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch follow events: %w", err)
	}
	seen := make(map[types.ActorID]bool)
	var followers []types.ActorID
	for _, ev := range page.Items() {
		c, ok := ev.Content().(FollowCreatedContent)
		if !ok || c.Subject != subject || c.Workspace != workspace {
			continue
		}
		if !seen[c.Follower] {
			seen[c.Follower] = true
			followers = append(followers, c.Follower)
		}
	}
	return followers, nil
}

// GetFollowingByWorkspace returns all actor IDs that the given follower follows in the workspace.
func (ss *SocialStore) GetFollowingByWorkspace(follower types.ActorID, workspace string) ([]types.ActorID, error) {
	page, err := ss.store.ByType(EventTypeFollowCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch follow events: %w", err)
	}
	seen := make(map[types.ActorID]bool)
	var following []types.ActorID
	for _, ev := range page.Items() {
		c, ok := ev.Content().(FollowCreatedContent)
		if !ok || c.Follower != follower || c.Workspace != workspace {
			continue
		}
		if !seen[c.Subject] {
			seen[c.Subject] = true
			following = append(following, c.Subject)
		}
	}
	return following, nil
}

// AddReaction records a social.reaction.created event on the graph.
func (ss *SocialStore) AddReaction(
	actor types.ActorID,
	postID types.EventID,
	reaction string,
	causes []types.EventID,
	convID types.ConversationID,
) (Reaction, error) {
	if reaction == "" {
		return Reaction{}, fmt.Errorf("reaction is required")
	}
	content := ReactionCreatedContent{
		Actor:    actor,
		PostID:   postID,
		Reaction: reaction,
	}
	ev, err := ss.factory.Create(EventTypeReactionCreated, actor, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Reaction{}, fmt.Errorf("create reaction event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Reaction{}, fmt.Errorf("append reaction event: %w", err)
	}
	return Reaction{
		ID:       stored.ID(),
		Actor:    actor,
		PostID:   postID,
		Reaction: reaction,
	}, nil
}

// AddReactionInWorkspace records a social.reaction.created event scoped to a workspace.
func (ss *SocialStore) AddReactionInWorkspace(
	actor types.ActorID,
	postID types.EventID,
	reaction string,
	workspace string,
	causes []types.EventID,
	convID types.ConversationID,
) (Reaction, error) {
	if reaction == "" {
		return Reaction{}, fmt.Errorf("reaction is required")
	}
	if workspace == "" {
		return Reaction{}, fmt.Errorf("workspace is required")
	}
	content := ReactionCreatedContent{
		Actor:     actor,
		PostID:    postID,
		Reaction:  reaction,
		Workspace: workspace,
	}
	ev, err := ss.factory.Create(EventTypeReactionCreated, actor, content, causes, convID, ss.store, ss.signer)
	if err != nil {
		return Reaction{}, fmt.Errorf("create reaction event: %w", err)
	}
	stored, err := ss.store.Append(ev)
	if err != nil {
		return Reaction{}, fmt.Errorf("append reaction event: %w", err)
	}
	return Reaction{
		ID:        stored.ID(),
		Actor:     actor,
		PostID:    postID,
		Reaction:  reaction,
		Workspace: workspace,
	}, nil
}

// ListReactions returns all reactions for the given post.
func (ss *SocialStore) ListReactions(postID types.EventID) ([]Reaction, error) {
	page, err := ss.store.ByType(EventTypeReactionCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch reaction events: %w", err)
	}
	var reactions []Reaction
	for _, ev := range page.Items() {
		c, ok := ev.Content().(ReactionCreatedContent)
		if !ok || c.PostID != postID {
			continue
		}
		reactions = append(reactions, Reaction{
			ID:        ev.ID(),
			Actor:     c.Actor,
			PostID:    c.PostID,
			Reaction:  c.Reaction,
			Workspace: c.Workspace,
		})
	}
	return reactions, nil
}

// ListReactionsByWorkspace returns all reactions for the given post in the workspace.
func (ss *SocialStore) ListReactionsByWorkspace(postID types.EventID, workspace string) ([]Reaction, error) {
	page, err := ss.store.ByType(EventTypeReactionCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch reaction events: %w", err)
	}
	var reactions []Reaction
	for _, ev := range page.Items() {
		c, ok := ev.Content().(ReactionCreatedContent)
		if !ok || c.PostID != postID || c.Workspace != workspace {
			continue
		}
		reactions = append(reactions, Reaction{
			ID:        ev.ID(),
			Actor:     c.Actor,
			PostID:    c.PostID,
			Reaction:  c.Reaction,
			Workspace: c.Workspace,
		})
	}
	return reactions, nil
}
