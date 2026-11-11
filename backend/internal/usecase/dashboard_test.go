package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
)

type fakeDashboardRepository struct {
	summary    domain.InventorySummary
	summaryErr error

	buckets    []domain.StockMovementBucket
	bucketsErr error

	bucketsCalled  bool
	gotGranularity string
	gotTimezone    string
	// gotFrom and gotTo are the calendar dates passed on, as YYYY-MM-DD.
	gotFrom string
	gotTo   string
}

func (f *fakeDashboardRepository) InventorySummary(ctx context.Context) (domain.InventorySummary, error) {
	return f.summary, f.summaryErr
}

func (f *fakeDashboardRepository) StockMovementBuckets(
	ctx context.Context,
	granularity, timezone string,
	from, to time.Time,
) ([]domain.StockMovementBucket, error) {
	f.bucketsCalled = true
	f.gotGranularity, f.gotTimezone = granularity, timezone
	f.gotFrom, f.gotTo = from.Format("2006-01-02"), to.Format("2006-01-02")
	return f.buckets, f.bucketsErr
}

// newTestDashboard returns a use case whose clock is fixed at now.
func newTestDashboard(repo *fakeDashboardRepository, now time.Time) *DashboardUseCase {
	uc := NewDashboardUseCase(repo)
	uc.now = func() time.Time { return now }
	return uc
}

func TestDashboardSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary domain.InventorySummary
	}{
		{"empty", domain.InventorySummary{}},
		{"mixed", domain.InventorySummary{TotalProducts: 6, TotalStock: 40, Normal: 3, LowStock: 2, OutOfStock: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewDashboardUseCase(&fakeDashboardRepository{summary: tt.summary})
			got, err := uc.Summary(context.Background())
			if err != nil {
				t.Fatalf("Summary: %v", err)
			}
			if got != tt.summary {
				t.Errorf("Summary = %+v, want %+v", got, tt.summary)
			}
		})
	}
}

func TestDashboardSummary_InconsistentCountsAreAnError(t *testing.T) {
	uc := NewDashboardUseCase(&fakeDashboardRepository{
		summary: domain.InventorySummary{TotalProducts: 5, Normal: 1, LowStock: 1, OutOfStock: 1},
	})
	if _, err := uc.Summary(context.Background()); !errors.Is(err, errInconsistentSummary) {
		t.Fatalf("error = %v, want errInconsistentSummary", err)
	}
}

func TestDashboardSummary_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection reset")
	uc := NewDashboardUseCase(&fakeDashboardRepository{summaryErr: dbErr})
	if _, err := uc.Summary(context.Background()); !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want the repository error", err)
	}
}

func TestStockMovementChart_Defaults(t *testing.T) {
	// 01:30 UTC on 12 Nov is still 11 Nov in New York and already 12 Nov in
	// Ho Chi Minh City: "today" depends on tz.
	now := time.Date(2026, 11, 12, 1, 30, 0, 0, time.UTC)

	tests := []struct {
		tz           string
		wantFrom     string
		wantTo       string
		wantTimezone string
	}{
		{"", "2026-10-14", "2026-11-12", "UTC"},
		{"Asia/Ho_Chi_Minh", "2026-10-14", "2026-11-12", "Asia/Ho_Chi_Minh"},
		{"America/New_York", "2026-10-13", "2026-11-11", "America/New_York"},
	}

	for _, tt := range tests {
		t.Run("tz="+tt.tz, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			chart, err := newTestDashboard(repo, now).StockMovementChart(context.Background(), dto.StockMovementChartQuery{Timezone: tt.tz})
			if err != nil {
				t.Fatalf("StockMovementChart: %v", err)
			}
			if chart.From != tt.wantFrom || chart.To != tt.wantTo || chart.Timezone != tt.wantTimezone {
				t.Errorf("range = %s..%s %s, want %s..%s %s", chart.From, chart.To, chart.Timezone, tt.wantFrom, tt.wantTo, tt.wantTimezone)
			}
			if chart.Granularity != ChartGranularityDay || len(chart.Points) != 30 {
				t.Errorf("granularity %s with %d points, want day with 30", chart.Granularity, len(chart.Points))
			}
			if repo.gotTimezone != tt.wantTimezone || repo.gotGranularity != ChartGranularityDay {
				t.Errorf("repository received tz=%q granularity=%q", repo.gotTimezone, repo.gotGranularity)
			}
			if repo.gotFrom != tt.wantFrom || repo.gotTo != tt.wantTo {
				t.Errorf("repository received %s..%s, want %s..%s", repo.gotFrom, repo.gotTo, tt.wantFrom, tt.wantTo)
			}
		})
	}
}

