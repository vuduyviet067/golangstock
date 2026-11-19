# GoStock — Inventory Management API

GoStock is a backend REST API for inventory management, built with Go and PostgreSQL.

The project provides product and inventory management, user authentication, role-based authorization, and database migration management. Its backend is organized around Clean Architecture principles to separate business logic from HTTP delivery and database persistence.

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go |
| HTTP server and routing | Go standard library (`net/http`) server, [chi](https://github.com/go-chi/chi) v5 router |
| Database | PostgreSQL 17 |
| Database access | `database/sql` |
| PostgreSQL driver | pgx v5 |
| Authentication | JWT access tokens and persisted refresh tokens |
| Password hashing | bcrypt |
| Database migrations | golang-migrate |
| Testing | Go `testing`, `net/http/httptest` |

## Features

### Product and inventory management

- Create, retrieve, update, and delete products.
- Search products by keyword.
- Paginate and sort product lists.
- Stock-in and stock-out operations.
- Validate product data and inventory quantities.
- Prevent negative inventory through database-level conditional updates.
- Handle missing products and insufficient stock.
- Protect inventory updates against concurrent requests.

### Authentication and authorization

- User registration and login.
- Password hashing with bcrypt.
- JWT-based access authentication.
- Refresh-token persistence, rotation, and revocation.
- Authentication through HttpOnly cookies.
- Logout and authenticated-user information.
- Role-based access control with `user` and `admin` roles.

### User administration

- List users with pagination and keyword search.
- Retrieve user details.
- Update user information and roles.
- Restrict user-management operations to administrators.
- Provide a server-side `gostock admin` command to create an administrator or reset an administrator's password.

### Application and security configuration

- Environment-specific configuration validation.
- CORS origin allowlist.
- Secure cookie configuration.
- JSON request validation and request-body size limits.
- Standardized API response envelopes.
- HTTP server timeouts and graceful shutdown.
- Liveness and database readiness endpoints.

## Architecture

The backend follows Clean Architecture principles.

```text
HTTP Handler
     |
     v
  UseCase
     |
     v
Repository Interface
     ^
     |
PostgreSQL Repository
     |
     v
 PostgreSQL
```

The main responsibilities are:

| Layer | Responsibility |
|---|---|
| Domain | Core entities, business-related types, and domain errors |
| UseCase | Application business rules and workflows |
| Repository | Persistence interfaces |
| Repository/PostgreSQL | PostgreSQL implementations of repository interfaces |
| Handler/HTTP | HTTP handlers, middleware, request validation, and response mapping |
| Platform | Infrastructure services such as PostgreSQL connection and JWT |
| App | Dependency composition, routing, server lifecycle, and graceful shutdown |
| Config | Environment configuration and validation |

The dependency direction keeps business logic independent of HTTP and PostgreSQL implementation details.

## Project Structure

```text
gostock/
├── backend/
│   ├── config/
│   │   ├── config.go
│   │   ├── config_test.go
│   │   ├── origin.go
│   │   └── origin_test.go
│   ├── internal/
│   │   ├── app/
│   │   │   ├── app.go
│   │   │   └── router.go
│   │   ├── domain/
│   │   ├── handle/
│   │   │   └── http/
│   │   │       └── ... handlers and middleware
│   │   ├── platform/
│   │   │   ├── database/
│   │   │   └── jwt/
│   │   ├── repository/
│   │   │   └── postgres/
│   │   └── usecase/
│   ├── migrations/
│   ├── pkg/
│   │   ├── phone/
│   │   └── response/
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── .env.example
├── .gitignore
└── README.md
```

## API Overview

All application endpoints use the `/api/v1` prefix, except for health and readiness endpoints.

### Health and readiness

| Method | Endpoint | Description | Authentication |
|---|---|---|---|
| GET | `/health` | Application liveness check | Public |
| GET | `/ready` | Database readiness check | Public |

`/health` reports whether the application HTTP process is responding. `/ready` checks database connectivity.

### Authentication

| Method | Endpoint | Description | Authentication |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Register a new user; only when `ALLOW_REGISTRATION=true`, otherwise `403 REGISTRATION_DISABLED` | Public |
| POST | `/api/v1/auth/login` | Log in | Public |
| POST | `/api/v1/auth/refresh` | Refresh authentication tokens | Refresh cookie |
| POST | `/api/v1/auth/logout` | Log out and revoke refresh token | Refresh cookie |
| GET | `/api/v1/auth/me` | Get the authenticated user | Authenticated |
| POST | `/api/v1/auth/change-password` | Change the authenticated user's own password | Authenticated (any role) |

Access and refresh tokens are delivered through HttpOnly cookies. The access token is a JWT; refresh tokens are persisted to support rotation and revocation.

#### Change password

`POST /api/v1/auth/change-password` with `Content-Type: application/json`:

```json
{ "current_password": "the current password", "new_password": "the new password" }
```

The new password follows the registration rules (8 to 72 bytes) and must differ from the current one. On success the response is `200` with `{"success":true,"message":"password changed","data":null}`; the new password hash is stored and **every refresh token of the user is revoked** in one transaction, and both auth cookies are cleared as on logout, so every session of the user (on every device) must log in again with the new password.

| Status | Code | When |
|---|---|---|
| 400 | `INVALID_CURRENT_PASSWORD` | `current_password` is wrong. It is not a 401, so the frontend does not try to refresh the session |
| 400 | `PASSWORD_UNCHANGED` | `new_password` equals the current password |
| 400 | `INVALID_PASSWORD` | `new_password` is shorter than 8 or longer than 72 bytes |
| 400 | `VALIDATION_ERROR` | Malformed JSON, an unknown field or more than one JSON object |
| 401 | `UNAUTHENTICATED` | No valid access token |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | `Content-Type` is not `application/json` |

A refused change keeps the session and its cookies. Known limitation: an access token issued before the change stays valid until it expires, at most 15 minutes (`ACCESS_TOKEN_TTL_MINUTES`), because access tokens are stateless JWTs that are not checked against the database; only the refresh tokens are revoked at once.

### Products and inventory

| Method | Endpoint | Description | Authorization |
|---|---|---|---|
| GET | `/api/v1/products` | List, search, paginate, and sort products | User or Admin |
| GET | `/api/v1/products/{id}` | Get product details | User or Admin |
| POST | `/api/v1/products` | Create a product | Admin |
| PUT | `/api/v1/products/{id}` | Update a product | Admin |
| DELETE | `/api/v1/products/{id}` | Delete a product | Admin |
| POST | `/api/v1/products/{id}/stock-in` | Increase inventory | User or Admin |
| POST | `/api/v1/products/{id}/stock-out` | Decrease inventory | User or Admin |

Product listing supports query parameters such as:

- `q`: search keyword.
- `page`: page number.
- `limit`: page size.
- `sortBy`: sorting field.
- `order`: sorting direction.

Example:

```http
GET /api/v1/products?q=keyboard&page=1&limit=10&sortBy=name&order=asc
```

### Dashboard

| Method | Endpoint | Description | Authorization |
|---|---|---|---|
| GET | `/api/v1/dashboard/summary` | Summary cards and inventory status breakdown | User or Admin |
| GET | `/api/v1/dashboard/stock-movements` | Stock moved in and out per day or month | User or Admin |

The stock movement chart accepts these optional query parameters:

- `from`, `to`: calendar dates in `YYYY-MM-DD` format, inclusive. `to` defaults to today in `tz`; `from` defaults to 29 days before `to`.
- `granularity`: `auto` (default), `day` or `month`. `auto` groups by day for ranges up to 62 days and by month beyond. A range may span at most 92 days by day and 731 days by month.
- `tz`: the time zone used for the date range and the buckets. Defaults to `UTC`. Accepted values are `UTC` and IANA names of the form `Area/Location`, such as `Asia/Ho_Chi_Minh` or `America/New_York`. Abbreviations such as `CET`, `EST` or `GMT` are rejected.

The range covers whole local days, from 00:00 on `from` to the end of `to` in `tz`, including days shortened or lengthened by daylight saving time. Stock-out quantities are positive numbers, and movements of deleted products are included.

Query parameter values must be URL-encoded. In particular, `+` in a time zone name must be sent as `%2B`, because a bare `+` in a query string is decoded as a space:

```http
GET /api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-30&tz=Etc/GMT%2B7
```

Note that `Etc/GMT+7` is seven hours behind UTC (POSIX sign convention).

### User administration

All user-management endpoints require the Admin role.

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/v1/users` | List and search users |
| GET | `/api/v1/users/{id}` | Get user details |
| PATCH | `/api/v1/users/{id}` | Update user information and role |

Newly registered accounts receive the `user` role. Administrator accounts are created through the operator CLI.

### Wood types and suppliers

Catalogs of the timber workshop (migration `000009`). Every endpoint requires the Admin role: without a token the answer is 401 `UNAUTHENTICATED`, with a user token 403 `FORBIDDEN`.

| Method | Endpoint | Description | Success |
|---|---|---|---|
| GET | `/api/v1/wood-types` | List every wood type, sorted by name (no pagination) | 200 |
| POST | `/api/v1/wood-types` | Create a wood type: `{"name": "Gỗ lim"}` | 201 |
| PATCH | `/api/v1/wood-types/{id}` | Rename a wood type | 200 |
| DELETE | `/api/v1/wood-types/{id}` | Delete a wood type | 204 |
| GET | `/api/v1/suppliers` | List every supplier, sorted by name (no pagination) | 200 |
| POST | `/api/v1/suppliers` | Create a supplier: `{"name": "An Phát", "phone": "+84 368 122 137"}` | 201 |
| PATCH | `/api/v1/suppliers/{id}` | Edit a supplier's name and/or phone | 200 |
| DELETE | `/api/v1/suppliers/{id}` | Delete a supplier | 204 |

Rules:

- Names are trimmed and must have 1 to 100 characters (`INVALID_WOOD_TYPE` / `INVALID_SUPPLIER`). Names are not unique: two wood types may have the same name.
- A supplier's phone is optional and not unique. Omitted, `null` or `""` means no phone. When given it must be a Vietnamese number and is stored normalized: spaces, dots, dashes and parentheses are removed and `+84`/`84` becomes `0`, so `+84 368 122 137` is stored as `0368122137`. Otherwise the answer is 400 `INVALID_PHONE`.
- PATCH changes only the fields sent; an omitted or `null` field keeps its value. For a supplier, `"phone": ""` removes the stored number. A body with no field is 400 `EMPTY_UPDATE`.
- DELETE removes the row for good. A row that another record still references is not deleted: 409 `IN_USE`.
- An `{id}` that is not a positive integer is 400 `INVALID_WOOD_TYPE_ID` / `INVALID_SUPPLIER_ID`; an unknown one is 404 `WOOD_TYPE_NOT_FOUND` / `SUPPLIER_NOT_FOUND`.

Example responses:

```json
{"success":true,"message":"Supplier created successfully","data":{"id":1,"name":"An Phát","phone":"0368122137","created_at":"2026-11-20T08:00:00Z","updated_at":"2026-11-20T08:00:00Z"}}
{"success":false,"code":"INVALID_PHONE","message":"invalid Vietnamese phone number","data":null}
```

## Prerequisites

- Go installed.
- PostgreSQL 17 installed and running.
- An empty PostgreSQL database named `gostock`.
- `golang-migrate` CLI built with the PostgreSQL driver.

Install the migration CLI:

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

Make sure the Go binary directory is included in your system `PATH`.

## Getting Started

### 1. Create the database

Create an empty PostgreSQL database named `gostock`.

For example, using `psql`:

```sql
CREATE DATABASE gostock;
```

Do not create application tables manually. Database migrations manage the application schema.

### 2. Configure environment variables

Copy `.env.example` to `.env` at the project root.

Replace the database password placeholder with your local PostgreSQL credentials.

The application reads environment variables directly. It does not automatically load `.env`.

For Git Bash:

```bash
set -a
source .env
set +a
```

Alternatively, export `DATABASE_URL` directly:

```bash
export DATABASE_URL='postgres://postgres:YOUR_PASSWORD@localhost:5432/gostock?sslmode=disable'
```

Never commit real credentials or production secrets to Git.

### 3. Install Go dependencies

From the project root:

```bash
cd backend
go mod download
```

### 4. Run database migrations

Make sure `DATABASE_URL` is set, then run these commands from the `backend` directory:

```bash
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" version
```

A fresh database must run every migration, starting from `000001`.

After all migrations have been applied, the database contains:

| Table | Created or managed by |
|---|---|
| `users` | `000002_create_users` |
| `refresh_tokens` | `000004_create_refresh_tokens` |
| `products` | `000006_create_or_validate_products` |
| `stock_movements` | `000008_create_stock_movements` |
| `wood_types`, `suppliers` | `000009_create_wood_types_and_suppliers` |

The `sessions` table is legacy migration history: migration `000003` creates it and migration `000005` drops it. It does not exist after all migrations have run.

Migration `000001_baseline_products` is a baseline marker only; it does not create or modify the products table.

Migration `000009_create_wood_types_and_suppliers` creates the two catalogs with CHECK constraints that repeat the API rules: a name of 1 to 100 characters that is not blank, and a phone that is `NULL` or a normalized Vietnamese number (`^0[1-9][0-9]{8,9}$`). Its down migration drops both tables with their rows.

#### Existing products table validation

If a `products` table already exists, migration `000006` validates it against the expected schema instead of blindly replacing it.

If validation fails:

1. The migration reports schema mismatches.
2. It does not modify the existing table.
3. The migration version is marked dirty at version 6.
4. Fix the reported schema differences.
5. Reset the migration version to 5 and retry:

```bash
migrate -path migrations -database "$DATABASE_URL" force 5
migrate -path migrations -database "$DATABASE_URL" up
```

Only use `force 5` after fixing the reported differences. Do not use it to bypass schema validation.

#### Rollback behavior

Rolling back one step from version 6:

```bash
migrate -path migrations -database "$DATABASE_URL" down 1
```

intentionally does not drop the `products` table.

Migration `000006` may have adopted a pre-existing table containing application data. Its down migration therefore cannot safely distinguish an adopted table from one it created.

After rollback, the database remains at version 5 while `products` still exists.

Only drop the table manually when using a disposable database and after verifying that its data is no longer needed.

### 5. Create the first administrator

Newly registered accounts receive the `user` role. Administrators are created, and their passwords reset, with the `admin` command of the backend binary. It runs on the server only; there is no HTTP API for it.

```text
gostock admin create --email <email>           # create an admin account
gostock admin reset-password --email <email>   # set a new password for an admin account and revoke its sessions
gostock admin --help                           # usage
```

From the `backend` directory with the Go toolchain:

```bash
go run . admin create --email admin@example.com
```

With a built binary (`go build -o gostock .`), or inside the container that runs it (replace `<container>` and the binary path with those of your deployment):

```bash
./gostock admin reset-password --email admin@example.com
docker exec -it <container> /path/to/gostock admin reset-password --email admin@example.com
```

The command requires `DATABASE_URL` and an initialized database. When `EXPECTED_DATABASE_NAME` is set, it checks `current_database()` like the server and stops before asking for the password if it is connected to another database.

**The password is read from standard input only.** It is never accepted as a command-line argument or an environment variable: a password there would end up in the shell history, the process list or the container configuration. The command rejects any extra argument.

- In a terminal (`docker exec -it`), it asks for the password twice without echo; both entries must match.
- Without a terminal, it reads the first line of standard input and drops its line ending (`\n` or `\r\n`). Pipe it from a secret store or a protected file, never from `echo` with the password typed on the command line:

```bash
go run . admin create --email admin@example.com < /run/secrets/gostock_admin_password
docker exec -i <container> /path/to/gostock admin create --email admin@example.com < admin-password.txt
```

Passwords must be between 8 and 72 bytes. The command prints only the email address on success, never the password or its hash.

| Exit code | Meaning |
| --- | --- |
| `0` | Success (or `--help`) |
| `1` | The command failed: invalid email or password, email already registered, account not found, account not an admin, passwords not matching, empty password, database or preflight error |
| `2` | Invalid command line: unknown subcommand, missing `--email`, unknown flag or extra argument |

### 6. Start the API

From the `backend` directory:

```bash
go run .
```

The server listens on port `8080` by default.

Set `HTTP_PORT` to use another valid port.

The application initializes its dependencies, connects to PostgreSQL, starts the HTTP server, and handles graceful shutdown signals.

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `EXPECTED_DATABASE_NAME` | No | When set, the API server and the `gostock admin` command check `current_database()` on their own connection right after connecting and stop if it differs, before serving requests or asking for credentials. If set, it must not be empty or have leading or trailing whitespace |
| `APP_ENV` | No | Application environment; defaults to `development` |
| `ENV` | No | Fallback environment variable when `APP_ENV` is unset |
| `HTTP_PORT` | No | HTTP port; defaults to `8080` |
| `JWT_SECRET` | Conditional | JWT signing secret; at least 32 bytes |
| `JWT_ISSUER` | No | JWT issuer; defaults to `gostock` |
| `JWT_AUDIENCE` | No | JWT audience; defaults to `gostock-api` |
| `ACCESS_TOKEN_TTL_MINUTES` | No | Access token lifetime; defaults to 15 minutes |
| `REFRESH_TOKEN_TTL_DAYS` | No | Refresh token lifetime; defaults to 7 days |
| `REFRESH_TOKEN_GRACE_PERIOD` | No | Refresh-token rotation grace period; defaults to 15 seconds |
| `COOKIE_SECURE` | No | Whether authentication cookies require HTTPS. Defaults to `false` only when `APP_ENV` is `development` (the default), `dev` or `test`, and to `true` in every other environment (`staging`, `production`, `prod`). `false` is rejected in `production` and `prod` |
| `ALLOW_REGISTRATION` | No | Whether `POST /api/v1/auth/register` may create accounts: `true` or `false`, defaults to `false`. Any other value stops startup. When `false`, the endpoint answers `403 REGISTRATION_DISABLED`; administrators are created with `gostock admin create` |
| `COOKIE_SAMESITE` | No | Cookie SameSite policy: `lax`, `strict`, or `none` |
| `CORS_ALLOWED_ORIGINS` | Conditional | Comma-separated list of allowed browser origins |

Supported application environments:

- `development`
- `dev`
- `test`
- `staging`
- `production`
- `prod`

`APP_ENV` takes precedence over `ENV`.

### Security configuration

- In development and test environments, the application provides development defaults for JWT and CORS when the corresponding variables are unset.
- The development JWT secret must never be used in a deployed environment.
- Staging and production require an explicit `JWT_SECRET` and `CORS_ALLOWED_ORIGINS`.
- Generate a strong JWT secret using:

```bash
openssl rand -base64 48
```

- The JWT secret must contain at least 32 bytes.
- Deployed CORS origins must use HTTPS and must not contain wildcard hosts.
- Authentication cookies are `Secure` by default. Only the local environments (`development`, `dev`, `test`) default to `Secure=false`, so the API works over plain HTTP on localhost. `COOKIE_SECURE` overrides the default.

| `APP_ENV` | `COOKIE_SECURE` unset | `COOKIE_SECURE=true` | `COOKIE_SECURE=false` |
|---|---|---|---|
| `development` (default), `dev`, `test` | `Secure=false` | `Secure=true` | `Secure=false` |
| `staging` | `Secure=true` | `Secure=true` | `Secure=false` |
| `production`, `prod` | `Secure=true` | `Secure=true` | startup error |

- `COOKIE_SAMESITE=none` requires `Secure=true`, whether it comes from the default or from `COOKIE_SECURE`; otherwise startup fails.
- Self-registration is off unless `ALLOW_REGISTRATION=true`.

These defaults are intended for local development only. Configure explicit secrets, origins, and cookie settings before deployment.

## API Response Format

The API uses a standardized response envelope, written by the shared package `backend/pkg/response`:

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `success` | boolean | always | `true` for 2xx responses, `false` for errors |
| `code` | string | errors only | Machine-readable error code in `UPPER_SNAKE_CASE`; never present on success |
| `message` | string | always | Human-readable message; do not compare it in client code, use `code` |
| `data` | any | always | Payload; `null` on most errors, `{"fields": {...}}` on validation errors |

Successful response example:

```json
{
  "success": true,
  "message": "Product retrieved successfully",
  "data": {
    "id": 1,
    "sku": "KB-001",
    "name": "Mechanical Keyboard"
  }
}
```

Error response example:

```json
{
  "success": false,
  "code": "PRODUCT_NOT_FOUND",
  "message": "Product not found",
  "data": null
}
```

Validation error example (query parameters of the dashboard):

```json
{
  "success": false,
  "code": "VALIDATION_ERROR",
  "message": "invalid query parameters",
  "data": { "fields": { "tz": "must be an IANA time zone name" } }
}
```

Paginated responses include pagination metadata inside `data`.

HTTP status codes are used to distinguish successful requests, validation errors, authentication failures, authorization failures, missing resources, and internal errors. Within a status, `code` tells the cases apart.

### Error codes

General codes, used when an error has no more specific code:

| Code | HTTP | When |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Invalid request: malformed or extra JSON, a field rejected by the handler, or invalid query parameters (details in `data.fields`) |
| `UNAUTHENTICATED` | 401 | No valid access token (`authentication required`, `invalid or expired token`), no refresh cookie (`refresh token required`), or the authenticated user no longer exists |
| `FORBIDDEN` | 403 | Role not allowed (`forbidden: admin role required`) or cross-origin request rejected |
| `NOT_FOUND` | 404 | Resource not found, when no specific code applies |
| `METHOD_NOT_ALLOWED` | 405 | HTTP method not supported by the route; the `Allow` header lists the allowed methods |
| `CONFLICT` | 409 | Conflict, when no specific code applies |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | `Content-Type` is not `application/json` |
| `INTERNAL_ERROR` | 500 | Unexpected server error; the cause is logged, never returned |
| `SERVICE_UNAVAILABLE` | 503 | Readiness probe cannot reach the database |

Specific codes:

| Code | HTTP | Message |
| --- | --- | --- |
| `INVALID_PRODUCT_ID` | 400 | `invalid product ID` |
| `INVALID_PRODUCT` | 400 | `Invalid product` |
| `INVALID_QUANTITY` | 400 | `quantity must be greater than 0` |
| `INVALID_USER_ID` | 400 | `invalid user ID` |
| `INVALID_USER` | 400 | `invalid user` |
| `INVALID_USER_ROLE` | 400 | `invalid user role` |
| `EMPTY_UPDATE` | 400 | `at least one field must be provided for update` |
| `INVALID_PAGE` | 400 | `invalid page` |
| `INVALID_LIMIT` | 400 | `invalid limit` |
| `INVALID_EMAIL` | 400 | `invalid email` |
| `INVALID_PASSWORD` | 400 | `invalid password` |
| `INVALID_CURRENT_PASSWORD` | 400 | `current password is incorrect` (change-password) |
| `PASSWORD_UNCHANGED` | 400 | `new password must differ from the current password` (change-password) |
| `INVALID_WOOD_TYPE_ID` | 400 | `invalid wood type ID` |
| `INVALID_SUPPLIER_ID` | 400 | `invalid supplier ID` |
| `INVALID_WOOD_TYPE` | 400 | `wood type name must have 1 to 100 characters` |
| `INVALID_SUPPLIER` | 400 | `supplier name must have 1 to 100 characters` |
| `INVALID_PHONE` | 400 | `invalid Vietnamese phone number` |
| `INVALID_CREDENTIALS` | 401 | `invalid email or password` |
| `INVALID_SESSION` | 401 | `invalid or expired session` |
| `INVALID_REFRESH_TOKEN` | 401 | `invalid or expired refresh token` (one code for unknown, revoked, expired and reused tokens) |
| `REGISTRATION_DISABLED` | 403 | `registration is disabled` (`ALLOW_REGISTRATION` is not `true`) |
| `PRODUCT_NOT_FOUND` | 404 | `Product not found` |
| `USER_NOT_FOUND` | 404 | `user not found` |
| `WOOD_TYPE_NOT_FOUND` | 404 | `wood type not found` |
| `SUPPLIER_NOT_FOUND` | 404 | `supplier not found` |
| `SKU_EXISTS` | 409 | `SKU already exists` |
| `EMAIL_EXISTS` | 409 | `email already exists` |
| `INSUFFICIENT_STOCK` | 409 | `Insufficient stock` |
| `CONCURRENT_REFRESH` | 409 | `concurrent refresh in progress` |
| `IN_USE` | 409 | `the record is in use and cannot be deleted` (wood types, suppliers) |

Codes are assigned in the HTTP layer (`MapError` in `backend/internal/handle/http/error_mapper.go` and the handlers), never in use cases. A message always comes with the same code.

## Testing

Run the following commands from the `backend` directory.

### Check formatting

```bash
gofmt -l $(find . -type f -name '*.go')
```

The command should produce no output when all Go files are formatted.

### Run unit and integration tests

```bash
go test ./... -count=1
```

### Run race detection

```bash
go test -race ./...
```

### Run static analysis

```bash
go vet ./...
```

### Build the application

```bash
go build ./...
```

The test suite covers configuration validation, domain and use-case behavior, HTTP handlers and middleware, application lifecycle, and PostgreSQL repository behavior.

### Embedded time zone database

The dashboard validates the `tz` query parameter with Go's `time.LoadLocation`, and the frontend always sends `Asia/Ho_Chi_Minh`. Go reads time zones from the host (the `ZONEINFO` variable, the system zoneinfo directories on Unix, and the Go installation's `lib/time/zoneinfo.zip`). A host with none of them, such as a minimal container or a Windows server without Go installed, would reject every valid time zone with `400 invalid query parameters`.

`main.go` therefore imports `time/tzdata`, which embeds a copy of the IANA time zone database in the executable (about 400 KB). Go still uses the host's zoneinfo when it exists; the embedded copy is only the fallback, so dates, ranges, buckets and daylight saving time are computed exactly as before. `TestProductionExecutableEmbedsTZData` in `main_test.go` fails if `time/tzdata` is no longer linked into the executable.

Smoke test, from `backend/`: build the `main` package's test binary with `-trimpath`, so it has no Go installation path built in, then run it without any zoneinfo source. It links the same packages as the server and sends `Asia/Ho_Chi_Minh` and `UTC` through the real dashboard validation:

```bash
go test -c -trimpath -o /tmp/gostock-main.test .
env -u ZONEINFO GOROOT=/nonexistent /tmp/gostock-main.test \
  -test.run TestDashboardTimezonesLoadWithProductionImports -test.v
```

The test must report `PASS`. Without the embedded database it fails with `tz must be an IANA time zone name`. This hides every zoneinfo source on Windows. On Linux, Go also reads `/usr/share/zoneinfo`, so run the binary in a container image that has no zoneinfo instead.

The embedded database only serves Go. PostgreSQL converts timestamps with its own time zone database (`AT TIME ZONE`), which this does not change; the PostgreSQL server must also recognize the time zones the dashboard uses.

### Configure the integration test database

PostgreSQL integration tests use a dedicated test database configured by `TEST_DATABASE_URL`. They never read `DATABASE_URL`, so loading `.env` cannot point them at the development database.

- If `TEST_DATABASE_URL` is not set, integration tests are skipped. A skipped integration test is not a passed test.
- With `REQUIRE_TEST_DATABASE=true` (as in CI), a missing `TEST_DATABASE_URL` fails the integration tests instead of skipping them. A value that is not a boolean also fails. A database that cannot be reached always fails, whether or not the variable is set.
- Before any test writes data, it checks `current_database()`. The test fails immediately unless the database is named exactly `gostock_test`. Any other name fails, including `gostock_ui_test`, which is reserved for manual UI testing.

Integration tests insert and delete products and users and create refresh tokens. Use a database that you are allowed to modify, never a shared or production database.

Create the test database once (for example, `CREATE DATABASE gostock_test;`), then migrate it to the latest version from the `backend` directory:

```bash
export TEST_DATABASE_URL='postgres://postgres:YOUR_PASSWORD@localhost:5432/gostock_test?sslmode=disable'
migrate -path migrations -database "$TEST_DATABASE_URL" up
```

Run the full suite, including integration tests:

```bash
go test -count=1 ./...
```

To set the variable for a single run without exporting it:

```bash
TEST_DATABASE_URL='postgres://postgres:YOUR_PASSWORD@localhost:5432/gostock_test?sslmode=disable' go test -count=1 ./...
```

### Run the frontend dashboard tests

From `frontend/`, after `npm ci`:

```bash
npm run test:dashboard
```

It type-checks the tests and runs them once per time zone (UTC, America/Santiago, Pacific/Kiritimati). It needs no backend or database.

### Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request:

- **backend**:
  - Starts a new, empty PostgreSQL 17 service database (`gostock_test`) and checks it.
  - Applies every migration with golang-migrate from scratch, then checks that the database is at the newest migration file and is not dirty.
  - Runs `go vet ./...` and `go build ./...`.
  - Runs `go test -json -count=1 ./...` twice with `REQUIRE_TEST_DATABASE=true`. After each run, `.github/scripts/go-test-report.mjs` fails the job on any failed or skipped test or subtest, or if a required dashboard test did not run with all its subtests passing. The job also fails if a test leaves rows in any public table other than `schema_migrations`.
  - Checks that the production build embeds `time/tzdata`.
- **tzdata-smoke**: builds the static test binary of `package main` and runs the time zone smoke test in a `busybox` container, which has no zoneinfo.
- **frontend**: `npm ci`, then `npm run test:dashboard`.

The CI database and its credentials exist only inside the job. CI never connects to a developer or production database.

### Run the application against the UI test database

Manual UI testing uses a separate database, `gostock_ui_test`. It is never the development database and never `gostock_test`. Keep its connection string in its own variable, `GOSTOCK_UI_DATABASE_URL`. Start the API with `DATABASE_URL` set explicitly from that variable, and set `EXPECTED_DATABASE_NAME` so the server refuses to start on any other database. This applies even when the shell already holds a development `DATABASE_URL`:

```bash
DATABASE_URL="${GOSTOCK_UI_DATABASE_URL:?not set}" EXPECTED_DATABASE_NAME=gostock_ui_test go run .
```

On success the server logs `database preflight passed` with the database name. On any other database it exits with `database preflight: connected to "…", expected "gostock_ui_test"` before opening its HTTP port. Use the same two variables for `go run . admin create` and `go run . admin reset-password`.

## Current Scope and Future Improvements

The current project focuses on the Go backend API and its architecture.

Potential future improvements include:

- OpenAPI documentation.
- Structured application logging and observability.
- Automated CI pipelines.
- Expanded integration and concurrency testing.
- Additional API and operational documentation.
- Frontend integration.

## License

No license has been specified yet.