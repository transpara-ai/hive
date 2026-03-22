# Scout Report — Iteration 46

## Gap: Mind uses polling in an event-driven architecture

Matt: "polling? why polling? we have event driven arch"

The site is event-driven — every action is a grammar op. When `handleOp` processes a `respond` in a conversation, it already has all the context. The Mind should be triggered there, not by polling the DB every 10 seconds.

## What "Filled" Looks Like

When a human sends a message in a conversation with an agent, the handler triggers the Mind immediately. No polling loop, no staleness guard, no wasted DB queries.