// TestStockMovementChart_OneSidedRange covers requests with only from or only
// to; the missing side is defaulted relative to the fixed clock.
func TestStockMovementChart_OneSidedRange(t *testing.T) {
	// 01:30 UTC on 12 Nov: 12 Nov in Ho Chi Minh City, 11 Nov in New York.
	now := time.Date(2026, 11, 12, 1, 30, 0, 0, time.UTC)

	tests := []struct {
		name             string
		q                dto.StockMovementChartQuery
		wantFrom, wantTo string
		wantGranularity  string
		wantPoints       int
	}{
		{"only to: 30 days ending on to", dto.StockMovementChartQuery{To: "2026-03-31"}, "2026-03-02", "2026-03-31", "day", 30},
		{"only to, across a year", dto.StockMovementChartQuery{To: "2026-01-10", Timezone: "America/Santiago"}, "2025-12-12", "2026-01-10", "day", 30},
		{"only from: up to today in tz", dto.StockMovementChartQuery{From: "2026-11-01", Timezone: "Asia/Ho_Chi_Minh"}, "2026-11-01", "2026-11-12", "day", 12},
		{"only from: today differs in New York", dto.StockMovementChartQuery{From: "2026-11-01", Timezone: "America/New_York"}, "2026-11-01", "2026-11-11", "day", 11},
		{"only from: today", dto.StockMovementChartQuery{From: "2026-11-12"}, "2026-11-12", "2026-11-12", "day", 1},
		{"only from: long range becomes monthly", dto.StockMovementChartQuery{From: "2026-01-15"}, "2026-01-15", "2026-11-12", "month", 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			chart, err := newTestDashboard(repo, now).StockMovementChart(context.Background(), tt.q)
			if err != nil {
				t.Fatalf("StockMovementChart: %v", err)
			}
			if chart.From != tt.wantFrom || chart.To != tt.wantTo || chart.Granularity != tt.wantGranularity || len(chart.Points) != tt.wantPoints {
				t.Errorf("chart %s..%s %s with %d points, want %s..%s %s with %d",
					chart.From, chart.To, chart.Granularity, len(chart.Points), tt.wantFrom, tt.wantTo, tt.wantGranularity, tt.wantPoints)
			}
			if repo.gotFrom != tt.wantFrom || repo.gotTo != tt.wantTo {
				t.Errorf("repository received %s..%s, want %s..%s", repo.gotFrom, repo.gotTo, tt.wantFrom, tt.wantTo)
			}
		})
	}

	// Only from, after today in tz: the defaulted to is earlier than from.
	repo := &fakeDashboardRepository{}
	_, err := newTestDashboard(repo, now).StockMovementChart(context.Background(),
		dto.StockMovementChartQuery{From: "2026-11-12", Timezone: "America/New_York"})
	if !errors.Is(err, ErrChartRangeReversed) || repo.bucketsCalled {
		t.Errorf("error = %v (repository called: %v), want ErrChartRangeReversed without a repository call", err, repo.bucketsCalled)
	}
}

