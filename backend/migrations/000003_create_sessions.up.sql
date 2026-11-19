CREATE TABLE sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT sessions_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT sessions_token_hash_unique
        UNIQUE (token_hash)
);

CREATE INDEX sessions_user_id_idx
    ON sessions (user_id);

CREATE INDEX sessions_expires_at_idx
    ON sessions (expires_at);