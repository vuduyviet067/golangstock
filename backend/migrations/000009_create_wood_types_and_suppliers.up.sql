-- Catalogs of the timber workshop: wood types and suppliers (XG-05).
--
-- golang-migrate sends this file as a single simple-protocol query, so
-- PostgreSQL runs both statements in one implicit transaction. Do not add
-- BEGIN/COMMIT (see 000008).
--
-- Names are not unique: two wood types may share a name. Rows are deleted for
-- real; tables added later that reference these ones make PostgreSQL refuse
-- the delete of a row still in use (SQLSTATE 23503).

CREATE TABLE wood_types (
    id         BIGSERIAL   PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT wood_types_name_check
        CHECK (btrim(name) <> '' AND char_length(name) <= 100)
);

CREATE TABLE suppliers (
    id         BIGSERIAL   PRIMARY KEY,
    name       TEXT        NOT NULL,
    -- Optional, not unique. When present it is a normalized Vietnamese number
    -- (pkg/phone): 0, a digit 1-9, then 8 or 9 digits.
    phone      TEXT        NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT suppliers_name_check
        CHECK (btrim(name) <> '' AND char_length(name) <= 100),
    CONSTRAINT suppliers_phone_check
        CHECK (phone IS NULL OR phone ~ '^0[1-9][0-9]{8,9}$')
);