// TestStockMovementChart_PassesCalendarDates checks that the repository gets
// the requested calendar dates and time zone unchanged; turning them into
// instants is left to the database (see the repository tests).
func TestStockMovementChart_PassesCalendarDates(t *testing.T) {
	for _, tz := range []string{"UTC", "Asia/Ho_Chi_Minh", "America/New_York", "America/Santiago"} {
		t.Run(tz, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			chart, err := newTestDashboard(repo, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)).StockMovementChart(
				context.Background(), dto.StockMovementChartQuery{From: "2026-09-05", To: "2026-09-07", Timezone: tz})
			if err != nil {
				t.Fatalf("StockMovementChart: %v", err)
			}
			if repo.gotFrom != "2026-09-05" || repo.gotTo != "2026-09-07" || repo.gotTimezone != tz {
				t.Errorf("repository received %s..%s %q, want 2026-09-05..2026-09-07 %q", repo.gotFrom, repo.gotTo, repo.gotTimezone, tz)
			}
			if chart.Timezone != tz || len(chart.Points) != 3 {
				t.Errorf("chart tz %q with %d points, want %q with 3", chart.Timezone, len(chart.Points), tz)
			}
		})
	}
}

func TestStockMovementChart_FillsEveryDayAndTotals(t *testing.T) {
	repo := &fakeDashboardRepository{buckets: []domain.StockMovementBucket{
		{Period: "2026-11-02", StockIn: 7, StockOut: 3},
		{Period: "2026-11-04", StockIn: 0, StockOut: 5},
	}}
	chart, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(), dto.StockMovementChartQuery{
		From: "2026-11-01", To: "2026-11-05",
	})
	if err != nil {
		t.Fatalf("StockMovementChart: %v", err)
	}

	want := []domain.StockMovementBucket{
		{Period: "2026-11-01"},
		{Period: "2026-11-02", StockIn: 7, StockOut: 3},
		{Period: "2026-11-03"},
		{Period: "2026-11-04", StockOut: 5},
		{Period: "2026-11-05"},
	}
	if len(chart.Points) != len(want) {
		t.Fatalf("points = %+v, want %+v", chart.Points, want)
	}
	for i := range want {
		if chart.Points[i] != want[i] {
			t.Errorf("point %d = %+v, want %+v", i, chart.Points[i], want[i])
		}
	}
	if chart.TotalStockIn != 7 || chart.TotalStockOut != 8 {
		t.Errorf("totals = %d in / %d out, want 7 / 8", chart.TotalStockIn, chart.TotalStockOut)
	}
}

func TestStockMovementChart_EmptyRangeIsAllZero(t *testing.T) {
	repo := &fakeDashboardRepository{buckets: []domain.StockMovementBucket{}}
	chart, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(), dto.StockMovementChartQuery{
		From: "2026-02-01", To: "2026-02-28",
	})
	if err != nil {
		t.Fatalf("StockMovementChart: %v", err)
	}
	if len(chart.Points) != 28 {
		t.Fatalf("points = %d, want 28", len(chart.Points))
	}
	for _, p := range chart.Points {
		if p.StockIn != 0 || p.StockOut != 0 {
			t.Errorf("point %+v, want zero", p)
		}
	}
	if chart.TotalStockIn != 0 || chart.TotalStockOut != 0 {
		t.Errorf("totals = %d / %d, want 0 / 0", chart.TotalStockIn, chart.TotalStockOut)
	}
}

func TestStockMovementChart_Granularity(t *testing.T) {
	tests := []struct {
		name            string
		from, to, gran  string
		wantGranularity string
		wantPoints      []string // first and last period, and count
		wantCount       int
	}{
		{"auto, 62 days is daily", "2026-01-01", "2026-03-03", "", "day", []string{"2026-01-01", "2026-03-03"}, 62},
		{"auto, 63 days is monthly", "2026-01-01", "2026-03-04", "auto", "month", []string{"2026-01-01", "2026-03-01"}, 3},
		{"single day", "2026-11-11", "2026-11-11", "", "day", []string{"2026-11-11", "2026-11-11"}, 1},
		{"explicit day, 92 days", "2026-01-01", "2026-04-02", "day", "day", []string{"2026-01-01", "2026-04-02"}, 92},
		{"explicit month across a year", "2025-11-15", "2026-02-02", "month", "month", []string{"2025-11-01", "2026-02-01"}, 4},
		{"explicit month, 731 days", "2025-01-01", "2027-01-01", "MONTH", "month", []string{"2025-01-01", "2027-01-01"}, 25},
		{"leap year February by day", "2028-02-27", "2028-03-01", "day", "day", []string{"2028-02-27", "2028-03-01"}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			chart, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(), dto.StockMovementChartQuery{
				From: tt.from, To: tt.to, Granularity: tt.gran,
			})
			if err != nil {
				t.Fatalf("StockMovementChart: %v", err)
			}
			if chart.Granularity != tt.wantGranularity || repo.gotGranularity != tt.wantGranularity {
				t.Errorf("granularity = %s (repository %s), want %s", chart.Granularity, repo.gotGranularity, tt.wantGranularity)
			}
			if len(chart.Points) != tt.wantCount {
				t.Fatalf("points = %d, want %d", len(chart.Points), tt.wantCount)
			}
			if first, last := chart.Points[0].Period, chart.Points[len(chart.Points)-1].Period; first != tt.wantPoints[0] || last != tt.wantPoints[1] {
				t.Errorf("periods %s..%s, want %s..%s", first, last, tt.wantPoints[0], tt.wantPoints[1])
			}
		})
	}
}

