# Zawaj Backend — Build Plan

Source: [SPEC.md](../SPEC.md) · [ARCHITECTURE.md](../docs/ARCHITECTURE.md) · [SYSTEM_DESIGN.md](../docs/SYSTEM_DESIGN.md)

**Strategy:** vertical slices. Each task delivers one complete path (handler → service →
repository → model → test), not a horizontal layer. Build order follows the dependency graph;
the authz core (roles) lands before guest writes because every write depends on it.

**Execution model — spine then swarm.** The shared spine (P0–P3 + the two seams below) is
built sequentially by one hand; it does not parallelize (shared router, models, middleware).
Once the spine compiles + is committed, the independent leaf modules (guests, notifications,
stats, member-management) fan out to parallel subagents in **git worktrees**, then merge.

Two seams make the swarm safe (no giant shared-file conflicts):
- **`router.Module` contract** — each module exposes `Register(rg *gin.RouterGroup)`; main
  appends it. Adding a module never edits `router.go`.
- **`Notifier` interface** — guest/member services emit events against an interface; the
  notifications module implements it. Producers and consumer build independently.

**Stack:** Go 1.22 + Gin + GORM + Postgres. Auth: username+password+JWT. See docs.

---

## Dependency graph

```
P0 scaffold ──► P1 auth ──► P2 wedding ──► P3 membership+roles ──► P4 guests ──► P5 stats
                  │                              │
                  └── JWT + RequireAuth ─────────┘  (RequireRole depends on membership)
P6 hardening (rate-limit, migrations, health, shutdown) ── cross-cuts, lands last before test/secure
```

Rules: nothing in P2+ works without P1's `RequireAuth`. Guest writes (P4) require P3's
`RequireRole`. Stats (P5) reads guests (P4). Hardening (P6) touches auth (P1) + infra (P0).

---

## Phases & checkpoints

Each phase ends at a **checkpoint** — a runnable, tested state where we stop for review before
continuing. Commit at each checkpoint (Conventional Commits, per-concern).

### P0 — Scaffold & infra
Goal: `make run` serves a health check against a real Postgres.
- Go module, folder layout per SPEC §3, Gin router, config from env, GORM connect.
- `docker-compose.yml` (postgres + api), `Dockerfile`, `Makefile`, `.env.example`.
- Middleware skeleton: CORS → RequestID → Logger → Recover. `GET /healthz`, `GET /readyz` (DB ping).
- Response envelope helper `{data,error}` + typed error codes.
- **Checkpoint C0:** `make up` boots; `curl /healthz` = 200; `/readyz` pings DB. ✅ commit.

### P1 — Auth vertical
Goal: a user can register, log in, refresh, recover — end to end.
- Models: `User`. AutoMigrate wired.
- `auth` pkg: bcrypt password hash, recovery-code gen+hash, JWT issue/verify (access+refresh).
- DTOs + validation (username format, password min 8).
- Endpoints: register, login, refresh, recover, `GET/PATCH /me`.
- `RequireAuth` middleware (parse access → userID in ctx).
- **Checkpoint C1:** register→login→call `/me`→refresh→recover all pass in integration test. ✅ commit.

### P2 — Wedding vertical
Goal: authed user creates/lists/reads weddings; owner regenerates share code.
- Models: `Wedding` (+ owner auto-membership on create — depends on P3 model, so create Membership model here too).
- Endpoints: `POST/GET /weddings`, `GET/PATCH/DELETE /weddings/:id`, `POST /weddings/:id/share`.
- Creating a wedding writes owner `Membership` in same txn.
- **Checkpoint C2:** create wedding → appears in my list → owner can patch/regenerate share code. ✅ commit.

### P3 — Membership & role enforcement (the authz core — ADR-004)
Goal: join by code; owner manages roles; `RequireRole` guards everything.
- Endpoints: `POST /weddings/join`, `GET /weddings/:id/members`, `PATCH/DELETE .../members/:userId`.
- `RequireRole(min)` middleware: load `Membership(:id,userID)` → 404 if none, 403 if role<min.
- Wire all `/weddings/:id/...` routes under one guarded Gin group.
- Join is idempotent; default role viewer; owner-leave blocked until transfer.
- **Checkpoint C3 (critical):** integration tests prove non-member→404, viewer-write→403, owner-only actions denied to editor. ✅ commit.

### P4 — Guests vertical
Goal: editors CRUD guests + change RSVP; anyone reads/filters/searches.
- Model: `Guest` (+ indexes: wedding_id, (wedding_id,status), (wedding_id,full_name)).
- Endpoints: list (filter status/side/category, `q` search, pagination), create, get, patch (incl status), delete.
- Service enforces: wedding_id from membership (never body); status transitions valid; write routes RequireRole(editor).
- **Checkpoint C4:** editor adds/edits/deletes guest; viewer blocked (403); filters + search + pagination return correct sets. ✅ commit.

### P5 — Stats
Goal: live counts per wedding.
- `GET /weddings/:id/stats` → single aggregate query (counts by status, total + confirmed seats).
- **Checkpoint C5:** stats match a seeded fixture exactly. ✅ commit.

### P6 — Hardening (pre-ship)
Goal: close the security/ops gaps flagged in ADRs.
- **Rate-limit + lockout** on login/recover (ADR-003 — password brute-force/stuffing is the real threat).
- Replace AutoMigrate with **golang-migrate** versioned migrations (ADR-002).
- Per-request context timeout; graceful shutdown on SIGTERM (drain + close pool).
- Error-envelope polish: map GORM errors → 409/404/500; never leak SQL.
- **Checkpoint C6:** lockout trips after N bad PINs (423); migrations apply clean on empty DB; SIGTERM drains. ✅ commit.

---

## Pre-build gate (BLOCKING)
**G0 — Import the design.** Read `zawaj_app/screens-*.jsx` + `styles.css`. Confirm guest fields
(side/category/party_size/notes/phone), statuses, and that no unplanned module (seating,
invite-sending) exists. If new modules surface → revise SPEC/ADR before P4.
- Unblock via: files in `zawaj_app/`, screenshots, or authorized claude.ai connector.
- **Do not start P4 (guests) until G0 clears** — that's where design details bite hardest.
  P0–P3 (scaffold/auth/wedding/roles) are design-independent and safe to build now.

---

## Verification per task (definition of done)
1. `make lint` clean · `make test -race` green.
2. New endpoint has ≥1 happy-path + ≥1 authz-denied integration test (per SPEC §7).
3. No handler touches GORM directly; no model bound from a request body.
4. Every wedding-scoped query filtered by `wedding_id` from verified membership.
5. Checkpoint criteria met → commit.

## Risk register
| Risk | Mitigation | Phase |
|---|---|---|
| Missed authz on a route | Single guarded router group; C3 tests assert deny paths | P3 |
| password brute-force/stuffing | Rate-limit + lockout | P6 |
| Design mismatch → guest rework | G0 gate before P4 | pre-P4 |
| AutoMigrate data loss in prod | Switch to versioned migrations | P6 |
| Concurrent guest edits overwrite | Accepted (LWW) v1; add version check if reported | later |
