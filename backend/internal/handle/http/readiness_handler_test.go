package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

// GET /ready is a readiness probe: it pings PostgreSQL. sqlmock stands in for
// the database, so no real connection is made.

const (
	readyBody    = `{"success":true,"message":"service is ready","data":{"database":"connected","status":"ready"}}`
	notReadyBody = `{"success":false,"message":"database disconnected","data":null}`
)

// newPingDB returns a database whose next Ping succeeds (pingErr nil) or fails
// with pingErr. Any other call, including a second Ping, fails.
func newPingDB(t *testing.T, pingErr error) (*ReadinessHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ping := mock.ExpectPing()
	if pingErr != nil {
		ping.WillReturnError(pingErr)
	}
	return NewReadinessHandler(db), mock
}

// captureLogs sends the default slog logger to a JSON buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// logRecords decodes every JSON log line in buf.
func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

func TestReadinessHandler_DatabaseReachable(t *testing.T) {
	logs := captureLogs(t)
	handler, mock := newPingDB(t, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	contractAssertResponse(t, rec, http.StatusOK, readyBody)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database was not pinged: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("a successful readiness check must not log, got %s", logs.String())
	}
}

// A failed ping answers 503 with the generic body and logs why on the server,
// without the driver's error text: pgconn.ConnectError names the user and the
// database, so only the SQLSTATE and a reason are logged (the convention of
// internal/platform/database preflightQueryError).
func TestReadinessHandler_DatabaseUnreachable(t *testing.T) {
	const secretUser, secretHost = "secret_ready_user", "db.internal.example"
	serverErr := &pgconn.PgError{Code: "28P01", Message: `password authentication failed for user "` + secretUser + `"`}
	pingErr := fmt.Errorf("failed to connect to `user=%s database=gostock host=%s`: %w", secretUser, secretHost, serverErr)

	logs := captureLogs(t)
	handler, mock := newPingDB(t, pingErr)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.Header.Set("X-Request-ID", "req-ready-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	contractAssertResponse(t, rec, http.StatusServiceUnavailable, notReadyBody)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database was not pinged: %v", err)
	}

	records := logRecords(t, logs)
	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1: %s", len(records), logs.String())
	}
	got := records[0]
	want := map[string]any{
		"level":      "ERROR",
		"msg":        "readiness check failed: database ping",
		"request_id": "req-ready-42",
		"reason":     "unavailable",
		"sqlstate":   "28P01",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("log %s = %v, want %v (record %v)", key, got[key], value, got)
		}
	}
	for _, secret := range []string{secretUser, secretHost, "password authentication", "failed to connect"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("log exposes %q: %s", secret, logs.String())
		}
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("response exposes %q: %s", secret, rec.Body.String())
		}
	}
}

func TestReadinessHandler_PingFailureReasons(t *testing.T) {
	t.Run("ping timeout", func(t *testing.T) {
		logs := captureLogs(t)
		handler, _ := newPingDB(t, context.DeadlineExceeded)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

		contractAssertResponse(t, rec, http.StatusServiceUnavailable, notReadyBody)
		records := logRecords(t, logs)
		if len(records) != 1 || records[0]["reason"] != "timeout" {
			t.Fatalf("log = %s, want one record with reason timeout", logs.String())
		}
		for _, absent := range []string{"request_id", "sqlstate"} {
			if _, ok := records[0][absent]; ok {
				t.Errorf("log has %s without a source for it: %v", absent, records[0])
			}
		}
	})

	t.Run("request cancelled", func(t *testing.T) {
		logs := captureLogs(t)
		handler, _ := newPingDB(t, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))

		contractAssertResponse(t, rec, http.StatusServiceUnavailable, notReadyBody)
		records := logRecords(t, logs)
		if len(records) != 1 || records[0]["reason"] != "canceled" {
			t.Fatalf("log = %s, want one record with reason canceled", logs.String())
		}
	})

	t.Run("other error without SQLSTATE", func(t *testing.T) {
		logs := captureLogs(t)
		handler, _ := newPingDB(t, fmt.Errorf("dial tcp 10.0.0.5:5432: connection refused"))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

		contractAssertResponse(t, rec, http.StatusServiceUnavailable, notReadyBody)
		records := logRecords(t, logs)
		if len(records) != 1 || records[0]["reason"] != "unavailable" {
			t.Fatalf("log = %s, want one record with reason unavailable", logs.String())
		}
		if _, ok := records[0]["sqlstate"]; ok {
			t.Errorf("log has a sqlstate for a non-server error: %v", records[0])
		}
		if strings.Contains(logs.String(), "10.0.0.5") {
			t.Errorf("log exposes the server address: %s", logs.String())
		}
	})
}

// Only GET is served; any other method is refused before the database is
// touched (the mock fails any unexpected ping).
func TestReadinessHandler_RejectsOtherMethods(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			rec := httptest.NewRecorder()
			NewReadinessHandler(db).ServeHTTP(rec, httptest.NewRequest(method, "/ready", nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405; body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Allow"); got != http.MethodGet {
				t.Errorf("Allow = %q, want %q", got, http.MethodGet)
			}
			if method != http.MethodHead {
				contractAssertResponse(t, rec, http.StatusMethodNotAllowed,
					`{"success":false,"message":"method not allowed","data":null}`)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected database activity: %v", err)
			}
		})
	}
}
