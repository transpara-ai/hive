package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/runner"
)

// defaultInterval is the reconciliation cycle cadence. Tuned to 60s so the
// chain sees missed ops within a minute without hammering the site API.
// Overridable via HIVE_RECON_INTERVAL (e.g. "30s", "2m").
const defaultInterval = 60 * time.Second

// Dispatcher is the subset of runner.Dispatcher the ticker needs. It
// matches the webhook path's contract so the ticker can reuse the same
// anchor+translate flow on the hive runtime.
type Dispatcher interface {
	AnchorSiteOp(ctx context.Context, op runner.OpEvent) (types.EventID, error)
	EmitSiteOp(ctx context.Context, op runner.OpEvent, anchorID types.EventID) error
}

// Source returns site ops created after the given watermark. In
// production this is the site API (GET /api/hive/site-ops). Tests
// provide in-memory stubs.
type Source interface {
	ListOpsSince(slug string, since time.Time) ([]runner.OpEvent, error)
}

// State is the persistence contract the ticker needs: watermark
// read/write and chain idempotency lookup. In production a pgxpool-backed
// implementation (pgState) is used; tests inject an in-memory stub so the
// tick loop can be exercised without postgres.
type State interface {
	LoadWatermark(ctx context.Context, space string) (time.Time, error)
	SaveWatermark(ctx context.Context, space string, w time.Time) error
	HasSiteOpReceived(ctx context.Context, opID string) (bool, error)
}

// pgState adapts a pgxpool.Pool to the State interface by delegating to
// the package-level helpers in watermark.go.
type pgState struct{ pool *pgxpool.Pool }

func (s pgState) LoadWatermark(ctx context.Context, space string) (time.Time, error) {
	return loadWatermark(ctx, s.pool, space)
}

func (s pgState) SaveWatermark(ctx context.Context, space string, w time.Time) error {
	return saveWatermark(ctx, s.pool, space, w)
}

func (s pgState) HasSiteOpReceived(ctx context.Context, opID string) (bool, error) {
	return hasSiteOpReceived(ctx, s.pool, opID)
}

// Ticker periodically fetches site ops newer than the stored watermark
// and pushes any that aren't yet anchored through the dispatcher. Safe
// to run concurrently with the webhook listener: HasSiteOpReceived
// gates the anchor emit, so a webhook and a reconciliation pass can
// race on the same op without producing duplicate anchors.
type Ticker struct {
	state      State
	dispatcher Dispatcher
	source     Source
	space      string
	interval   time.Duration
}

// NewTicker builds a Ticker with the production pgxpool-backed State.
// interval is clamped to defaultInterval when HIVE_RECON_INTERVAL is
// unset or unparseable.
func NewTicker(pool *pgxpool.Pool, dispatcher Dispatcher, source Source, space string) *Ticker {
	return newTickerWithState(pgState{pool: pool}, dispatcher, source, space)
}

// newTickerWithState is the package-internal constructor used by tests
// to inject an in-memory State. Shares interval resolution with NewTicker.
func newTickerWithState(state State, dispatcher Dispatcher, source Source, space string) *Ticker {
	interval := defaultInterval
	if v := os.Getenv("HIVE_RECON_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		} else {
			log.Printf("[recon] ignoring invalid HIVE_RECON_INTERVAL=%q, using %v", v, defaultInterval)
		}
	}
	return &Ticker{
		state:      state,
		dispatcher: dispatcher,
		source:     source,
		space:      space,
		interval:   interval,
	}
}

