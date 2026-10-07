-- The owner's last decision about a member, kept after the membership itself
-- is gone, so invite links can't override it:
--   max_role 'editor' | 'viewer' : links grant at most this role
--   max_role 'none'              : removed by the owner; links are refused
-- Written by owner role changes and removals; cleared when the owner
-- re-invites the user by username.
CREATE TABLE IF NOT EXISTS public.membership_ceilings (
    wedding_id uuid NOT NULL,
    user_id uuid NOT NULL,
    max_role character varying(10) NOT NULL,
    set_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (wedding_id, user_id)
);
