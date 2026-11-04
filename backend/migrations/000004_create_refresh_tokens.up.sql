CREATE TABLE refresh_tokens (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    replaced_by_hash TEXT,

    CONSTRAINT refresh_tokens_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT refresh_tokens_token_hash_unique
        UNIQUE (token_hash)
);

CREATE INDEX refresh_tokens_user_id_idx
    ON refresh_tokens (user_id);

CREATE INDEX refresh_tokens_expires_at_idx
    ON refresh_tokens (expires_at);
