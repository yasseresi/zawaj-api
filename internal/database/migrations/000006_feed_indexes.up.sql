-- Supporting indexes for keyset (cursor) pagination. The feed queries filter by
-- owner then ORDER BY created_at DESC, id DESC; these composite indexes let
-- Postgres satisfy both the filter and the ordering without a sort.
CREATE INDEX idx_activity_feed ON public.activity_logs USING btree (wedding_id, created_at DESC, id DESC);
CREATE INDEX idx_notif_feed ON public.notifications USING btree (user_id, created_at DESC, id DESC);
