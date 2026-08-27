# Zawaj Backend — TODO

## PROGRESS (2026-08-27) — v1 feature-complete
- ✅ P0 scaffold · ✅ P1 auth (username+password, JWT, recovery) · ✅ P2 weddings · ✅ P3 roles+invite-links/QR
- ✅ P4 guests (+notes, status, activity history) · ✅ settings (change pw, prefs)
- ✅ SWARM P5 (4 parallel worktree agents, merged): stats · activity feed · notifications (fan-out) · CSV export
- ✅ Full schema migrated (8 tables) · ✅ design imported → docs/DESIGN_SPEC.md
- ✅ All integration tests green (-race): auth, roles matrix, guests, settings, stats, activity, notifications, export
- Local dev DB: EDB PostgreSQL 17 on :5432 (postgres/postgres), db `zawaj`
- ▶ REMAINING: P6 hardening (login rate-limit/lockout; AutoMigrate→golang-migrate) · then test→secure→review→ship
- LATER (deferred v1): OAuth, premium/subscription, PDF export, FCM push, guest reminders (none)


Ordered task list. See [plan.md](plan.md) for rationale, dependency graph, checkpoints.
`[ ]` todo · `[~]` in progress · `[x]` done. Commit at each checkpoint (C0–C6).

## G0 — Pre-build gate (BLOCKING for P4)
- [x] Import design: read `zawaj_app/screens-*.jsx`, `icons.jsx`, `design-canvas.jsx`, `styles.css`
- [x] Confirm guest fields / statuses / no unplanned modules → reconcile SPEC if needed
- [x] Answer SPEC open Qs: join role (viewer?), party_size semantics, multi-wedding, recovery-code UX

## P0 — Scaffold & infra → C0
- [x] `go mod init`, folder layout (cmd/api, internal/*, pkg)
- [x] config pkg (env → typed Config), `.env.example`
- [x] GORM Postgres connect + pool settings
- [x] Gin router + middleware: CORS → RequestID → Logger → Recover
- [x] Response envelope `{data,error}` + error-code constants
- [x] `GET /healthz`, `GET /readyz` (DB ping)
- [x] Makefile, Dockerfile, docker-compose.yml
- [x] **C0:** `make up` + `curl /healthz`=200, `/readyz` pings DB → commit

## P1 — Auth → C1
- [x] `User` model + AutoMigrate
- [x] auth pkg: bcrypt password, recovery-code gen+hash, JWT issue/verify (access+refresh)
- [x] DTOs + validation (username, password min 8)
- [x] `POST /auth/register` (returns recovery_code once)
- [x] `POST /auth/login`
- [x] `POST /auth/refresh` (rotate)
- [x] `POST /auth/recover`
- [x] `RequireAuth` middleware
- [x] `GET /me`, `PATCH /me`
- [x] Integration tests: full auth flow
- [x] **C1** → commit

## P2 — Wedding → C2
- [x] `Wedding` + `Membership` models + indexes
- [x] `POST /weddings` (creator → owner membership, same txn)
- [x] `GET /weddings` (my weddings via membership)
- [x] `GET /weddings/:id` (+ my role)
- [x] `PATCH /weddings/:id` (owner)
- [x] `DELETE /weddings/:id` (owner, cascade)
- [~] ~~`POST /weddings/:id/share`~~ OBSOLETE — replaced by role-scoped invite-links (`POST /weddings/:id/invite-links`, `/invite/:token/accept`) per design
- [x] Tests
- [x] **C2** → commit

## P3 — Membership & roles (authz core) → C3
- [x] `RequireRole(min)` middleware (load membership → 404/403)
- [x] Wire all `/weddings/:id/*` under one guarded router group
- [~] ~~`POST /weddings/join`~~ OBSOLETE — replaced by `POST /invite/:token/accept` (idempotent, no-downgrade)
- [x] `GET /weddings/:id/members`
- [x] `PATCH /weddings/:id/members/:userId` (owner)
- [x] `DELETE /weddings/:id/members/:userId` (owner removes / self-leave) — NOTE: owner cannot leave; **transfer-ownership endpoint NOT built** → see below
- [x] **Critical tests:** non-member→404, viewer-write→403, editor-does-owner-action→403
- [ ] Transfer-ownership endpoint (owner → another member) — NOT built
- [x] **C3** → commit

## P4 — Guests → C4  (needs G0 cleared)
- [x] `Guest` model + indexes (wedding_id; wedding_id,status; wedding_id,full_name)
- [x] `GET /weddings/:id/guests` (filter status/side/category + `q` + pagination)
- [x] `POST /weddings/:id/guests` (editor+)
- [x] `GET /weddings/:id/guests/:guestId`
- [x] `PATCH /weddings/:id/guests/:guestId` (editor+, incl status)
- [x] `DELETE /weddings/:id/guests/:guestId` (editor+)
- [x] Service: wedding_id from membership; valid status transitions
- [x] Tests (CRUD + viewer-blocked + filter/search correctness)
- [x] **C4** → commit

## SPINE / SWARM SPLIT
P0–P3 above = **spine** (sequential, one hand). Below = **swarm** (parallel worktree agents,
merge after). Spine must compile + commit before swarm launches. Seams: `router.Module`, `Notifier`.

## P4 — Guests → C4  [SWARM] (needs G0 cleared)
(as listed above — worktree agent A)

## P5 — Stats → C5  [SWARM]  (worktree agent B; reads Guest model — depends on P4 model contract)
- [x] `GET /weddings/:id/stats` (single aggregate query)
- [x] Test vs seeded fixture
- [x] **C5** → commit

## P5b — Notifications → C5b  [SWARM]  (worktree agent C)
- [x] `Notification` model + index(user_id, read_at)
- [x] `Notifier` interface impl: fan event → recipients (other members)
- [x] `GET /notifications` (feed, ?unread, pagination)
- [x] `GET /notifications/unread_count`
- [x] `PATCH /notifications/:id/read`, `POST /notifications/read_all`
- [x] Wire emit points in guest/member services (via interface — no direct coupling)
- [x] Tests: event → correct recipients; read/unread transitions
- [x] **C5b** → commit
- [ ] (LATER) FCM push + device_tokens; WebSocket realtime — deferred, not built

## P5c — Member management polish  [SWARM]  (worktree agent D — if split from P3)

## P6 — Hardening → C6
- [~] Login lockout done (423 after N fails). NOT done: recover throttle, 429 rate-limit
- [ ] golang-migrate versioned migrations (retire AutoMigrate) — **NOT done (ship blocker)**
- [ ] Per-request context timeout — NOT done
- [x] Graceful shutdown on SIGTERM (drain + close pool)
- [~] Service errors → typed apperr (no SQL leak) done; explicit raw-GORM unique→409 mapping NOT added (dup handled by pre-check)
- [ ] **C6** → commit (blocked on migrations)

## After build
- [~] Tests exist per module (all -race green); no dedicated `agent-skills:test` expansion pass
- [x] Phase: secure — `security-review` + `VibeSec-Skill` (auth, authz, brute-force)
- [x] Phase: review — `agent-skills:review`
- [ ] Phase: ship — deploy checklist, backups, migrations on target — NOT done
