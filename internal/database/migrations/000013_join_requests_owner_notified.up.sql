-- Whether creating this request pushed the owner. A stale queue re-pushes
-- only when no pending request has pushed in the last 24h, so a burst into
-- an undrained queue notifies once instead of once per request.
ALTER TABLE public.join_requests
    ADD COLUMN IF NOT EXISTS owner_notified boolean NOT NULL DEFAULT false;
