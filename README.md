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
- PostgreSQL installed and running
- A PostgreSQL database named `gostock`

## Setup

### 1. Create the database and products table

Create a PostgreSQL database named `gostock`, connect to it, and execute:

```sql
CREATE TABLE products (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sku VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(200) NOT NULL,
    price NUMERIC(12, 2) NOT NULL CHECK (price >= 0),
    quantity INTEGER NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Optional sample data:

```sql
INSERT INTO products (sku, name, price, quantity)
VALUES
    ('KB-001', 'Mechanical Keyboard', 850000, 20),
    ('MS-001', 'Wireless Mouse', 350000, 15);
```

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

### 4. Start the API

From the `backend` directory:

```bash
go run .
```

The server address and port are configured in `main.go`.
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

Integration tests require a reachable PostgreSQL database configured through `DATABASE_URL`.

## Future Improvements

- Add structured logging and HTTP server timeouts.
- Document the API with OpenAPI.
- Expand test coverage for product listing, filtering, pagination, sorting, and error handling.
- Configure CI to run automated tests.
- Improve health checks by verifying database connectivity when appropriate.