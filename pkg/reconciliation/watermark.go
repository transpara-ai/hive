package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadWatermark returns the last watermark recorded for a space, or the
// zero time.Time when no row exists yet. ListOpsSince treats the zero
// time as "everything" on the first ever cycle.
func loadWatermark(ctx context.Context, pool *pgxpool.Pool, space string) (time.Time, error) {
	var w time.Time
	err := pool.QueryRow(ctx,
		`SELECT watermark FROM reconciliation_state WHERE space_slug = $1`,
		space,
	).Scan(&w)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("load watermark: %w", err)
	}
	return w, nil
}

// saveWatermark upserts the watermark for a space. Called at the end of a
// reconciliation cycle once all newer ops have been anchored.
func saveWatermark(ctx context.Context, pool *pgxpool.Pool, space string, w time.Time) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO reconciliation_state (space_slug, watermark, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (space_slug) DO UPDATE
		SET watermark = EXCLUDED.watermark, updated_at = now()
	`, space, w)
	if err != nil {
		return fmt.Errorf("save watermark: %w", err)
	}
	return nil
}

// hasSiteOpReceived checks whether the chain already contains a
// site.op.received event whose external_ref.id matches opID. Used to
// skip ops that the webhook path already anchored — the two paths
// cooperate, but only one anchor event is ever produced per op.
//
// Queries against the JSONB content column. The external_ref field is
// written by AnchorSiteOp as {system: "site", id: <op.ID>}. The store's
// schema indexes on event_type; this predicate narrows by event type
// first, then by the JSONB path.
func hasSiteOpReceived(ctx context.Context, pool *pgxpool.Pool, opID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM events
			WHERE event_type = 'site.op.received'
			  AND content_json->'external_ref'->>'id' = $1
		)
	`, opID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has site op received: %w", err)
	}
	return exists, nil
}
