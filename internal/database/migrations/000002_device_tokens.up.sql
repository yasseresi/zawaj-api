-- Push device registrations (FCM). One row per device token; unique per token so
-- a re-registering device re-points to its current owner via upsert. Style mirrors
-- 000001 (app-managed timestamps/UUIDs, no DB-level FKs).
CREATE TABLE public.device_tokens (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id uuid NOT NULL,
    token character varying(512) NOT NULL,
    platform character varying(10) NOT NULL,
    last_seen_at timestamp with time zone
);

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_device_tokens_token ON public.device_tokens USING btree (token);
CREATE INDEX idx_device_tokens_user ON public.device_tokens USING btree (user_id);
