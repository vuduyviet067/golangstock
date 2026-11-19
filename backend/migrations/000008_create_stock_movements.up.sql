-- Stock movement history: one immutable row per stock change.
--
-- golang-migrate sends this file as a single simple-protocol query, so
-- PostgreSQL runs every statement in one implicit transaction: either all of
-- them apply or none do. Do not add BEGIN/COMMIT: on failure an explicit block
-- would leave the migration connection in an aborted transaction.
--
-- The lock blocks concurrent writes to products until the backfill commits,
-- so the opening balances below match products.quantity exactly.
LOCK TABLE products IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE stock_movements (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY,
    -- NULL once the product is deleted; product_sku keeps the history readable.
    product_id      BIGINT,
    product_sku     VARCHAR(50) NOT NULL,
    movement_type   VARCHAR(20) NOT NULL,
    -- Signed: positive for initial/in, negative for out.
    quantity_change INTEGER     NOT NULL,
    quantity_before INTEGER     NOT NULL,
    quantity_after  INTEGER     NOT NULL,
    -- NULL only for opening balances created by this migration.
    user_id         BIGINT,
    -- clock_timestamp(), not NOW(): the time the row is written, after the
    -- product row lock is acquired, not when the transaction started.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT stock_movements_pkey PRIMARY KEY (id),
    CONSTRAINT stock_movements_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE SET NULL,
    CONSTRAINT stock_movements_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT stock_movements_type_check
        CHECK (movement_type IN ('initial', 'in', 'out')),
    CONSTRAINT stock_movements_sign_check
        CHECK ((movement_type IN ('initial', 'in') AND quantity_change > 0)
            OR (movement_type = 'out' AND quantity_change < 0)),
    CONSTRAINT stock_movements_quantity_before_check
        CHECK (quantity_before >= 0),
    CONSTRAINT stock_movements_quantity_after_check
        CHECK (quantity_after >= 0),
    CONSTRAINT stock_movements_balance_check
        CHECK (quantity_after = quantity_before + quantity_change),
    CONSTRAINT stock_movements_actor_check
        CHECK (user_id IS NOT NULL OR movement_type = 'initial')
);

-- Per-product history, newest first; also serves ON DELETE SET NULL.
CREATE INDEX stock_movements_product_id_idx ON stock_movements (product_id, id DESC);
-- Time-range filters for reports.
CREATE INDEX stock_movements_created_at_idx ON stock_movements (created_at);
-- Serves the ON DELETE RESTRICT check on users.
CREATE INDEX stock_movements_user_id_idx ON stock_movements (user_id);

-- Opening balance for every product that currently has stock. Earlier stock
-- changes were never recorded, so no other history is invented.
INSERT INTO stock_movements (
    product_id, product_sku, movement_type,
    quantity_change, quantity_before, quantity_after, user_id
)
SELECT id, sku, 'initial', quantity, 0, quantity, NULL
FROM products
WHERE quantity > 0
ORDER BY id;
