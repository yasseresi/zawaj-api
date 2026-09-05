# Zawaj API

Backend for the Zawaj wedding-planning app: a Go **modular monolith** (Gin + GORM + Postgres) exposing a versioned REST API under `/api/v1`.

- **Auth:** username + password (bcrypt) with a one-time recovery code. No email/SMS. Stateless JWT (short access token + long refresh token), attempt-based login lockout.
- **AuthZ:** default-deny, wedding-scoped RBAC (`Owner > Editor > Viewer`).
- **Response contract:** every response is `{ "data": ..., "error": null }`, or on error `data: null` with `error: { code, message, details }`.
- **Architecture:** layered — `handler → service → repository → GORM/Postgres`. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the ADRs and [`../CLAUDE.md`](../CLAUDE.md) for the working guide.

---

## Requirements

- **Go 1.26.6** (pinned in `go.mod` — earlier toolchains carry stdlib CVEs remediated here)
- **PostgreSQL 15+** (local install *or* Docker)
- `psql` client on `PATH` (for the DB bootstrap script)
- Optional: Docker + Docker Compose

---

## Quick start

The API auto-runs SQL migrations on boot, but it does **not** create its own Postgres role or database. Two ways to cover that gap.

### Option A — Local Postgres (recommended for dev)

One command bootstraps the role/db and starts the server:

```bash
cd backend
cp .env.example .env      # first time only
make dev
```

`make dev` runs `scripts/setup-db.sh` (creates role `zawaj` + database `zawaj`, idempotent) then `go run ./cmd/api`. Migrations apply automatically on boot.

This fixes the classic cold-start error:

```
FATAL: role "zawaj" does not exist (SQLSTATE 28000)
```

Subsequent runs can just use `make run` (DB already exists).

### Option B — Docker (Postgres + API, zero local setup)

```bash
cd backend
make up      # docker compose up -d --build  (bundled Postgres + API on :8080)
make down    # tear down
```

The compose stack creates the `zawaj` role/db inside its own Postgres container, so no script is needed. This is **dev only** — production uses `docker-compose.prod.yml` (API alone against an external managed DB).

### Verify

```bash
curl -s localhost:8080/healthz          # {"data":{"status":"ok"},"error":null}
```

Register a user and get tokens:

```bash
curl -s -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"amine","password":"correct horse battery staple"}'
```

Swagger UI (non-production): http://localhost:8080/swagger/index.html

---

## The DB bootstrap script

`scripts/setup-db.sh` parses `DATABASE_URL` from `.env` and, connecting as a Postgres **superuser**, ensures the login role, the database, and `public` schema ownership all exist. It is **idempotent** — safe to run repeatedly.

```bash
make db-setup                              # create role + db if missing
make db-reset                              # DROP + recreate db, then migrations re-run clean
./scripts/setup-db.sh --superuser postgres # connect as a specific superuser
./scripts/setup-db.sh --help               # flags + behavior
```

Use **`make db-reset`** when migrations fail with a `already exists` error after a partial or older init — it drops the database and lets the next boot apply migrations from scratch. **It destroys all local data in that database.**

Superuser detection order: `--superuser` flag → current OS user → `postgres`. On Homebrew macOS the superuser is usually your OS user with no password.

---

## Make targets

| Target | Does |
|--------|------|
| `make dev` | Bootstrap DB **and** run the API (one-shot local start) |
| `make run` | `go run ./cmd/api` (assumes DB exists) |
| `make db-setup` | Create local role + database (idempotent) |
| `make db-reset` | Drop + recreate the local database |
| `make build` | Build `bin/api` |
| `make test` | `go test ./... -race -cover` |
| `make lint` | `golangci-lint run` |
| `make tidy` | `go mod tidy` |
| `make up` / `make down` | Docker compose up/down (bundled Postgres) |
| `make docs` | Regenerate OpenAPI/Swagger from handler annotations (needs `swag` v1.16.4) |

---

## Configuration

All config is parsed once at startup from env (+ optional `.env`). Copy `.env.example` → `.env` and adjust. Key vars:

| Var | Meaning |
|-----|---------|
| `APP_ENV` | `development` / `production` (prod fails fast on unsafe config) |
| `APP_PORT` | HTTP port (default `8080`) |
| `DATABASE_URL` | `postgres://user:pass@host:port/db?sslmode=...` — also drives the setup script |
| `JWT_ACCESS_SECRET` / `JWT_REFRESH_SECRET` | Signing secrets (prod: ≥32 bytes, distinct, non-placeholder) |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | Token lifetimes |
| `LOGIN_MAX_ATTEMPTS` / `LOGIN_LOCKOUT` | Login lockout tuning |
| `CORS_ALLOWED_ORIGINS` | Exact origins (prod rejects `*`) |
| `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST` | Per-IP token bucket (`0` disables) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Firebase service-account JSON → enables FCM push (unset = no-op) |
| `ENABLE_SWAGGER` | Opt Swagger UI on in production |

Any secret can instead come from `<NAME>_FILE` (Docker/K8s secrets convention), which takes precedence. **Production** boot fails fast on: wildcard CORS, weak/placeholder/identical JWT secrets, and `sslmode=disable`.

---

## Testing

```bash
make test                                        # all unit tests, race + cover
go test ./internal/service -run TestName -race   # a single unit test
```

Integration tests in `test/` are **black-box**: they build the fully wired router against a **real Postgres** and skip unless `TEST_DATABASE_URL` is set.

```bash
TEST_DATABASE_URL='postgres://zawaj:zawaj@localhost:5432/zawaj?sslmode=disable' \
  go test ./test -run TestAuth -race
```

---

## Database & migrations

- Schema changes go through **versioned SQL migrations** in `internal/database/migrations/` (`NNNNNN_name.up.sql` / `.down.sql`, golang-migrate).
- `main.go` runs pending migrations on boot (`database.RunMigrations`). GORM AutoMigrate is intentionally retired — add a new numbered migration pair instead.
- After changing handler godoc annotations, run `make docs` or the committed `docs/swagger/*` drifts.

---

## Project layout

```
cmd/api/              entrypoint + composition root (wires repos → services → handlers)
internal/
  router/             global middleware + module mounting
  handler/            HTTP layer (each implements Register(rg *gin.RouterGroup))
  service/            business logic (returns *apperr.Error)
  repository/         GORM/Postgres access
  models/ dto/        domain + transport types
  middleware/         auth, RBAC, idempotency, rate limit, security headers…
  apperr/ auth/ config/ database/ events/ observability/ push/
pkg/{response,pagination,token}
scripts/setup-db.sh   local Postgres role/db bootstrap
test/                 black-box integration tests
docs/                 ARCHITECTURE.md (ADRs) + generated Swagger
```

To trace how anything is wired, start at [`cmd/api/main.go`](cmd/api/main.go).

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `role "zawaj" does not exist` | `make db-setup` (or `make dev`) |
| migration fails, `... already exists` | `make db-reset` |
| `bind: address already in use` on `:8080` | kill the stray API: `lsof -ti:8080 \| xargs kill` |
| `psql: could not connect` in the script | start Postgres (`brew services start postgresql`) or use `make up` |
| script picks the wrong superuser | `./scripts/setup-db.sh --superuser postgres` |
