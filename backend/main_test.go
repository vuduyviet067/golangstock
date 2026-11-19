package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/usecase"
)

// The production executable must carry its own copy of the IANA time zone
// database (main.go imports time/tzdata). The dashboard validates the tz query
// parameter with time.LoadLocation, and the frontend always sends
// Asia/Ho_Chi_Minh; on a host without system zoneinfo and without a Go
// installation, every chart request would otherwise fail with 400 "invalid
// time zone". Go still prefers the host's zoneinfo when there is one; the
// embedded copy is only the fallback. It does not affect PostgreSQL, which
// uses its own time zone database for AT TIME ZONE.

// TestProductionExecutableEmbedsTZData checks the dependency graph of this
// package, which is what the linker puts into the executable: time/tzdata
// must be in it. An import that only a _test.go file makes does not count, so
// moving it into a test would fail here.
func TestProductionExecutableEmbedsTZData(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is needed to list the executable's dependencies: %v", err)
	}
	out, err := exec.Command(goTool, "list", "-deps", "-f", "{{.ImportPath}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps .: %v\n%s", err, out)
	}

	deps := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		deps[strings.TrimSpace(line)] = true
	}
	// The listing really is the server's: an empty or unrelated listing must
	// not pass by accident.
	if !deps["gostock/internal/app"] || !deps["gostock/internal/usecase"] {
		t.Fatalf("go list -deps . did not list the server's packages; output:\n%s", out)
	}
	if !deps["time/tzdata"] {
		t.Fatal(`time/tzdata is not linked into the production executable; keep the blank import _ "time/tzdata" in main.go`)
	}
}

// TestDashboardTimezonesLoadWithProductionImports sends the dashboard's time
// zones through the real validation (usecase.DashboardUseCase, which calls
// time.LoadLocation). This test binary links the same packages as the
// executable, embedded time zone database included, so building it with
// -trimpath and running it without system zoneinfo is the smoke test
// described in README.md ("Embedded time zone database"). On a host with
// zoneinfo it passes either way; the dependency check above is what fails
// when the import is removed.
func TestDashboardTimezonesLoadWithProductionImports(t *testing.T) {
	uc := usecase.NewDashboardUseCase(emptyDashboardRepository{})
	for _, tz := range []string{"Asia/Ho_Chi_Minh", "UTC"} {
		chart, err := uc.StockMovementChart(context.Background(), dto.StockMovementChartQuery{
			From: "2026-11-01", To: "2026-11-02", Timezone: tz,
		})
		if err != nil {
			t.Errorf("tz=%s: %v", tz, err)
			continue
		}
		if chart.Timezone != tz || len(chart.Points) != 2 {
			t.Errorf("tz=%s: chart timezone %q with %d points, want %q with 2", tz, chart.Timezone, len(chart.Points), tz)
		}
	}
}

// emptyDashboardRepository has no products and no movements.
type emptyDashboardRepository struct{}

func (emptyDashboardRepository) InventorySummary(context.Context) (domain.InventorySummary, error) {
	return domain.InventorySummary{}, nil
}

func (emptyDashboardRepository) StockMovementBuckets(context.Context, string, string, time.Time, time.Time) ([]domain.StockMovementBucket, error) {
	return []domain.StockMovementBucket{}, nil
}
