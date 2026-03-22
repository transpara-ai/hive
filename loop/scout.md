# Scout Report — Iteration 32

## Map

Iteration 31 added the conversation primitive: kind='conversation', 'converse' grammar op, Chat lens. But clicking a conversation routes to generic NodeDetail — a task/post detail view with "Replies (N)" header. Not chat.

## Gap Type

Interface bottleneck (blocks adoption of new primitive)

## The Gap

Conversations exist as data but have no chat-optimized UI. NodeDetail shows task metadata, edit forms, subtask progress — none of which apply to conversations. Messages display as generic "replies" with no chat feel. No message input at bottom. No visual distinction between your messages and others'.

## Why This Gap Over Others

Lesson 18: unlock the bottleneck before building what flows through it. Lesson 20: infrastructure → interface → management. The conversation primitive (infrastructure) is done. The interface that uses it is broken. Building Mind response integration before fixing the conversation UI is building flow through a broken pipe.

## What "Filled" Looks Like

`/app/{slug}/conversation/{id}` — dedicated route with chat-optimized view. Messages in chronological order. Chat bubbles with visual distinction (your messages vs others vs agents). Input form at bottom. Back arrow to conversations list. HTMX-powered message sending that appends and auto-scrolls.
