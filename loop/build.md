# Build Report — Iterations 37-39

## Iteration 37: Conversation List Preview

- `ConversationSummary` struct wraps Node with `LastAuthor`, `LastAuthorKind`, `LastBody`
- `ListConversations` enhanced with `LEFT JOIN LATERAL` subquery for last message
- Template shows `Author: snippet...` under each conversation card, agent authors in violet
- `truncate()` helper added for body truncation

## Iteration 38: Discover Social Proof

- `SpaceWithStats` extended with `MemberCount` and `HasAgent` fields
- `ListPublicSpaces` enhanced with second LATERAL: `COUNT(DISTINCT actor)` + `BOOL_OR(kind='agent')` from ops JOIN users
- Discover cards show contributor count and violet "agents" indicator dot
- Handler mapping in main.go updated to pass new fields

## Iteration 39: Agent Picker

- `ListAgentNames()` store method queries `users WHERE kind = 'agent'`
- `ConversationsView` takes `agents []string` parameter
- Quick-add chips: violet pills below participants field, one per agent user
- `addParticipant(name)` templ script component — click to append to comma-separated input
- Deduplication: won't add same name twice

## Files Changed

- `site/graph/store.go` — ConversationSummary, LATERAL subqueries, ListAgentNames
- `site/graph/handlers.go` — pass agents to ConversationsView
- `site/graph/views.templ` — conversation preview, agent picker chips, addParticipant script
- `site/views/discover.templ` — member count, agent indicator
- `site/cmd/site/main.go` — pass MemberCount/HasAgent to DiscoverSpace
