# Build Report — iter 239: hive CTA on landing page

## Gap
Landing page had no link to `/hive`, leaving the "Watch it build" story inaccessible from the homepage.

## Change
**`site/views/home.templ`** — Added "Watch it build →" CTA link below the hero action buttons.

Placement: inside the hero `<section>`, immediately after the primary CTA `<div>` (Try it free / See how it works). Styled as a subtle `text-sm text-warm-faint` paragraph with the link in `text-brand` rose accent — uncluttered and consistent with Ember Minimalism.

```html
<p class="reveal text-sm text-warm-faint" style="--d:3">
    or <a href="/hive" class="text-brand hover:text-brand-dark transition-colors">Watch it build →</a>
</p>
```

## Verification
- `templ generate` — 16 updates, no errors
- `go build -buildvcs=false ./...` — clean
- `go test ./...` — all pass (graph: 0.583s, auth: cached)
