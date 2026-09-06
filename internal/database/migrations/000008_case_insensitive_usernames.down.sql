DROP INDEX IF EXISTS idx_users_username_ci;
CREATE UNIQUE INDEX idx_users_username ON public.users USING btree (username);
