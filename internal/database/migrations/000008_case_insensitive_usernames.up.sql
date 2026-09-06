-- Usernames are normalized at the API boundary. Normalize existing rows before
-- enforcing the same invariant in the database so concurrent registrations
-- cannot create case variants.
UPDATE public.users
SET username = lower(btrim(username));

DROP INDEX IF EXISTS idx_users_username;
CREATE UNIQUE INDEX idx_users_username_ci
    ON public.users (lower(username));
