-- The invite link a member joined with (NULL for owners and username
-- invites), kept for history and audit.
ALTER TABLE public.memberships ADD COLUMN IF NOT EXISTS via_link_id uuid;