// TestStockMovementChart_DaylightSavingDays checks that a 23-hour day (DST
// start, also at midnight in Santiago) and a 25-hour day (DST end) each give
// exactly one daily point. The instants of those days are checked against
// PostgreSQL in the repository tests.
func TestStockMovementChart_DaylightSavingDays(t *testing.T) {
	for _, r := range []struct{ tz, from, to, middle string }{
		{"America/New_York", "2026-03-07", "2026-03-09", "2026-03-08"}, // DST starts 8 March 02:00
		{"America/New_York", "2026-10-31", "2026-11-02", "2026-11-01"}, // DST ends 1 November 02:00
		{"America/Santiago", "2026-09-05", "2026-09-07", "2026-09-06"}, // DST starts 6 September 00:00
		{"America/Santiago", "2026-04-04", "2026-04-06", "2026-04-05"}, // DST ends 5 April 00:00
	} {
		repo := &fakeDashboardRepository{buckets: []domain.StockMovementBucket{{Period: r.middle, StockIn: 1}}}
		chart, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(), dto.StockMovementChartQuery{
			From: r.from, To: r.to, Timezone: r.tz,
		})
		if err != nil {
			t.Fatalf("%s %s..%s: %v", r.tz, r.from, r.to, err)
		}
		want := []string{r.from, r.middle, r.to}
		if len(chart.Points) != len(want) {
			t.Fatalf("%s %s..%s: %d points, want 3", r.tz, r.from, r.to, len(chart.Points))
		}
		for i, p := range chart.Points {
			if p.Period != want[i] {
				t.Errorf("%s point %d = %s, want %s", r.tz, i, p.Period, want[i])
			}
		}
		if chart.Points[1].StockIn != 1 || chart.TotalStockIn != 1 {
			t.Errorf("%s: bucket for %s not placed: %+v", r.tz, r.middle, chart.Points)
		}
	}
}

// TestStockMovementChart_TimezonePolicy checks which tz values are accepted.
// Rejected names never reach the repository.
func TestStockMovementChart_TimezonePolicy(t *testing.T) {
	accepted := []string{
		"UTC", "Asia/Ho_Chi_Minh", "America/New_York", "America/Santiago", "Europe/Berlin",
		"America/Argentina/Buenos_Aires", "Etc/GMT+7", "Etc/GMT-14", " Asia/Ho_Chi_Minh ",
	}
	for _, tz := range accepted {
		t.Run("accept "+tz, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			if _, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(),
				dto.StockMovementChartQuery{From: "2026-09-01", To: "2026-09-02", Timezone: tz}); err != nil {
				t.Fatalf("StockMovementChart: %v", err)
			}
			if want := strings.TrimSpace(tz); repo.gotTimezone != want {
				t.Errorf("repository received tz %q, want %q", repo.gotTimezone, want)
			}
		})
	}

	rejected := []string{
		// Abbreviations that Go knows as DST zones but PostgreSQL reads as fixed offsets.
		"CET", "MET", "EET", "WET",
		// Other names without an Area/Location form.
		"EST", "GMT", "Zulu", "UCT", "utc", "Local", "EST5EDT", "PST8PDT",
		// Malformed or unknown names.
		"Mars/Base", "Asia/Ho Chi Minh", "Etc/GMT 7", "Asia/", "/UTC", "Asia//Tokyo",
		"../../etc/passwd", "Asia/../UTC", "Europe/Berlin\x00", "UTC'; DROP TABLE products;--",
	}
	for _, tz := range rejected {
		t.Run("reject "+tz, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			_, err := newTestDashboard(repo, time.Now()).StockMovementChart(context.Background(),
				dto.StockMovementChartQuery{From: "2026-09-01", To: "2026-09-02", Timezone: tz})
			if !errors.Is(err, ErrInvalidTimezone) {
				t.Fatalf("error = %v, want ErrInvalidTimezone", err)
			}
			if repo.bucketsCalled {
				t.Fatal("repository must not be called for a rejected time zone")
			}
		})
	}
}