// Start blocks until ctx is cancelled, running one cycle immediately and
// then one per interval. Each cycle is bounded: source.ListOpsSince
// returns a finite slice, and per-op anchor work is best-effort (errors
// log and continue so a single bad op does not halt the loop).
func (t *Ticker) Start(ctx context.Context) {
	log.Printf("[recon] starting: space=%s interval=%v", t.space, t.interval)
	t.tick(ctx)

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[recon] stopping")
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

// tick runs one reconciliation cycle: load watermark, list ops since,
// anchor the ones the chain hasn't already seen, advance the watermark.
// Errors log and return early — the next tick will retry.
func (t *Ticker) tick(ctx context.Context) {
	watermark, err := t.state.LoadWatermark(ctx, t.space)
	if err != nil {
		log.Printf("[recon] load watermark: %v", err)
		return
	}

	ops, err := t.source.ListOpsSince(t.space, watermark)
	if err != nil {
		log.Printf("[recon] list ops since %v: %v", watermark, err)
		return
	}
	if len(ops) == 0 {
		return
	}

	log.Printf("[recon] cycle: watermark=%v ops=%d", watermark, len(ops))

	newWatermark := watermark
	for _, op := range ops {
		// Advance the watermark regardless of outcome — an op we've
		// already seen still counts as progress. Without this, a stuck
		// op (e.g. agent-authored, skipped below) would hold the
		// watermark back forever.
		if op.CreatedAt.After(newWatermark) {
			newWatermark = op.CreatedAt
		}

		// Self-loop guard: ops the hive emitted back to the site must
		// not re-enter the anchor path.
		if op.ActorKind == "agent" {
			continue
		}

		anchored, err := t.state.HasSiteOpReceived(ctx, op.ID)
		if err != nil {
			log.Printf("[recon] has-site-op %s: %v", op.ID, err)
			continue
		}
		if anchored {
			continue
		}

		anchorID, err := t.dispatcher.AnchorSiteOp(ctx, op)
		if err != nil {
			log.Printf("[recon] anchor %s: %v", op.ID, err)
			continue
		}
		log.Printf("[recon] anchored missed op: id=%s op=%s actor=%s", op.ID, op.Op, op.Actor)

		// Translate asynchronously to mirror the webhook path — we've
		// already paid the anchor cost, the chain is consistent, and
		// translation errors are recorded on-chain as site.op.rejected
		// by the dispatcher.
		go func(op runner.OpEvent, anchorID types.EventID) {
			if err := t.dispatcher.EmitSiteOp(ctx, op, anchorID); err != nil {
				log.Printf("[recon] translate %s: %v", op.ID, err)
			}
		}(op, anchorID)
	}

	if newWatermark.After(watermark) {
		if err := t.state.SaveWatermark(ctx, t.space, newWatermark); err != nil {
			log.Printf("[recon] save watermark: %v", err)
		}
	}
}

// HTTPSource is the production implementation of Source. It hits the
// site's /api/hive/site-ops endpoint (authored by Prompt 5 of this
// migration). Until that endpoint lands, ListOpsSince returns an empty
// slice and logs the failure — the ticker keeps running, so once the
// endpoint ships no restart is needed.
type HTTPSource struct {
	Base   string
	APIKey string
	Client *http.Client
}

// NewHTTPSource builds a Source with a 30s HTTP client.
func NewHTTPSource(base, apiKey string) *HTTPSource {
	return &HTTPSource{
		Base:   base,
		APIKey: apiKey,
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListOpsSince fetches ops from /api/hive/site-ops. The `since` timestamp
// is RFC3339Nano; the zero value is omitted so the server returns all
// available ops for the first cycle.
func (s *HTTPSource) ListOpsSince(slug string, since time.Time) ([]runner.OpEvent, error) {
	q := url.Values{}
	q.Set("space", slug)
	if !since.IsZero() {
		q.Set("since", since.UTC().Format(time.RFC3339Nano))
	}
	u := fmt.Sprintf("%s/api/hive/site-ops?%s", s.Base, q.Encode())

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Endpoint not yet deployed — log once, return empty. The
		// ticker will keep trying on its own cadence.
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}

	var body struct {
		Ops []runner.OpEvent `json:"ops"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode ops: %w", err)
	}
	return body.Ops, nil
}
