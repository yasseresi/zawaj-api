-- Security/compliance audit trail. Style mirrors 000001 (app-managed
-- UUIDs/timestamps, no DB-level FKs). Distinct from activity_logs.
CREATE TABLE public.audit_logs (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    actor_id uuid,
    action character varying(60) NOT NULL,
    target_type character varying(40),
    target_id uuid,
    ip character varying(64),
    meta jsonb
);

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);

CREATE INDEX idx_audit_logs_actor ON public.audit_logs USING btree (actor_id);
CREATE INDEX idx_audit_logs_action ON public.audit_logs USING btree (action);
CREATE INDEX idx_audit_logs_created ON public.audit_logs USING btree (created_at);
