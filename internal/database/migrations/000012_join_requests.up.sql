-- Joining through an invite link needs the owner's approval: accepting a link
-- creates a join request; the owner approves (creating the membership) or
-- declines it. At most one pending request per (wedding, user).
CREATE TABLE IF NOT EXISTS public.join_requests (
    id uuid NOT NULL PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    wedding_id uuid NOT NULL,
    user_id uuid NOT NULL,
    link_id uuid NOT NULL,
    role character varying(10) NOT NULL,
    status character varying(10) NOT NULL DEFAULT 'pending',
    decided_at timestamp with time zone,
    decided_by uuid
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_join_requests_pending
    ON public.join_requests USING btree (wedding_id, user_id)
    WHERE (status = 'pending');

-- Owner's pending list (wedding, status, oldest first) and the requester's
-- decline-cooldown lookup.
CREATE INDEX IF NOT EXISTS idx_join_requests_wedding_status
    ON public.join_requests USING btree (wedding_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_join_requests_user
    ON public.join_requests USING btree (user_id);
