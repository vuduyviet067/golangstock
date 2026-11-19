-- Removes every object created by 000007_add_products_minimum_stock.up.sql so
-- products again matches the canonical schema validated by migration 000006.
-- Rollback discards all stored minimum_stock values.
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_minimum_stock_check;
ALTER TABLE products DROP COLUMN IF EXISTS minimum_stock;
