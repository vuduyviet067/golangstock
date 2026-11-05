# GoStock - Inventory Management API

GoStock is a REST API for managing products and inventory operations, built with Go and PostgreSQL.

The project focuses on product CRUD operations, search, pagination, sorting, and safe inventory updates under concurrent requests.

## Tech Stack

- **Language:** Go
- **HTTP Server:** Go standard library (`net/http`)
- **Database:** PostgreSQL
- **Database Access:** `database/sql`
- **PostgreSQL Driver:** pgx v5
- **Testing:** Go `testing` and `net/http/httptest`

## Features

- Create, retrieve, update, and delete products.
- Search products by keyword.
- Paginate and sort product lists.
- Manage inventory through stock-in and stock-out operations.
- Validate stock quantities and handle missing products.
- Prevent negative inventory through conditional SQL updates.
- Return appropriate HTTP status codes for inventory errors.
- Test successful operations, invalid quantities, missing products, and concurrent stock-out requests.

## Project Structure
```
gostock/
├── backend/
│   ├── main.go
│   ├── database.go
│   ├── health.go
│   ├── go.mod
│   ├── go.sum
│   └── internal/
│       ├── domain/
│       │   ├── product.go
│       │   └── product_repository.go
│       ├── usecase/
│       │   ├── product.go
│       │   └── product_test.go
│       ├── repository/
│       │   └── postgres/
│       │       └── product_repository.go
│       └── handle/
│           └── http/
│               ├── product_handler.go
│               ├── product_handler_test.go
│               └── product_handler_integration_test.go
├── frontend/
├── deploy/
├── docs/
├── .env.example
├── .gitignore
└── README.md
```
## Prerequisites

- Go installed
- PostgreSQL 17 installed and running
- An empty PostgreSQL database named `gostock`
- The [golang-migrate](https://github.com/golang-migrate/migrate) CLI built with the PostgreSQL driver:

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Setup

### 1. Create the database

Create an empty PostgreSQL database named `gostock`. Do not create application tables by hand; the migrations in `backend/migrations` create them.

### 2. Configure the database connection

Copy `.env.example` to `.env` and replace `YOUR_PASSWORD` with your local PostgreSQL password.

The application reads `DATABASE_URL` from the environment. It does not automatically load `.env`, so set the variable in your terminal.

For Git Bash:

```bash
export DATABASE_URL='postgres://postgres:YOUR_PASSWORD@localhost:5432/gostock?sslmode=disable'
```

Never commit real credentials to Git.

### 3. Install dependencies

Run these commands from the project root:

```bash
cd backend
go mod download
```

### 4. Run the database migrations

From the `backend` directory, with `DATABASE_URL` set:

```bash
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" version
```

A fresh database must run every migration, starting from `000001`. After `up`, `version` reports the highest migration number and the database contains:

| Table | Created by |
|---|---|
| `users` | `000002_create_users` |
| `refresh_tokens` | `000004_create_refresh_tokens` |
| `products` | `000006_create_or_validate_products` |

`sessions` is legacy history: `000003` creates it and `000005` drops it, so it does not exist after `up`. `000001_baseline_products` is a marker only and creates nothing.

If `products` was created by hand before migrations managed it, `000006` validates that table against the expected schema instead of creating it. On any difference it changes nothing, reports every mismatch, and leaves the migration version marked dirty at 6. Fix the reported differences, then run `migrate -path migrations -database "$DATABASE_URL" force 5` and `up` again. Do not use `force` to skip the validation.

**Rollback behavior:** Rolling back one step from version 6 (`migrate down 1`) intentionally does not drop `products`. Migration `000006` may have adopted a pre-existing table containing application data, and its down migration cannot distinguish an adopted table from one it created. Therefore, after rolling back, the database remains at version 5 while `products` still exists. Only drop the table manually when working with a disposable database and after verifying that its data is no longer needed.

Optional sample data, after the migrations have run:

```sql
INSERT INTO products (sku, name, price, quantity)
VALUES
    ('KB-001', 'Mechanical Keyboard', 850000, 20),
    ('MS-001', 'Wireless Mouse', 350000, 15);
```

### 5. Create the first administrator

New accounts get the `user` role. Create the first `admin` account with the operator CLI, from the `backend` directory, with `DATABASE_URL` set and migrations applied:

```bash
go run ./cmd/createadmin
```

The CLI prompts for the email, the password and its confirmation. In a terminal the password is not echoed. The password must be 8 to 72 bytes, and the command fails if the email is already registered.

### 6. Start the API

From the `backend` directory:

```bash
go run .
```

The server listens on port `8080` by default. Set the `HTTP_PORT` environment variable to use a different port.
## Testing

Run the following commands from the `backend` directory.

Check Go formatting:

```bash
gofmt -l $(find . -type f -name '*.go')
```

Run all tests:

```bash
go test -v -count=1 ./...
```

Run static analysis:

```bash
go vet ./...
```

The test suite includes HTTP handler tests, use case validation tests, and PostgreSQL integration tests for concurrent stock-out requests.

PostgreSQL integration tests run only when `DATABASE_URL` is set; otherwise they are skipped. A skipped integration test is not a passed one. They require a database migrated to the latest version, and they write to it: they insert and delete products and users and create refresh tokens. Point `DATABASE_URL` at a dedicated database you are allowed to modify, never a shared or production database.

## Future Improvements

- Add structured logging and HTTP server timeouts.
- Document the API with OpenAPI.
- Expand test coverage for product listing, filtering, pagination, sorting, and error handling.
- Configure CI to run automated tests.
- Improve health checks by verifying database connectivity when appropriate.