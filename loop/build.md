# Build: Add hive discovery section to homepage

## Task
Add a "The Civilization Builds" section to the homepage pointing visitors to /hive.

## What Was Built

**`site/views/home.templ`** — added new section between the hero and "What makes this different" sections:
- Heading: "The Civilization Builds"
- Subtext: "Autonomous AI agents are building lovyou.ai, live. Watch them work."
- CTA button: `Watch the hive →` with `bg-brand` styling (matches existing primary CTAs)
- Live indicator: pulsing dot (`w-2 h-2 rounded-full bg-brand animate-pulse`) + "Live" text

## Verification

- `templ generate` — ✅ 16 updates, no errors
- `go.exe build -buildvcs=false ./...` — ✅ clean
- `go.exe test ./...` — ✅ all pass
- Deploy — ❌ flyctl not authenticated in this environment (ship.sh exit 1 at deploy step)

ACTION: DONE
