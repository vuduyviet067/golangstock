package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// Inventory statuses. With quantity >= 0 and minimum_stock >= 0 every product
// is in exactly one: normal (quantity > minimum_stock), low_stock
// (0 < quantity <= minimum_stock) or out_of_stock (quantity = 0).
const (
	InventoryStatusNormal     = "normal"
	InventoryStatusLowStock   = "low_stock"
	InventoryStatusOutOfStock = "out_of_stock"
)

// Stock movement chart granularities.
const (
	ChartGranularityDay   = "day"
	ChartGranularityMonth = "month"
)

// Stock movement chart query errors. Each concerns one query parameter.
var (
	ErrInvalidChartFrom   = errors.New("from must be a date in YYYY-MM-DD format")
	ErrInvalidChartTo     = errors.New("to must be a date in YYYY-MM-DD format")
	ErrChartRangeReversed = errors.New("from must be on or before to")
	ErrChartRangeTooLong  = errors.New("date range is too long for the granularity")
	ErrInvalidGranularity = errors.New("granularity must be auto, day or month")
	ErrInvalidTimezone    = errors.New("tz must be an IANA time zone name")
)

// errInconsistentSummary means the status counts do not add up to the
// product total; with the database constraints this indicates a bug.
var errInconsistentSummary = errors.New("inventory status counts do not add up to the product total")

const (
	chartDateLayout = "2006-01-02"
	// defaultChartDays is the range used when from is omitted.
	defaultChartDays = 30
	// maxAutoDayDays is the longest range auto granularity shows by day.
	maxAutoDayDays = 62
	// maxDayDays and maxMonthDays bound the number of buckets.
	maxDayDays   = 92
	maxMonthDays = 731
	defaultTZ    = "UTC"
)

// DashboardUseCase computes the dashboard figures. It only reads.
type DashboardUseCase struct {
	repo repository.DashboardRepository
	// now returns the current time; it decides the default date range.
	now func() time.Time
}

func NewDashboardUseCase(repo repository.DashboardRepository) *DashboardUseCase {
	return &DashboardUseCase{
		repo: repo,
		now:  time.Now,
	}
}

// Summary returns the summary cards and the status breakdown from one
// database snapshot.
func (uc *DashboardUseCase) Summary(ctx context.Context) (domain.InventorySummary, error) {
	s, err := uc.repo.InventorySummary(ctx)
	if err != nil {
		return domain.InventorySummary{}, fromRepository(err)
	}
	if s.Normal+s.LowStock+s.OutOfStock != s.TotalProducts {
		return domain.InventorySummary{}, fmt.Errorf("%w: %d + %d + %d != %d",
			errInconsistentSummary, s.Normal, s.LowStock, s.OutOfStock, s.TotalProducts)
	}
	return s, nil
}

// StockMovementChart returns the stock moved in and out per day or month.
//
// Defaults: tz UTC, to = today in tz, from = 29 days before to, granularity
// auto (by day up to 62 days, by month beyond). A range may span at most 92
// days by day and 731 days by month. The range covers whole local days in
// tz, from the first instant of from to the last instant of to, even when
// daylight saving time skips or repeats midnight. The repository receives
// the calendar dates and selects movements by their local date in the
// database, so the range and the buckets use the same time zone rules
// (time.Date would pick an arbitrary instant for a skipped or repeated
// midnight).
func (uc *DashboardUseCase) StockMovementChart(
	ctx context.Context,
	q dto.StockMovementChartQuery,
) (domain.StockMovementChart, error) {
	loc, err := chartLocation(q.Timezone)
	if err != nil {
		return domain.StockMovementChart{}, err
	}

	from, to, err := chartDates(q.From, q.To, uc.now().In(loc))
	if err != nil {
		return domain.StockMovementChart{}, err
	}

	days := int(to.Sub(from)/(24*time.Hour)) + 1
	granularity, err := chartGranularity(q.Granularity, days)
	if err != nil {
		return domain.StockMovementChart{}, err
	}

	buckets, err := uc.repo.StockMovementBuckets(ctx, granularity, loc.String(), from, to)
	if err != nil {
		return domain.StockMovementChart{}, fromRepository(err)
	}

	chart := domain.StockMovementChart{
		From:        from.Format(chartDateLayout),
		To:          to.Format(chartDateLayout),
		Granularity: granularity,
		Timezone:    loc.String(),
		Points:      chartPeriods(from, to, granularity),
	}

	index := make(map[string]int, len(chart.Points))
	for i, p := range chart.Points {
		index[p.Period] = i
	}
	for _, b := range buckets {
		i, ok := index[b.Period]
		if !ok {
			return domain.StockMovementChart{}, fmt.Errorf("stock movement bucket %q is outside %s..%s", b.Period, chart.From, chart.To)
		}
		chart.Points[i].StockIn = b.StockIn
		chart.Points[i].StockOut = b.StockOut
		chart.TotalStockIn += b.StockIn
		chart.TotalStockOut += b.StockOut
	}

	return chart, nil
}

