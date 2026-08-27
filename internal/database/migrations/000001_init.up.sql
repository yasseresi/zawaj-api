
COMMENT ON SCHEMA public IS '';

CREATE TABLE public.activity_logs (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    guest_id uuid,
    action character varying(30) NOT NULL,
    meta jsonb
);

CREATE TABLE public.guest_notes (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    guest_id uuid NOT NULL,
    author_id uuid NOT NULL,
    body text NOT NULL
);

CREATE TABLE public.guests (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    added_by uuid NOT NULL,
    full_name character varying(120) NOT NULL,
    contact character varying(120),
    relationship character varying(80),
    status character varying(12) DEFAULT 'pending'::character varying NOT NULL,
    companions bigint DEFAULT 0 NOT NULL,
    table_label character varying(40),
    meal character varying(40)
);

CREATE TABLE public.invite_links (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    created_by uuid NOT NULL,
    token character varying(24) NOT NULL,
    role character varying(10) NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    expires_at timestamp with time zone
);

CREATE TABLE public.memberships (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role character varying(10) NOT NULL,
    joined_at timestamp with time zone
);

CREATE TABLE public.notifications (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id uuid NOT NULL,
    wedding_id uuid,
    type character varying(40) NOT NULL,
    title character varying(160) NOT NULL,
    body text,
    data jsonb,
    read_at timestamp with time zone
);

CREATE TABLE public.users (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    username character varying(120) NOT NULL,
    display_name character varying(80),
    email character varying(120),
    password_hash text NOT NULL,
    recovery_hash text NOT NULL,
    dark_mode boolean DEFAULT false NOT NULL,
    notif_push boolean DEFAULT true NOT NULL,
    notif_email boolean DEFAULT true NOT NULL,
    notif_rsvp boolean DEFAULT true NOT NULL,
    is_premium boolean DEFAULT false NOT NULL,
    failed_attempts bigint DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone
);

CREATE TABLE public.weddings (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name character varying(120) NOT NULL,
    event_date date,
    description text,
    owner_id uuid NOT NULL
);

ALTER TABLE ONLY public.activity_logs
    ADD CONSTRAINT activity_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.guest_notes
    ADD CONSTRAINT guest_notes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.guests
    ADD CONSTRAINT guests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.invite_links
    ADD CONSTRAINT invite_links_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.weddings
    ADD CONSTRAINT weddings_pkey PRIMARY KEY (id);

CREATE INDEX idx_activity_guest ON public.activity_logs USING btree (guest_id);

CREATE INDEX idx_activity_wedding ON public.activity_logs USING btree (wedding_id);

CREATE INDEX idx_guest_notes_guest_id ON public.guest_notes USING btree (guest_id);

CREATE INDEX idx_guest_wedding ON public.guests USING btree (wedding_id);

CREATE INDEX idx_guest_wedding_status ON public.guests USING btree (wedding_id, status);

CREATE UNIQUE INDEX idx_invite_links_token ON public.invite_links USING btree (token);

CREATE INDEX idx_invite_links_wedding_id ON public.invite_links USING btree (wedding_id);

CREATE UNIQUE INDEX idx_membership_wedding_user ON public.memberships USING btree (wedding_id, user_id);

CREATE INDEX idx_memberships_user_id ON public.memberships USING btree (user_id);

CREATE INDEX idx_notif_user_read ON public.notifications USING btree (user_id, read_at);

CREATE UNIQUE INDEX idx_users_username ON public.users USING btree (username);

CREATE INDEX idx_weddings_owner_id ON public.weddings USING btree (owner_id);

