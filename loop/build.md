# Build Report — Iterations 168-171

**168: Inline reply** — Reply button on message hover. Reply preview bar above chat input. Prepends `> @author: text` as markdown quote on send. Cancel button clears.

**169: Keyboard shortcuts** — ? opens help dialog listing all shortcuts. G+B/F/C/A/K navigates to Board/Feed/Chat/Activity/Knowledge. G+H goes to dashboard. Extracts space slug from URL. Skips when focused in input/textarea.

**170: Inline status change** — Select dropdown appears on TaskCard hover (opacity transition). stopPropagation prevents card navigation. Fires POST to /node/{id}/state. Reloads page.

**171: Empty state illustrations** — Feed, Threads, Chat empty states replaced with SVG icons + descriptive text + helpful hint.
