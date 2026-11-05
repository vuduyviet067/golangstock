# GoStock — Inventory Management API

GoStock is a backend REST API for inventory management, built with Go and PostgreSQL.

The project provides product and inventory management, user authentication, role-based authorization, and database migration management. Its backend is organized around Clean Architecture principles to separate business logic from HTTP delivery and database persistence.

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go |
| HTTP server and routing | Go standard library (`net/http`) |
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
- Provide a CLI command to create the first administrator.

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
│   ├── cmd/
│   │   └── createadmin/
│   │       └── main.go
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
│   │   │       ├── response/
│   │   │       └── ... handlers and middleware
│   │   ├── platform/
│   │   │   ├── database/
│   │   │   └── jwt/
│   │   ├── repository/
│   │   │   └── postgres/
│   │   └── usecase/
│   ├── migrations/
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
| POST | `/api/v1/auth/register` | Register a new user | Public |
| POST | `/api/v1/auth/login` | Log in | Public |
| POST | `/api/v1/auth/refresh` | Refresh authentication tokens | Refresh cookie |
| POST | `/api/v1/auth/logout` | Log out and revoke refresh token | Refresh cookie |
| GET | `/api/v1/auth/me` | Get the authenticated user | Authenticated |

Access and refresh tokens are delivered through HttpOnly cookies. The access token is a JWT; refresh tokens are persisted to support rotation and revocation.

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

### User administration

All user-management endpoints require the Admin role.

| Method | Endpoint | Description |
|---|---|---|
| GET | `/api/v1/users` | List and search users |
| GET | `/api/v1/users/{id}` | Get user details |
| PATCH | `/api/v1/users/{id}` | Update user information and role |

Newly registered accounts receive the `user` role. Administrator accounts are created through the operator CLI.

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

The `sessions` table is legacy migration history: migration `000003` creates it and migration `000005` drops it. It does not exist after all migrations have run.

Migration `000001_baseline_products` is a baseline marker only; it does not create or modify the products table.

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

Newly registered accounts receive the `user` role.

Create the first administrator from the `backend` directory:

```bash
go run ./cmd/createadmin
```

The command requires `DATABASE_URL` and an initialized database.

It interactively prompts for:

- Email address.
- Password.
- Password confirmation.

The password is not echoed in the terminal. Passwords must be between 8 and 72 bytes.

The command fails if the email address is already registered.

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
| `APP_ENV` | No | Application environment; defaults to `development` |
| `ENV` | No | Fallback environment variable when `APP_ENV` is unset |
| `HTTP_PORT` | No | HTTP port; defaults to `8080` |
| `JWT_SECRET` | Conditional | JWT signing secret; at least 32 bytes |
| `JWT_ISSUER` | No | JWT issuer; defaults to `gostock` |
| `JWT_AUDIENCE` | No | JWT audience; defaults to `gostock-api` |
| `ACCESS_TOKEN_TTL_MINUTES` | No | Access token lifetime; defaults to 15 minutes |
| `REFRESH_TOKEN_TTL_DAYS` | No | Refresh token lifetime; defaults to 7 days |
| `REFRESH_TOKEN_GRACE_PERIOD` | No | Refresh-token rotation grace period; defaults to 15 seconds |
| `COOKIE_SECURE` | No | Whether authentication cookies require HTTPS |
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
- Production requires secure cookies.
- `COOKIE_SAMESITE=none` requires `COOKIE_SECURE=true`.

These defaults are intended for local development only. Configure explicit secrets, origins, and cookie settings before deployment.

## API Response Format

The API uses a standardized response envelope.

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
  "message": "Product not found",
  "data": null
}
```

Paginated responses include pagination metadata inside `data`.

HTTP status codes are used to distinguish successful requests, validation errors, authentication failures, authorization failures, missing resources, and internal errors.

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

PostgreSQL integration tests run only when `DATABASE_URL` is set. Otherwise, they are skipped; a skipped integration test is not a passed test.

Integration tests require a database migrated to the latest version. They may insert and delete products and users and create refresh tokens.

Always use a dedicated test database that you are allowed to modify. Never point integration tests at a shared or production database.

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