func TestStockMovementChart_InvalidQuery(t *testing.T) {
	tests := []struct {
		name    string
		q       dto.StockMovementChartQuery
		wantErr error
	}{
		{"from not a date", dto.StockMovementChartQuery{From: "11/01/2026"}, ErrInvalidChartFrom},
		{"from impossible date", dto.StockMovementChartQuery{From: "2026-02-30", To: "2026-03-01"}, ErrInvalidChartFrom},
		{"from without zero padding", dto.StockMovementChartQuery{From: "2026-1-5", To: "2026-02-01"}, ErrInvalidChartFrom},
		{"to not a date", dto.StockMovementChartQuery{To: "tomorrow"}, ErrInvalidChartTo},
		{"to with time", dto.StockMovementChartQuery{To: "2026-11-11T00:00:00Z"}, ErrInvalidChartTo},
		{"from after to", dto.StockMovementChartQuery{From: "2026-11-02", To: "2026-11-01"}, ErrChartRangeReversed},
		{"day range over 92 days", dto.StockMovementChartQuery{From: "2026-01-01", To: "2026-04-03", Granularity: "day"}, ErrChartRangeTooLong},
		{"month range over 731 days", dto.StockMovementChartQuery{From: "2025-01-01", To: "2027-01-02", Granularity: "month"}, ErrChartRangeTooLong},
		{"auto range over 731 days", dto.StockMovementChartQuery{From: "2024-01-01", To: "2026-01-01"}, ErrChartRangeTooLong},
		{"unknown granularity", dto.StockMovementChartQuery{Granularity: "week"}, ErrInvalidGranularity},
		{"unknown time zone", dto.StockMovementChartQuery{Timezone: "Mars/Base"}, ErrInvalidTimezone},
		{"Local time zone", dto.StockMovementChartQuery{Timezone: "Local"}, ErrInvalidTimezone},
		{"path in time zone", dto.StockMovementChartQuery{Timezone: "../../etc/passwd"}, ErrInvalidTimezone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeDashboardRepository{}
			_, err := newTestDashboard(repo, time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)).StockMovementChart(context.Background(), tt.q)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if repo.bucketsCalled {
				t.Fatal("repository must not be called for an invalid query")
			}
		})
	}
}

func TestStockMovementChart_RepositoryErrors(t *testing.T) {
	dbErr := errors.New("connection reset")
	uc := newTestDashboard(&fakeDashboardRepository{bucketsErr: dbErr}, time.Now())
	if _, err := uc.StockMovementChart(context.Background(), dto.StockMovementChartQuery{}); !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want the repository error", err)
	}

	// A bucket outside the requested range indicates a bug and is reported.
	uc = newTestDashboard(&fakeDashboardRepository{buckets: []domain.StockMovementBucket{{Period: "1999-01-01", StockIn: 1}}}, time.Now())
	if _, err := uc.StockMovementChart(context.Background(), dto.StockMovementChartQuery{From: "2026-11-01", To: "2026-11-02"}); err == nil {
		t.Fatal("bucket outside the range was accepted")
	}
}
