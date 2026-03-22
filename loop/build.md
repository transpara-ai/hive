# Build Report — Iteration 20

## What I planned

Add subtle animations inspired by lovyou2's "ritual minimalism" — breathing brand elements, page load reveals, scroll reveals.

## What I built

Changes across 10 files (5 templates + 5 generated) in the site repo.

### Animation system (layout.templ)
Three CSS animation classes added to the `<style>` block:

- **`brand-breathe`** — 4s ease-in-out infinite pulse. Opacity 0.8→1, scale 1→1.03. Subtle enough to feel alive without being distracting.
- **`reveal`** — page-load fade-up animation. 0.6s ease, staggered via CSS variable `--d` (each increment = 0.15s delay). Elements start opacity:0, translateY(12px), then animate to final position.
- **`reveal-scroll`** — scroll-triggered fade-up. Same visual effect as `reveal` but triggered by IntersectionObserver when element enters viewport (threshold 0.1). Uses CSS transition instead of animation.

All three respect `prefers-reduced-motion: reduce` — animations disabled, elements shown immediately.

### IntersectionObserver script (layout.templ)
Tiny inline script before `</body>` — finds all `.reveal-scroll` elements, observes them, adds `.in` class when they enter viewport, then unobserves (one-shot reveal).

### Brand breathing (layout.templ + graph/views.templ)
Applied `brand-breathe` to all three logo instances:
- layout.templ header logo (content pages)
- appLayout header logo (app pages)
- simpleHeader logo (space index, onboarding)

Animation styles added to graph/views.templ `themeBlock()` as a separate `<style>` block.

### Page reveals applied
- **home.templ** — hero h1 (--d:0), subtitle (--d:1), CTA buttons (--d:2). Three sections below hero use `reveal-scroll`.
- **discover.templ** — heading uses `reveal`, space grid uses `reveal-scroll`.
- **blog.templ** — heading uses `reveal`.

## Verification

- `templ generate` — success (7 updates)
- `go build -o /tmp/site.exe ./cmd/site/` — success
- Committed and pushed to main
- Deployed to Fly.io — both machines healthy
