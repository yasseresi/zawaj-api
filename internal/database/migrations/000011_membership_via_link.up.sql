-- The invite link a member joined with (NULL for owners and username
-- invites). When the owner removes the member, or demotes them below the role
-- that link grants, the link is revoked so a second account can't reuse it.
ALTER TABLE public.memberships ADD COLUMN IF NOT EXISTS via_link_id uuid;
