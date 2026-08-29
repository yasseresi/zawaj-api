-- Idempotency records for mutating requests. Unique per (user_id, key); style
-- mirrors 000001 (app-managed UUIDs/timestamps, no DB-level FKs).
CREATE TABLE public.idempotency_keys (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id uuid NOT NULL,
    key character varying(200) NOT NULL,
    request_hash character varying(64) NOT NULL,
    status_code integer NOT NULL,
    response bytea
);

ALTER TABLE ONLY public.idempotency_keys
    ADD CONSTRAINT idempotency_keys_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_idem_user_key ON public.idempotency_keys USING btree (user_id, key);
