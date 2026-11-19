package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// Characterization test of the Auth and User Management contract against the
// real router, real PostgreSQL repositories (gostock_test only, via testdb)
// and the real JWT service.
//
// Dynamic values (IDs, created_at/updated_at set by PostgreSQL, JWT and
// refresh tokens) are handled as follows:
//   - IDs and timestamps are read back from the users table of the fixture
//     rows and spliced into otherwise literal expected bodies, so the whole
//     body is still compared byte for byte.
//   - Each timestamp field is also matched against a format regexp, so the
//     Auth (RFC3339, seconds) and Users (RFC3339Nano) rules stay pinned
//     independently of the actual clock value.
//   - Token cookie values are matched by shape only and must never appear in
//     a body.
//
// All fixture rows share a unique email prefix; they are deleted on cleanup
// (refresh_tokens cascade), and list queries filter on that prefix so rows
// from other tests never affect the result.

var (
	pgContractAuthTime  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})$`)
	pgContractUsersTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$`)
	pgContractJWT       = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
	pgContractRefresh   = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

const pgContractClearedExpires = "Expires=Thu, 01 Jan 1970 00:00:01 GMT"

type pgContractUserRow struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func pgContractLoadUser(t *testing.T, db *sql.DB, email string) pgContractUserRow {
	t.Helper()
	var u pgContractUserRow
	err := db.QueryRowContext(context.Background(),
		`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		t.Fatalf("load fixture user %q: %v", email, err)
	}
	if u.PasswordHash == "" {
		t.Fatalf("fixture user %q has no password hash", email)
	}
	return u
}

// pgContractAuthJSON is the expected dto.AuthUserResponse body: created_at is
// RFC3339 to the second.
func pgContractAuthJSON(message string, u pgContractUserRow, createdAt string) string {
	return fmt.Sprintf(`{"success":true,"message":%q,"data":{"id":%d,"email":%q,"role":%q,"created_at":%q}}`,
		message, u.ID, u.Email, u.Role, createdAt)
}

// pgContractUserItem is one expected dto.UserResponse object: timestamps are
// time.Time JSON (RFC3339Nano).
func pgContractUserItem(u pgContractUserRow) string {
	return fmt.Sprintf(`{"id":%d,"email":%q,"role":%q,"created_at":%q,"updated_at":%q}`,
		u.ID, u.Email, u.Role, u.CreatedAt.Format(time.RFC3339Nano), u.UpdatedAt.Format(time.RFC3339Nano))
}

func pgContractAssertBody(t *testing.T, step string, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Errorf("%s: status = %d, want %d", step, rec.Code, wantStatus)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s: Content-Type = %q, want application/json", step, ct)
	}
	if got := rec.Body.String(); got != wantBody+"\n" {
		t.Errorf("%s: body mismatch\n got: %s\nwant: %s", step, strings.TrimSuffix(got, "\n"), wantBody)
	}
}

func pgContractAssertNoSecrets(t *testing.T, step, body string, secrets ...string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, key := range []string{"password", "password_hash", "passwordhash", "token", "access_token", "refresh_token"} {
		if strings.Contains(lower, `"`+key+`":`) {
			t.Errorf("%s: body must not contain field %q: %s", step, key, body)
		}
	}
	if strings.Contains(lower, "hash") {
		t.Errorf("%s: body must not mention a hash: %s", step, body)
	}
	for _, s := range secrets {
		if s != "" && strings.Contains(body, s) {
			t.Errorf("%s: body leaks secret value: %s", step, body)
		}
	}
}

// pgContractTimeFields checks that each named field of data (or of every
// data.items[i]) is a string matching re.
func pgContractTimeFields(t *testing.T, step string, body []byte, re *regexp.Regexp, fields ...string) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("%s: decode body: %v", step, err)
	}
	var objects []map[string]any
	var single map[string]any
	if err := json.Unmarshal(env.Data, &single); err == nil {
		if items, ok := single["items"].([]any); ok {
			for _, it := range items {
				objects = append(objects, it.(map[string]any))
			}
		} else {
			objects = append(objects, single)
		}
	}
	if len(objects) == 0 {
		t.Fatalf("%s: no data objects to check", step)
	}
	for _, obj := range objects {
		for _, f := range fields {
			s, ok := obj[f].(string)
			if !ok || !re.MatchString(s) {
				t.Errorf("%s: %s = %#v, want string matching %s", step, f, obj[f], re)
			}
		}
	}
}

func pgContractIssuedCookies(t *testing.T, step string, rec *httptest.ResponseRecorder) (access, refresh *http.Cookie) {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("%s: got %d cookies, want access_token and refresh_token", step, len(cookies))
	}
	access, refresh = cookies[0], cookies[1]

	for _, c := range []struct {
		cookie        *http.Cookie
		name, path    string
		maxAge        int
		valueShape    *regexp.Regexp
		valueShapeTag string
	}{
		{access, productHTTP.AccessTokenCookieName, "/", 900, pgContractJWT, "JWT"},
		{refresh, productHTTP.RefreshTokenCookieName, "/api/v1/auth", 604800, pgContractRefresh, "43-char base64url"},
	} {
		got := c.cookie
		if got.Name != c.name || got.Path != c.path || got.MaxAge != c.maxAge || !got.HttpOnly || got.Secure || got.SameSite != http.SameSiteLaxMode {
			t.Errorf("%s: cookie = {Name:%q Path:%q MaxAge:%d HttpOnly:%v Secure:%v SameSite:%v}, want {Name:%q Path:%q MaxAge:%d HttpOnly:true Secure:false SameSite:Lax}",
				step, got.Name, got.Path, got.MaxAge, got.HttpOnly, got.Secure, got.SameSite, c.name, c.path, c.maxAge)
		}
		if !c.valueShape.MatchString(got.Value) {
			t.Errorf("%s: %s value is not a %s", step, c.name, c.valueShapeTag)
		}
	}
	return access, refresh
}

func TestAuthUserContract_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	var dbName string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
		t.Fatalf("current_database(): %v", err)
	}
	if dbName != testdb.RequiredName {
		t.Fatalf("connected to %q, want %q", dbName, testdb.RequiredName)
	}
	t.Logf("integration database: %s", dbName)

	jwtService, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}
	userRepo := postgres.NewUserRepository(db)
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), jwtService, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	router := NewRouter(
		db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db))),
		productHTTP.NewAuthHandler(authUC, productHTTP.NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)),
		authUC,
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		nil,
		nil,
	)

	tag := fmt.Sprintf("mapper-contract-%d", time.Now().UnixNano())
	memberEmail := tag + "-member@example.com"
	adminEmail := tag + "-admin@example.com"
	renamedEmail := tag + "-renamed@example.com"
	const password = "ContractPass123!"

	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE email LIKE $1`, tag+"-%"); err != nil {
			t.Errorf("cleanup fixture users: %v", err)
		}
	})

	serve := func(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		for _, c := range cookies {
			req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	credentials := func(email string) string {
		return fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	}

	// --- Register -----------------------------------------------------------
	rec := serve(http.MethodPost, "/api/v1/auth/register", credentials(strings.ToUpper(memberEmail)))
	member := pgContractLoadUser(t, db, memberEmail)
	pgContractAssertBody(t, "register", rec, http.StatusCreated,
		pgContractAuthJSON("User registered successfully", member, member.CreatedAt.Format(time.RFC3339)))
	pgContractTimeFields(t, "register", rec.Body.Bytes(), pgContractAuthTime, "created_at")
	pgContractAssertNoSecrets(t, "register", rec.Body.String(), member.PasswordHash, password)
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("register: Set-Cookie = %q, want none", got)
	}
	if member.Role != "user" {
		t.Fatalf("registered role = %q, want user", member.Role)
	}

	rec = serve(http.MethodPost, "/api/v1/auth/register", credentials(memberEmail))
	pgContractAssertBody(t, "register duplicate", rec, http.StatusConflict, `{"success":false,"code":"EMAIL_EXISTS","message":"email already exists","data":null}`)

	rec = serve(http.MethodPost, "/api/v1/auth/register", credentials(adminEmail))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register admin fixture: status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE email = $1`, adminEmail); err != nil {
		t.Fatalf("promote admin fixture: %v", err)
	}
	admin := pgContractLoadUser(t, db, adminEmail)

	// --- Login --------------------------------------------------------------
	rec = serve(http.MethodPost, "/api/v1/auth/login", credentials(memberEmail))
	pgContractAssertBody(t, "login", rec, http.StatusOK,
		pgContractAuthJSON("Login successful", member, member.CreatedAt.Format(time.RFC3339)))
	pgContractTimeFields(t, "login", rec.Body.Bytes(), pgContractAuthTime, "created_at")
	memberAccess, memberRefresh := pgContractIssuedCookies(t, "login", rec)
	pgContractAssertNoSecrets(t, "login", rec.Body.String(), member.PasswordHash, password, memberAccess.Value, memberRefresh.Value)

	rec = serve(http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"WrongPass123!"}`, memberEmail))
	pgContractAssertBody(t, "login wrong password", rec, http.StatusUnauthorized, `{"success":false,"code":"INVALID_CREDENTIALS","message":"invalid email or password","data":null}`)

	rec = serve(http.MethodPost, "/api/v1/auth/login", credentials(adminEmail))
	if rec.Code != http.StatusOK {
		t.Fatalf("login admin fixture: status %d: %s", rec.Code, rec.Body.String())
	}
	pgContractAssertBody(t, "login admin", rec, http.StatusOK,
		pgContractAuthJSON("Login successful", admin, admin.CreatedAt.Format(time.RFC3339)))
	adminAccess, _ := pgContractIssuedCookies(t, "login admin", rec)

	// --- Me -----------------------------------------------------------------
	// The middleware rebuilds the user from JWT claims only, so created_at is
	// "" even though the row has a created_at.
	rec = serve(http.MethodGet, "/api/v1/auth/me", "", memberAccess)
	pgContractAssertBody(t, "me", rec, http.StatusOK,
		pgContractAuthJSON("User profile retrieved successfully", member, ""))
	pgContractAssertNoSecrets(t, "me", rec.Body.String(), member.PasswordHash, memberAccess.Value)

	rec = serve(http.MethodGet, "/api/v1/auth/me", "")
	pgContractAssertBody(t, "me unauthenticated", rec, http.StatusUnauthorized, `{"success":false,"code":"UNAUTHENTICATED","message":"authentication required","data":null}`)

	// --- Refresh ------------------------------------------------------------
	rec = serve(http.MethodPost, "/api/v1/auth/refresh", "", memberRefresh)
	pgContractAssertBody(t, "refresh", rec, http.StatusOK,
		pgContractAuthJSON("Token refreshed successfully", member, member.CreatedAt.Format(time.RFC3339)))
	pgContractTimeFields(t, "refresh", rec.Body.Bytes(), pgContractAuthTime, "created_at")
	rotatedAccess, rotatedRefresh := pgContractIssuedCookies(t, "refresh", rec)
	if rotatedRefresh.Value == memberRefresh.Value {
		t.Error("refresh: refresh token was not rotated")
	}
	pgContractAssertNoSecrets(t, "refresh", rec.Body.String(), member.PasswordHash, rotatedAccess.Value, rotatedRefresh.Value, memberRefresh.Value)

	rec = serve(http.MethodPost, "/api/v1/auth/refresh", "")
	pgContractAssertBody(t, "refresh without cookie", rec, http.StatusUnauthorized, `{"success":false,"code":"UNAUTHENTICATED","message":"refresh token required","data":null}`)

	// --- Users: authorization ----------------------------------------------
	rec = serve(http.MethodGet, "/api/v1/users", "", memberAccess)
	pgContractAssertBody(t, "users as member", rec, http.StatusForbidden, `{"success":false,"code":"FORBIDDEN","message":"forbidden: admin role required","data":null}`)

	// --- Users: List --------------------------------------------------------
	rec = serve(http.MethodGet, "/api/v1/users?page=1&limit=10&q="+tag, "", adminAccess)
	pgContractAssertBody(t, "users list", rec, http.StatusOK,
		`{"success":true,"message":"Users retrieved successfully","data":{"items":[`+
			pgContractUserItem(member)+`,`+pgContractUserItem(admin)+
			`],"pagination":{"page":1,"limit":10,"total":2,"total_pages":1}}}`)
	pgContractTimeFields(t, "users list", rec.Body.Bytes(), pgContractUsersTime, "created_at", "updated_at")
	pgContractAssertNoSecrets(t, "users list", rec.Body.String(), member.PasswordHash, admin.PasswordHash)

	rec = serve(http.MethodGet, "/api/v1/users?page=2&limit=1&q="+tag, "", adminAccess)
	pgContractAssertBody(t, "users list page 2", rec, http.StatusOK,
		`{"success":true,"message":"Users retrieved successfully","data":{"items":[`+
			pgContractUserItem(admin)+
			`],"pagination":{"page":2,"limit":1,"total":2,"total_pages":2}}}`)

	rec = serve(http.MethodGet, "/api/v1/users?q="+tag+"-nobody", "", adminAccess)
	pgContractAssertBody(t, "users list empty", rec, http.StatusOK,
		`{"success":true,"message":"Users retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}}}`)

	// --- Users: GetByID -----------------------------------------------------
	rec = serve(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", member.ID), "", adminAccess)
	pgContractAssertBody(t, "users get", rec, http.StatusOK,
		`{"success":true,"message":"User retrieved successfully","data":`+pgContractUserItem(member)+`}`)
	pgContractTimeFields(t, "users get", rec.Body.Bytes(), pgContractUsersTime, "created_at", "updated_at")
	pgContractAssertNoSecrets(t, "users get", rec.Body.String(), member.PasswordHash)

	rec = serve(http.MethodGet, "/api/v1/users/9223372036854775807", "", adminAccess)
	pgContractAssertBody(t, "users get missing", rec, http.StatusNotFound, `{"success":false,"code":"USER_NOT_FOUND","message":"user not found","data":null}`)

	// --- Users: Update ------------------------------------------------------
	rec = serve(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", member.ID), fmt.Sprintf(`{"email":%q}`, strings.ToUpper(renamedEmail)), adminAccess)
	renamed := pgContractLoadUser(t, db, renamedEmail)
	if renamed.ID != member.ID || !renamed.CreatedAt.Equal(member.CreatedAt) {
		t.Fatalf("update changed identity: before %+v, after %+v", member, renamed)
	}
	pgContractAssertBody(t, "users update", rec, http.StatusOK,
		`{"success":true,"message":"User updated successfully","data":`+pgContractUserItem(renamed)+`}`)
	pgContractTimeFields(t, "users update", rec.Body.Bytes(), pgContractUsersTime, "created_at", "updated_at")
	pgContractAssertNoSecrets(t, "users update", rec.Body.String(), renamed.PasswordHash)

	rec = serve(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", member.ID), fmt.Sprintf(`{"email":%q}`, adminEmail), adminAccess)
	pgContractAssertBody(t, "users update duplicate", rec, http.StatusConflict, `{"success":false,"code":"EMAIL_EXISTS","message":"email already exists","data":null}`)

	// --- Logout -------------------------------------------------------------
	rec = serve(http.MethodPost, "/api/v1/auth/logout", "", rotatedRefresh)
	pgContractAssertBody(t, "logout", rec, http.StatusOK, `{"success":true,"message":"logged out","data":{"message":"logged out"}}`)
	wantCleared := []string{
		"access_token=; Path=/; " + pgContractClearedExpires + "; Max-Age=0; HttpOnly; SameSite=Lax",
		"refresh_token=; Path=/api/v1/auth; " + pgContractClearedExpires + "; Max-Age=0; HttpOnly; SameSite=Lax",
	}
	if got := rec.Header().Values("Set-Cookie"); strings.Join(got, "\n") != strings.Join(wantCleared, "\n") {
		t.Errorf("logout: Set-Cookie = %q, want %q", got, wantCleared)
	}

	rec = serve(http.MethodPost, "/api/v1/auth/refresh", "", rotatedRefresh)
	pgContractAssertBody(t, "refresh after logout", rec, http.StatusUnauthorized, `{"success":false,"code":"INVALID_REFRESH_TOKEN","message":"invalid or expired refresh token","data":null}`)
}
