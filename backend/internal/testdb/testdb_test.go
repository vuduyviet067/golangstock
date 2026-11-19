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
		{name: "integration test database", dbName: "gostock_test"},
		{name: "UI test database", dbName: "gostock_ui_test", wantErr: true},
		{name: "other _test database", dbName: "inventory_test", wantErr: true},
		{name: "development database", dbName: "gostock", wantErr: true},
		{name: "system database", dbName: "postgres", wantErr: true},
		{name: "empty", dbName: "", wantErr: true},
		{name: "suffix only", dbName: "_test", wantErr: true},
		{name: "longer name", dbName: "gostock_testing", wantErr: true},
		{name: "prefixed name", dbName: "old_gostock_test", wantErr: true},
		{name: "without underscore", dbName: "gostocktest", wantErr: true},
		{name: "case sensitive", dbName: "GOSTOCK_TEST", wantErr: true},
		{name: "leading space", dbName: " gostock_test", wantErr: true},
		{name: "trailing space", dbName: "gostock_test ", wantErr: true},
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

// openWithRecorder runs Open with a recorder in its own goroutine, so that
// Skipf and Fatalf end only that goroutine, and reports what Open did.
func openWithRecorder(t *testing.T) *skipRecorder {
	t.Helper()
	rec := &skipRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Open(rec)
		t.Error("Open returned a database; want it to skip or fail")
	}()
	<-done
	return rec
}

func TestOpen_SkipsWithoutTestDatabaseURLAndIgnoresDatabaseURL(t *testing.T) {
	t.Setenv(EnvVar, "")
	// Not required: CI sets REQUIRE_TEST_DATABASE for the whole run.
	t.Setenv(RequireEnvVar, "")
	// DATABASE_URL must never be used as a fallback, even when it is set.
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/gostock?sslmode=disable")

	rec := openWithRecorder(t)
	if !rec.skipped || rec.failed {
		t.Fatalf("skipped = %v, failed = %v; want the test skipped without a failure", rec.skipped, rec.failed)
	}
}

// With REQUIRE_TEST_DATABASE set to false the tests are optional, as when it
// is unset.
func TestOpen_SkipsWhenExplicitlyNotRequired(t *testing.T) {
	for _, value := range []string{"false", "0", " FALSE "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvVar, "")
			t.Setenv(RequireEnvVar, value)

			rec := openWithRecorder(t)
			if !rec.skipped || rec.failed {
				t.Fatalf("skipped = %v, failed = %v; want the test skipped", rec.skipped, rec.failed)
			}
		})
	}
}

// When the integration tests are required, a missing TEST_DATABASE_URL fails
// them: a CI run without a database must not pass on skipped tests.
func TestOpen_FailsWithoutTestDatabaseURLWhenRequired(t *testing.T) {
	for _, value := range []string{"true", "1", "TRUE", " true "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvVar, "")
			t.Setenv(RequireEnvVar, value)
			t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/gostock?sslmode=disable")

			rec := openWithRecorder(t)
			if !rec.failed || rec.skipped {
				t.Fatalf("failed = %v, skipped = %v; want the test failed, not skipped", rec.failed, rec.skipped)
			}
		})
	}
}

// A value that is not a boolean fails rather than being read as "not
// required", even when TEST_DATABASE_URL is missing.
func TestOpen_FailsOnInvalidRequireValue(t *testing.T) {
	for _, value := range []string{"yes", "on", "required", "2"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvVar, "")
			t.Setenv(RequireEnvVar, value)

			rec := openWithRecorder(t)
			if !rec.failed || rec.skipped {
				t.Fatalf("failed = %v, skipped = %v; want the test failed, not skipped", rec.failed, rec.skipped)
			}
		})
	}
}

// A TEST_DATABASE_URL that cannot be reached fails, required or not; it is
// never a skip. Port 1 on the loopback address refuses the connection.
func TestOpen_FailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	for _, value := range []string{"", "true"} {
		t.Run("required="+value, func(t *testing.T) {
			t.Setenv(EnvVar, "postgres://nobody@127.0.0.1:1/gostock_test?sslmode=disable&connect_timeout=2")
			t.Setenv(RequireEnvVar, value)

			rec := openWithRecorder(t)
			if !rec.failed || rec.skipped {
				t.Fatalf("failed = %v, skipped = %v; want the test failed, not skipped", rec.failed, rec.skipped)
			}
		})
	}
}

func TestRequired(t *testing.T) {
	tests := []struct {
		value   string
		want    bool
		wantErr bool
	}{
		{value: "", want: false},
		{value: "   ", want: false},
		{value: "true", want: true},
		{value: "1", want: true},
		{value: "false", want: false},
		{value: "0", want: false},
		{value: "yes", wantErr: true},
		{value: "enabled", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(RequireEnvVar, tt.value)
			got, err := Required()
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("Required() = %v, %v; want %v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
