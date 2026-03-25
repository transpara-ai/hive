# Build Report — Add KindQuestion entity kind

## Gap
Q&A product foundation missing. Knowledge layer (Layer 6) needs question entity kind to support Q&A mode alongside claims and documents.

## Changes

### site/graph/store.go
- Added `KindQuestion = "question"` constant after `KindDocument`

### site/graph/handlers.go
- Added `handleQuestions` — lists all questions in a space (GET /app/{slug}/questions), supports `?q=` search, JSON+HTML responses
- Added `handleQuestionDetail` — shows a single question with its answers (GET /app/{slug}/question/{id}), JSON+HTML responses
- Registered routes in `Register()`: `GET /app/{slug}/questions`, `GET /app/{slug}/question/{id}`
- Added `KindQuestion` to the intend handler kind allowlist (alongside KindDocument, KindPolicy, etc.)

### site/graph/views.templ
- Added `questionsIcon()` — question mark circle SVG icon
- Added `QuestionsView` — list view with new question form, search, question cards showing title/body/author/answer count
- Added `QuestionDetailView` — detail view showing question + answers list + answer submission form (respond op)
- Added `questions` lens to sidebar "More" section (after documents)
- Added `questions` mobile nav tab (after documents)

### site/graph/handlers_test.go
- Added `TestHandlerQuestions` with 3 subtests:
  - `create_question` — POST /op with kind=question, verifies node created with correct kind/title
  - `list_questions` — GET /app/{slug}/questions, verifies questions returned
  - `question_detail` — GET /app/{slug}/question/{id}, verifies question fetched with correct id/kind

## Verification
- `templ generate` — OK (15 updates)
- `go.exe build -buildvcs=false ./...` — OK
- `go.exe test ./...` — OK (all pass)

## Pattern followed
KindDocument pattern from iter 234: same handler structure, same JSON/HTML content negotiation, same form-based create flow, same sidebar/mobile nav placement.
