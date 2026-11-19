package repository

import (
	"context"
	"time"

	"gostock/internal/domain"
)

// DashboardRepository aggregates dashboard figures in the database. It only
// reads.
type DashboardRepository interface {
	// InventorySummary counts products per inventory status and sums their
	// stock in one query, so all figures come from the same snapshot.
	InventorySummary(ctx context.Context) (domain.InventorySummary, error)

	// StockMovementBuckets sums 'in' and 'out' movements per granularity
	// period over the calendar dates from to to (inclusive) in the IANA time
	// zone timezone: every movement whose local date is in that range, i.e.
	// whole local days even when daylight saving time skips or repeats
	// midnight. Only the year, month and day of from and to are used; the
	// database selects and groups the movements by the same local date, so
	// both use the same time zone rules. Only periods with movements are
	// returned, oldest first. Movements of deleted products are included.
	StockMovementBuckets(
		ctx context.Context,
		granularity, timezone string,
		from, to time.Time,
	) ([]domain.StockMovementBucket, error)
}
