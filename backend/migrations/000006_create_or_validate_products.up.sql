
DO $migration$
DECLARE
    products_oid oid;
    rel          record;
    item         record;
    problems     text[] := ARRAY[]::text[];
BEGIN
    -- NULL only when no relation of any kind named products exists in public.
    products_oid := to_regclass('public.products');

    IF products_oid IS NULL THEN
        CREATE TABLE public.products (
            id         BIGINT GENERATED ALWAYS AS IDENTITY,
            sku        VARCHAR(50)    NOT NULL,
            name       VARCHAR(200)   NOT NULL,
            price      NUMERIC(12, 2) NOT NULL,
            quantity   INTEGER        NOT NULL DEFAULT 0,
            created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),

            CONSTRAINT products_pkey           PRIMARY KEY (id),
            CONSTRAINT products_sku_key        UNIQUE (sku),
            CONSTRAINT products_price_check    CHECK (price >= 0),
            CONSTRAINT products_quantity_check CHECK (quantity >= 0)
        );
        RETURN;
    END IF;

    -- 1. Relation kind. A view, partitioned table, sequence, index, etc. named
    -- products cannot be adopted; stop before column checks produce noise.
    SELECT c.relkind, c.relpersistence, c.relispartition,
           c.relrowsecurity, c.relforcerowsecurity
      INTO rel
      FROM pg_catalog.pg_class AS c
     WHERE c.oid = products_oid;

    IF rel.relkind <> 'r' THEN
        RAISE EXCEPTION USING
            ERRCODE = 'wrong_object_type',
            MESSAGE = format(
                'migration 000006: public.products exists but is not an ordinary table (relkind %s); refusing to adopt it',
                rel.relkind),
            HINT = 'Nothing was changed. Resolve the conflict by hand, then run "migrate force 5" and "migrate up" again.';
    END IF;

    -- 2. Relation properties that change durability or which rows queries see.
    IF rel.relpersistence <> 'p' THEN
        problems := problems || format('table persistence is %s, expected permanent (p)', rel.relpersistence);
    END IF;
    IF rel.relispartition THEN
        problems := problems || 'table is a partition of another table'::text;
    END IF;
    IF rel.relrowsecurity OR rel.relforcerowsecurity THEN
        problems := problems || 'row level security is enabled'::text;
    END IF;

    FOR item IN
        SELECT inh.inhparent::regclass::text AS other
          FROM pg_catalog.pg_inherits AS inh
         WHERE inh.inhrelid = products_oid
    LOOP
        problems := problems || format('table inherits from %s', item.other);
    END LOOP;

    FOR item IN
        SELECT inh.inhrelid::regclass::text AS other
          FROM pg_catalog.pg_inherits AS inh
         WHERE inh.inhparent = products_oid
    LOOP
        problems := problems || format('table has inheritance child %s (its rows would appear in product queries)', item.other);
    END LOOP;

    -- 3. Columns: exactly these seven, compared by name (column order is not
    -- significant; the application always lists columns explicitly).
    FOR item IN
        WITH expected (col, type_name, not_null, identity_kind, default_expr, collation_name) AS (
            VALUES
                ('id',         'bigint',                   true, 'a', NULL::text, NULL::text),
                ('sku',        'character varying(50)',    true, '',  NULL,       'pg_catalog.default'),
                ('name',       'character varying(200)',   true, '',  NULL,       'pg_catalog.default'),
                ('price',      'numeric(12,2)',            true, '',  NULL,       NULL),
                ('quantity',   'integer',                  true, '',  '0',        NULL),
                ('created_at', 'timestamp with time zone', true, '',  'now()',    NULL),
                ('updated_at', 'timestamp with time zone', true, '',  'now()',    NULL)
        ),
        actual AS (
            SELECT a.attname::text                                  AS col,
                   pg_catalog.format_type(a.atttypid, a.atttypmod)  AS type_name,
                   a.attnotnull                                     AS not_null,
                   a.attidentity::text                              AS identity_kind,
                   a.attgenerated::text                             AS generated_kind,
                   pg_catalog.pg_get_expr(d.adbin, d.adrelid)       AS default_expr,
                   (SELECT n.nspname::text || '.' || co.collname::text
                      FROM pg_catalog.pg_collation AS co
                      JOIN pg_catalog.pg_namespace AS n ON n.oid = co.collnamespace
                     WHERE co.oid = a.attcollation)                 AS collation_name
              FROM pg_catalog.pg_attribute AS a
              LEFT JOIN pg_catalog.pg_attrdef AS d
                ON d.adrelid = a.attrelid AND d.adnum = a.attnum
             WHERE a.attrelid = products_oid
               AND a.attnum > 0
               AND NOT a.attisdropped
        )
        SELECT e.col            AS e_col,
               e.type_name      AS e_type,
               e.not_null       AS e_not_null,
               e.identity_kind  AS e_identity,
               e.default_expr   AS e_default,
               e.collation_name AS e_collation,
               a.col            AS a_col,
               a.type_name      AS a_type,
               a.not_null       AS a_not_null,
               a.identity_kind  AS a_identity,
               a.generated_kind AS a_generated,
               a.default_expr   AS a_default,
               a.collation_name AS a_collation
          FROM expected AS e
          FULL JOIN actual AS a ON a.col = e.col
         ORDER BY coalesce(e.col, a.col)
    LOOP
        IF item.a_col IS NULL THEN
            problems := problems || format('missing column %I', item.e_col);
            CONTINUE;
        END IF;
        IF item.e_col IS NULL THEN
            problems := problems || format('unexpected column %I (%s)', item.a_col, item.a_type);
            CONTINUE;
        END IF;
        IF item.a_type IS DISTINCT FROM item.e_type THEN
            problems := problems || format('column %I: type is %s, expected %s',
                                           item.a_col, item.a_type, item.e_type);
        END IF;
        IF item.a_not_null IS DISTINCT FROM item.e_not_null THEN
            problems := problems || format('column %I: nullable, expected NOT NULL', item.a_col);
        END IF;
        IF item.a_identity IS DISTINCT FROM item.e_identity THEN
            problems := problems || format('column %I: identity is %s, expected %s',
                item.a_col,
                CASE item.a_identity WHEN 'a' THEN 'GENERATED ALWAYS' WHEN 'd' THEN 'GENERATED BY DEFAULT' ELSE 'none' END,
                CASE item.e_identity WHEN 'a' THEN 'GENERATED ALWAYS' ELSE 'none' END);
        END IF;
        IF item.a_generated <> '' THEN
            problems := problems || format('column %I: is a generated column, expected a plain column', item.a_col);
        END IF;
        IF item.a_default IS DISTINCT FROM item.e_default THEN
            problems := problems || format('column %I: default is %s, expected %s',
                item.a_col, coalesce(item.a_default, 'none'), coalesce(item.e_default, 'none'));
        END IF;
        IF item.a_collation IS DISTINCT FROM item.e_collation THEN
            problems := problems || format('column %I: collation is %s, expected %s',
                item.a_col, coalesce(item.a_collation, 'none'), coalesce(item.e_collation, 'none'));
        END IF;
    END LOOP;

    -- 4. Constraints: exactly these four, by exact name. Each is checked
    -- semantically (constraint type + referenced columns) and by its normalized
    -- pg_get_constraintdef() text, and must be validated and not deferrable.
    -- Any other constraint (FK, exclusion, extra CHECK/UNIQUE) is a mismatch.
    --
    -- connoinherit is compared per constraint, not required to be false:
    -- PostgreSQL records index-backed constraints (PRIMARY KEY, UNIQUE) on an
    -- ordinary table with connoinherit = true because traditional inheritance
    -- children never inherit them, while a plain CHECK is inherited
    -- (connoinherit = false) unless declared NO INHERIT. Values verified
    -- against the PostgreSQL 17 development database.
    FOR item IN
        WITH expected (conname, contype, column_names, condef, noinherit) AS (
            VALUES
                ('products_pkey',           'p', ARRAY['id']::text[], 'PRIMARY KEY (id)',                true),
                ('products_sku_key',        'u', ARRAY['sku'],        'UNIQUE (sku)',                    true),
                ('products_price_check',    'c', ARRAY['price'],      'CHECK ((price >= (0)::numeric))', false),
                ('products_quantity_check', 'c', ARRAY['quantity'],   'CHECK ((quantity >= 0))',         false)
        ),
        actual AS (
            SELECT con.conname::text AS conname,
                   con.contype::text AS contype,
                   ARRAY(
                       SELECT att.attname::text
                         FROM unnest(con.conkey) WITH ORDINALITY AS k (attnum, ord)
                         JOIN pg_catalog.pg_attribute AS att
                           ON att.attrelid = con.conrelid AND att.attnum = k.attnum
                        ORDER BY k.ord
                   ) AS column_names,
                   pg_catalog.pg_get_constraintdef(con.oid) AS condef,
                   con.convalidated,
                   con.condeferrable,
                   con.connoinherit
              FROM pg_catalog.pg_constraint AS con
             WHERE con.conrelid = products_oid
               AND con.contype <> 'n'  -- PostgreSQL 18+ NOT NULL rows; see header
        )
        SELECT e.conname      AS e_name,
               e.contype      AS e_type,
               e.column_names AS e_columns,
               e.condef       AS e_def,
               e.noinherit    AS e_noinherit,
               a.conname      AS a_name,
               a.contype      AS a_type,
               a.column_names AS a_columns,
               a.condef       AS a_def,
               a.convalidated AS a_validated,
               a.condeferrable AS a_deferrable,
               a.connoinherit AS a_noinherit
          FROM expected AS e
          FULL JOIN actual AS a ON a.conname = e.conname
         ORDER BY coalesce(e.conname, a.conname)
    LOOP
        IF item.a_name IS NULL THEN
            problems := problems || format('missing constraint %I (%s)', item.e_name, item.e_def);
            CONTINUE;
        END IF;
        IF item.e_name IS NULL THEN
            problems := problems || format('unexpected constraint %I (%s)', item.a_name, item.a_def);
            CONTINUE;
        END IF;
        IF item.a_type IS DISTINCT FROM item.e_type THEN
            problems := problems || format('constraint %I: type is %s, expected %s',
                                           item.a_name, item.a_type, item.e_type);
        END IF;
        IF item.a_columns IS DISTINCT FROM item.e_columns THEN
            problems := problems || format('constraint %I: columns are %s, expected %s',
                                           item.a_name, item.a_columns, item.e_columns);
        END IF;
        IF item.a_def IS DISTINCT FROM item.e_def THEN
            problems := problems || format('constraint %I: definition is %s, expected %s',
                                           item.a_name, item.a_def, item.e_def);
        END IF;
        IF NOT item.a_validated THEN
            problems := problems || format('constraint %I: is NOT VALID (existing rows were never checked)', item.a_name);
        END IF;
        IF item.a_deferrable THEN
            problems := problems || format('constraint %I: is DEFERRABLE, expected immediate', item.a_name);
        END IF;
        IF item.a_noinherit IS DISTINCT FROM item.e_noinherit THEN
            problems := problems || format('constraint %I: connoinherit is %s, expected %s',
                                           item.a_name, item.a_noinherit, item.e_noinherit);
        END IF;
    END LOOP;

    -- 5. Unique indexes not backing a constraint enforce uniqueness under a name
    -- the application does not map (a duplicate would surface as HTTP 500).
    -- Ordinary non-unique indexes are allowed and not reported.
    FOR item IN
        SELECT ic.relname::text AS index_name,
               pg_catalog.pg_get_indexdef(i.indexrelid) AS index_def
          FROM pg_catalog.pg_index AS i
          JOIN pg_catalog.pg_class AS ic ON ic.oid = i.indexrelid
         WHERE i.indrelid = products_oid
           AND i.indisunique
           AND NOT EXISTS (
               SELECT 1
                 FROM pg_catalog.pg_constraint AS con
                WHERE con.conrelid = products_oid
                  AND con.conindid = i.indexrelid)
         ORDER BY 1
    LOOP
        problems := problems || format('unexpected unique index %I (%s)', item.index_name, item.index_def);
    END LOOP;

    -- 6. Objects that can alter application behaviour without changing columns.
    FOR item IN
        SELECT t.tgname::text AS obj_name, pg_catalog.pg_get_triggerdef(t.oid) AS obj_def
          FROM pg_catalog.pg_trigger AS t
         WHERE t.tgrelid = products_oid
           AND NOT t.tgisinternal
         ORDER BY 1
    LOOP
        problems := problems || format('unexpected trigger %I (%s)', item.obj_name, item.obj_def);
    END LOOP;

    FOR item IN
        SELECT r.rulename::text AS obj_name
          FROM pg_catalog.pg_rewrite AS r
         WHERE r.ev_class = products_oid
         ORDER BY 1
    LOOP
        problems := problems || format('unexpected rule %I', item.obj_name);
    END LOOP;

    FOR item IN
        SELECT p.polname::text AS obj_name
          FROM pg_catalog.pg_policy AS p
         WHERE p.polrelid = products_oid
         ORDER BY 1
    LOOP
        problems := problems || format('unexpected row level security policy %I', item.obj_name);
    END LOOP;

    -- Foreign keys in other tables that reference products change DELETE
    -- behaviour (blocked deletes or cascades). Self-references are already
    -- reported as unexpected constraints in step 4.
    FOR item IN
        SELECT con.conname::text AS obj_name,
               con.conrelid::regclass::text AS referencing_table,
               pg_catalog.pg_get_constraintdef(con.oid) AS obj_def
          FROM pg_catalog.pg_constraint AS con
         WHERE con.contype = 'f'
           AND con.confrelid = products_oid
           AND con.conrelid <> products_oid
         ORDER BY 2, 1
    LOOP
        problems := problems || format('unexpected foreign key %I on %s referencing products (%s)',
                                       item.obj_name, item.referencing_table, item.obj_def);
    END LOOP;

    IF cardinality(problems) > 0 THEN
        RAISE EXCEPTION USING
            ERRCODE = 'invalid_table_definition',
            MESSAGE = format(
                'migration 000006: public.products exists but does not match the canonical schema (%s problem(s)): %s',
                cardinality(problems), array_to_string(problems, '; ')),
            HINT = 'Nothing was changed. Resolve each difference by hand, then run "migrate force 5" and "migrate up" again.';
    END IF;

    -- Existing table matches the canonical schema: adopt it unchanged.
END
$migration$;
