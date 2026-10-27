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

```text
gostock/
├── database.go
├── main.go
├── product.go
├── product_handlers.go
├── product_handlers_test.go
├── go.mod
├── go.sum
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

Set the `DATABASE_URL` environment variable. Replace `YOUR_PASSWORD` with your PostgreSQL password.

For Git Bash:

```bash
export DATABASE_URL='postgres://postgres:YOUR_PASSWORD@localhost:5432/gostock?sslmode=disable'
```

Do not commit database passwords or other secrets to Git.

### 3. Download dependencies

```bash
go mod download
```

### 4. Run the server

```bash
go run .
```

The server address and port are configured in `main.go`.

## API Endpoints

Base path: `/api/v1`

| Method | Endpoint | Description |
|---|---|---|
| GET | `/health` | Check server health |
| GET | `/api/v1/products` | List products |
| GET | `/api/v1/products/{id}` | Retrieve a product |
| POST | `/api/v1/products` | Create a product |
| PUT | `/api/v1/products/{id}` | Update a product |
| DELETE | `/api/v1/products/{id}` | Delete a product |
| POST | `/api/v1/products/{id}/stock-in` | Increase stock |
| POST | `/api/v1/products/{id}/stock-out` | Decrease stock |

### Search, pagination, and sorting

Example requests:

```http
GET /api/v1/products?q=mouse
GET /api/v1/products?page=1&limit=2
GET /api/v1/products?sortBy=price&order=asc
```

### Stock-in example

```http
POST /api/v1/products/1/stock-in
Content-Type: application/json

{
  "quantity": 5
}
```

### Stock-out example

```http
POST /api/v1/products/1/stock-out
Content-Type: application/json

{
  "quantity": 3
}
```

## Concurrency and Inventory Safety

Stock-out uses a conditional SQL update:

```sql
UPDATE products
SET quantity = quantity - $1,
    updated_at = NOW()
WHERE id = $2
  AND quantity >= $1
RETURNING id, sku, name, price, quantity;
```

The quantity check and update happen in a single SQL statement. PostgreSQL coordinates concurrent updates to the same row, helping prevent overselling and negative inventory.

- `409 Conflict`: insufficient stock.
- `404 Not Found`: product does not exist.

## Testing

Run all tests:

```bash
go test -v -count=1 ./...
```

The current test suite covers:

- Successful stock-in and stock-out.
- Insufficient stock.
- Zero and negative quantities.
- Non-existent products.
- Concurrent stock-out requests.

The integration tests require a reachable PostgreSQL database configured through `DATABASE_URL`.

## Future Improvements

- Add structured logging and request timeouts.
- Document the API with OpenAPI.
- Expand test coverage for product CRUD, search, and pagination.
- Configure CI to run automated tests.
