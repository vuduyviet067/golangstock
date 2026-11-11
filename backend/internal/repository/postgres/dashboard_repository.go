package postgres

import (
	"context"
	"database/sql"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

type DashboardRepository struct {
	db *sql.DB
}

func NewDashboardRepository(db *sql.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// InventorySummary reads the whole product table once. The status conditions
// match usecase.InventoryStatus*: they are mutually exclusive and, with the
// quantity and minimum_stock CHECK constraints, cover every product.
func (r *DashboardRepository) InventorySummary(ctx context.Context) (domain.InventorySummary, error) {
	var s domain.InventorySummary
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(quantity), 0),
		        COUNT(*) FILTER (WHERE quantity > minimum_stock),
		        COUNT(*) FILTER (WHERE quantity > 0 AND quantity <= minimum_stock),
		        COUNT(*) FILTER (WHERE quantity = 0)
		 FROM products`,
	).Scan(&s.TotalProducts, &s.TotalStock, &s.Normal, &s.LowStock, &s.OutOfStock)
	if err != nil {
		return domain.InventorySummary{}, err
	}
	return s, nil
}

// StockMovementBuckets groups by the local date (or month) of created_at in
// the given time zone and keeps the movements whose local date is from..to.
// That is the same expression as the bucket, so the range and the buckets
// always agree, including on days where a daylight saving change skips local
// midnight (the day starts at the first instant after the gap) or repeats it
// (the day starts at the first 00:00). Turning a local midnight into an
// instant cannot express both: PostgreSQL picks the later offset for a
// repeated time, which would drop the first 00:xx of the day.
//
// The extra bounds one day wider on each side contain every instant of those
// local dates (no UTC offset change reaches a day) and are constants for the
// query, so the scan can use stock_movements_created_at_idx. There is no join
// with products, so movements of deleted products are counted.
func (r *DashboardRepository) StockMovementBuckets(
	ctx context.Context,
	granularity, timezone string,
	from, to time.Time,
) ([]domain.StockMovementBucket, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT to_char(date_trunc($1, created_at AT TIME ZONE $2), 'YYYY-MM-DD') AS period,
		        COALESCE(SUM(quantity_change) FILTER (WHERE movement_type = 'in'), 0)   AS stock_in,
		        COALESCE(-SUM(quantity_change) FILTER (WHERE movement_type = 'out'), 0) AS stock_out
		 FROM stock_movements
		 WHERE movement_type IN ('in', 'out')
		   AND created_at >= (($3::date - 1)::timestamp AT TIME ZONE $2)
		   AND created_at < (($4::date + 2)::timestamp AT TIME ZONE $2)
		   AND (created_at AT TIME ZONE $2)::date BETWEEN $3::date AND $4::date
		 GROUP BY 1
		 ORDER BY 1`,
		granularity,
		timezone,
		from,
		to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := make([]domain.StockMovementBucket, 0)
	for rows.Next() {
		var b domain.StockMovementBucket
		if err := rows.Scan(&b.Period, &b.StockIn, &b.StockOut); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return buckets, nil
}

var _ repository.DashboardRepository = (*DashboardRepository)(nil)
