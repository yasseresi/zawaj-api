-- Targeted collaborator invites (screen 10 "Pending Invites"): an owner invites a
-- specific registered user to a wedding with a role; the invitee accepts or
-- declines. Distinct from invite_links (shareable tokens). Style mirrors 000001
-- (app-managed UUIDs/timestamps, no DB-level FKs).
CREATE TABLE public.wedding_invites (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    inviter_id uuid NOT NULL,
    invitee_id uuid NOT NULL,
    role character varying(10) NOT NULL,
    status character varying(10) NOT NULL DEFAULT 'pending'
);

ALTER TABLE ONLY public.wedding_invites
    ADD CONSTRAINT wedding_invites_pkey PRIMARY KEY (id);

-- One outstanding invite per (wedding, invitee); resolved invites don't block re-invites.
CREATE UNIQUE INDEX idx_wedding_invites_pending
    ON public.wedding_invites USING btree (wedding_id, invitee_id)
    WHERE (status = 'pending');

-- The invitee's "my pending invites" query.
CREATE INDEX idx_wedding_invites_invitee
    ON public.wedding_invites USING btree (invitee_id, status);
