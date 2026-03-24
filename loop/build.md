# Build Report — Iteration 192

## Quote Post (Derive Grammar Op)

**Schema:**
- `ALTER TABLE nodes ADD COLUMN IF NOT EXISTS quote_of_id TEXT NOT NULL DEFAULT ''`

**Node struct:**
- Added `QuoteOfID`, `QuoteOfAuthor`, `QuoteOfTitle`, `QuoteOfBody` fields
- Resolved at query time via correlated subqueries (same pattern as reply_to)

**Store:**
- `CreateNodeParams`: added `QuoteOfID` field
- `CreateNode` INSERT: added `quote_of_id` column ($17)
- `GetNode`: added 4 correlated subqueries for quote resolution (author, title, body)
- `ListNodes`: same 4 correlated subqueries added

**Handler:**
- `express` op: reads optional `quote_of_id` from form, passes to CreateNodeParams
- Feed handler: reads `?quote={id}` query param, loads quoted post for compose preview

**Template:**
- `FeedView`: accepts `quotePost *Node`, shows quote preview in compose form when present
- `FeedCard`: renders inline quote preview (bordered card with author + title + body) when `QuoteOfID` is set
- "quote" link in engagement bar → `/app/{slug}/feed?quote={id}`
- Compose form: hidden `quote_of_id` input, brand-bordered preview, "Add your thoughts..." placeholder

**Files changed:**
- `graph/store.go` — schema migration, Node struct, CreateNodeParams, GetNode, ListNodes queries
- `graph/handlers.go` — express op wiring, feed handler quote param
- `graph/views.templ` — FeedView, FeedCard, compose form, quotePostPlaceholder helper
