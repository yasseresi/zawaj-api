-- Server-side refresh-token records for rotation + revocation. id is the token's
-- jti. Style mirrors 000001 (app-managed UUIDs/timestamps, no DB-level FKs).
CREATE TABLE public.refresh_tokens (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone
);

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);

CREATE INDEX idx_refresh_tokens_user ON public.refresh_tokens USING btree (user_id);
