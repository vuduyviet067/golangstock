-- Removes every object created by 000008_create_stock_movements.up.sql.
-- Dropping the table also drops its constraints (including both foreign keys),
-- its three indexes and its identity sequence. All recorded stock movement
-- history is discarded.
DROP TABLE IF EXISTS stock_movements;
