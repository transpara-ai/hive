// Package reconciliation polls the site for missed ops and replays them
// through the hive's anchor+translate dispatcher. It is the safety net for
// the webhook path in pkg/runner — if a webhook is dropped (site restart,
// network partition, hive downtime), the ticker catches up on the next
// cycle. Idempotency is enforced by checking the chain for an existing
// site.op.received event with the same external ref before anchoring.
package reconciliation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schema creates the reconciliation watermark table. One row per space
// slug — the newest created_at observed from the site. The ticker reads
// it at the start of each cycle and writes the max seen at the end.
const schema = `
CREATE TABLE IF NOT EXISTS reconciliation_state (
    space_slug TEXT PRIMARY KEY,
    watermark  TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// EnsureTables creates the reconciliation_state table. Idempotent — safe to
// call on every startup.
func EnsureTables(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("reconciliation schema: %w", err)
	}
	return nil
}
