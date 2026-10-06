-- Optional, unverified Algerian contact phone (national format, e.g. 0672859965).
-- Not an auth factor (ADR-003). The legacy email column is kept but no longer exposed.
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS phone character varying(10);