// areaLocationTZ matches IANA names of the form Area/Location (possibly with
// more parts, e.g. America/Argentina/Buenos_Aires, or Etc/GMT+7).
var areaLocationTZ = regexp.MustCompile(`^[A-Za-z]+(/[A-Za-z0-9_+-]+)+$`)

// chartLocation resolves the tz parameter. Only "UTC" and existing
// Area/Location names are accepted. Other names are rejected even when Go
// knows them: PostgreSQL reads names such as CET, MET, EET or WET as
// fixed-offset abbreviations while Go applies daylight saving time, and
// "Local" is the server's zone. No abbreviation contains a slash.
func chartLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultTZ
	}
	if name != defaultTZ && !areaLocationTZ.MatchString(name) {
		return nil, ErrInvalidTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidTimezone
	}
	return loc, nil
}

// chartDates parses from and to as calendar dates (returned as UTC
// midnights, so day arithmetic is exact) and applies the defaults relative to
// now, which is already in the chart's time zone.
func chartDates(rawFrom, rawTo string, now time.Time) (time.Time, time.Time, error) {
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if rawTo = strings.TrimSpace(rawTo); rawTo != "" {
		parsed, err := time.Parse(chartDateLayout, rawTo)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidChartTo
		}
		to = parsed
	}

	from := to.AddDate(0, 0, -(defaultChartDays - 1))
	if rawFrom = strings.TrimSpace(rawFrom); rawFrom != "" {
		parsed, err := time.Parse(chartDateLayout, rawFrom)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidChartFrom
		}
		from = parsed
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, ErrChartRangeReversed
	}
	return from, to, nil
}

// chartGranularity resolves auto and checks the range length (in days,
// inclusive) against the granularity's limit.
func chartGranularity(raw string, days int) (string, error) {
	granularity := strings.ToLower(strings.TrimSpace(raw))
	switch granularity {
	case "", "auto":
		granularity = ChartGranularityDay
		if days > maxAutoDayDays {
			granularity = ChartGranularityMonth
		}
	case ChartGranularityDay, ChartGranularityMonth:
	default:
		return "", ErrInvalidGranularity
	}

	if granularity == ChartGranularityDay && days > maxDayDays ||
		granularity == ChartGranularityMonth && days > maxMonthDays {
		return "", ErrChartRangeTooLong
	}
	return granularity, nil
}

// chartPeriods lists every period from from to to (UTC calendar dates) with
// zero totals: each day, or the first day of each month.
func chartPeriods(from, to time.Time, granularity string) []domain.StockMovementBucket {
	var points []domain.StockMovementBucket
	if granularity == ChartGranularityMonth {
		last := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
		for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(last); m = m.AddDate(0, 1, 0) {
			points = append(points, domain.StockMovementBucket{Period: m.Format(chartDateLayout)})
		}
		return points
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		points = append(points, domain.StockMovementBucket{Period: d.Format(chartDateLayout)})
	}
	return points
}
