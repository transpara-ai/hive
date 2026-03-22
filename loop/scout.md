# Scout Report — Iteration 34

## Map

Conversations exist as data (31), interface (32), and participant (33). The Mind can reply via `cmd/reply`. But the chat view has no live updates — when the Mind posts a response, the human won't see it until they manually reload the page. The human-agent duo can't have a conversation.

## Gap Type

Missing plumbing (live feedback loop between two working systems)

## The Gap

The conversation detail view renders messages once on page load. When a new message arrives (from the Mind via `cmd/reply`, or from another participant), nothing happens in the browser. There is no polling, SSE, or WebSocket mechanism. The human sends a message into the void and has to reload to see a reply.

This breaks the core product differentiator. The human-agent duo requires conversational flow — send message, see response appear. Without live updates, the chat UI is a form that submits into silence.

## Why This Gap Over Others

End-to-end testing of `cmd/reply` (state.md option 1) is blocked by ANTHROPIC_API_KEY availability and is a verification step, not a build. Conversation types (option 2) add structure to something that doesn't flow yet. Opening the auth gate (option 3) invites users to a broken conversation experience.

Live updates are the gap between "infrastructure works" and "the product works." Lesson 22: "works correctly" vs "works as intended" — the conversation stack works correctly but the experience is broken.

## What "Filled" Looks Like

HTMX polling on the conversation detail view. The messages container polls every 3 seconds for new messages since the last rendered message ID. New messages appear at the bottom with the same chat bubble styling. No WebSockets, no SSE — just an HTMX `hx-trigger="every 3s"` on a lightweight endpoint that returns empty or new `chatMessage` fragments.

The endpoint: `GET /app/{slug}/conversation/{id}/messages?after={lastMessageID}` — returns only new messages as HTML fragments. The existing `chatMessage` templ component is reused.
