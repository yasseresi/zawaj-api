# Zawaj Backend — Production Setup Runbook

> **This file is intentionally NOT committed.** It is your operator checklist.
> It contains no secrets — but keep it that way (link to your secret store, never
> paste real values here). Delete or move it out of the repo if you prefer.

The application code is production-hardened (CORS allowlist, per-IP rate limiting,
refresh-token rotation/revocation, secret validation, non-root container, versioned
migrations). The steps below are the **infrastructure** work only you can do.

---

## 1. Generate secrets

Two distinct, high-entropy JWT secrets (≥ 32 bytes each). The app rejects short,
equal, or placeholder secrets in production.

```bash
# Run twice; store each output in your secret manager.
openssl rand -base64 48
```

Produce: `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET` (must differ).

## 2. Choose a secret store (pick one)

The app reads each secret from the env var **or** from a file via `<NAME>_FILE`
(precedence to `_FILE`). Use whichever your platform offers:

- **Docker secrets / Compose**: mount files, set `JWT_ACCESS_SECRET_FILE=/run/secrets/jwt_access`, etc.
- **Kubernetes**: `Secret` → mount as files (`_FILE`) or inject as env.
- **AWS**: Secrets Manager / SSM Parameter Store → inject at deploy.
- **GCP**: Secret Manager → mount or env.
- **Vault**: Agent/CSI injects files → point `_FILE` at them.

Never bake secrets into the image or commit them. `.env` is gitignored; keep it
root-owned (`chmod 600`) if you use a file on a host.

## 3. Provision a managed Postgres (DB is NOT bundled in prod)

Use RDS / Cloud SQL / Supabase / a dedicated PG host. Then:

- Create DB + least-privilege app role (only DML + DDL it needs; not superuser).
- Require TLS: `DATABASE_URL=...?sslmode=require` (app rejects `sslmode=disable` in prod).
- Note the pool: defaults `DB_MAX_OPEN_CONNS=20`. Keep
  `instances × DB_MAX_OPEN_CONNS` below the server's `max_connections`
  (use PgBouncer if you scale out).

## 4. Run migrations against the target

Migrations run automatically on boot (golang-migrate, `internal/database/migrations`).
**Before first prod boot, dry-run against a restored copy** of the DB, not prod:

```bash
# example with the migrate CLI against a staging clone
migrate -path internal/database/migrations -database "$STAGING_DATABASE_URL" up
```

Rollback one step if needed: `migrate ... down 1` (each migration ships a `.down.sql`).

## 5. Backups + PITR (non-negotiable)

- Enable automated daily backups + **point-in-time recovery** on the managed DB
  (RDS/Cloud SQL/Supabase all offer this — turn it on).
- Set retention (e.g. 7–30 days) per your RPO.
- **Test a restore** into a scratch instance at least once, and after any schema change.
- Document RPO/RTO and where restores run.

## 6. Deploy the API

```bash
export DATABASE_URL='postgres://app:...@your-db:5432/zawaj?sslmode=require'
export JWT_ACCESS_SECRET='...'; export JWT_REFRESH_SECRET='...'
export CORS_ALLOWED_ORIGINS='https://app.zawaj.example'   # exact origins, no '*'
docker compose -f docker-compose.prod.yml up -d --build
```

`APP_ENV=production` is set by the prod compose (enables HSTS, quiet logs,
strict config validation, Swagger off).

## 7. TLS + reverse proxy

The API speaks plain HTTP on :8080. Put a TLS terminator in front
(Caddy/Nginx/Traefik or a cloud LB):

- Terminate HTTPS, forward to the container.
- If the proxy sets `X-Forwarded-For`, configure the app's trusted proxies
  (currently `SetTrustedProxies(nil)` in `internal/router/router.go`) so
  `ClientIP()` — the rate-limit key — reflects the real client, not the proxy.

## 8. Health checks + rollback

- Liveness: `GET /healthz` · Readiness: `GET /readyz` (checks DB).
- Wire both into the LB/orchestrator; the image also has a Docker `HEALTHCHECK`.
- **Rollback**: redeploy the previous image tag. If a migration must be reverted,
  run its `.down.sql` (`migrate ... down 1`) — but prefer forward-fix.

## 9. Optional: FCM push

Only if you want real push (otherwise the app no-ops cleanly):

- Firebase console → service account → JSON key.
- Mount it as a secret and set `GOOGLE_APPLICATION_CREDENTIALS=/run/secrets/firebase-sa.json`.
- The JSON's `project_id` selects the Firebase project.

## 10. Observability (P1 — see roadmap)

Not yet wired: Prometheus `/metrics`, OpenTelemetry tracing, alerting/SLOs.
Add before you rely on this in production for anything beyond low traffic.

---

## Pre-launch checklist

- [ ] `JWT_ACCESS_SECRET` / `JWT_REFRESH_SECRET` generated, distinct, in secret store
- [ ] Managed Postgres provisioned, least-privilege role, `sslmode=require`
- [ ] Pool sizing vs `max_connections` verified
- [ ] Migrations dry-run on a restored copy; forward + rollback tested
- [ ] Automated backups + PITR enabled; **restore tested**
- [ ] `CORS_ALLOWED_ORIGINS` = exact prod origins (no `*`)
- [ ] TLS terminator in front; trusted proxies set if behind a proxy
- [ ] `/healthz` + `/readyz` wired to LB/orchestrator
- [ ] Rollback procedure rehearsed (previous image tag + `down` migration)
- [ ] CI green including the new security + image scan jobs
- [ ] (If push) Firebase service account mounted
- [ ] (P1) metrics/tracing/alerting before real traffic
