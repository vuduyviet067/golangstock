-- Per-product low-stock threshold. Existing rows get 0 (no threshold), so no
-- product is reported as low stock until a threshold is set.
ALTER TABLE products
    ADD COLUMN minimum_stock INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT products_minimum_stock_check CHECK (minimum_stock >= 0);
