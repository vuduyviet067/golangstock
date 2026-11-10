package testdb

import (
	"runtime"
	"testing"
)

func TestCheckName(t *testing.T) {
	tests := []struct {
		name    string
		dbName  string
		wantErr bool
	}{
		{name: "test database", dbName: "gostock_test"},
		{name: "other test database", dbName: "inventory_test"},
		{name: "development database", dbName: "gostock", wantErr: true},
		{name: "system database", dbName: "postgres", wantErr: true},
		{name: "empty", dbName: "", wantErr: true},
		{name: "suffix only", dbName: "_test", wantErr: true},
		{name: "suffix not at end", dbName: "gostock_testing", wantErr: true},
		{name: "suffix without underscore", dbName: "gostocktest", wantErr: true},
		{name: "case sensitive", dbName: "gostock_TEST", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckName(tt.dbName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckName(%q) error = %v, wantErr %v", tt.dbName, err, tt.wantErr)
			}
		})
	}
}

// skipRecorder records Skipf and Fatalf instead of ending the real test.
type skipRecorder struct {
	testing.TB
	skipped bool
	failed  bool
}

func (r *skipRecorder) Helper()          {}
func (r *skipRecorder) Cleanup(f func()) {}

func (r *skipRecorder) Skipf(string, ...any) {
	r.skipped = true
	runtime.Goexit()
}

func (r *skipRecorder) Fatalf(string, ...any) {
	r.failed = true
	runtime.Goexit()
}

func TestOpen_SkipsWithoutTestDatabaseURLAndIgnoresDatabaseURL(t *testing.T) {
	t.Setenv(EnvVar, "")
	// DATABASE_URL must never be used as a fallback, even when it is set.
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/gostock?sslmode=disable")

	rec := &skipRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Open(rec)
		t.Error("Open returned a database; want it to skip")
	}()
	<-done

	if !rec.skipped || rec.failed {
		t.Fatalf("skipped = %v, failed = %v; want the test skipped without a failure", rec.skipped, rec.failed)
	}
}
