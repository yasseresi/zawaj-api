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
- [ ] Import design: read `zawaj_app/screens-*.jsx`, `icons.jsx`, `design-canvas.jsx`, `styles.css`
- [ ] Confirm guest fields / statuses / no unplanned modules → reconcile SPEC if needed
- [ ] Answer SPEC open Qs: join role (viewer?), party_size semantics, multi-wedding, recovery-code UX

## P0 — Scaffold & infra → C0
- [ ] `go mod init`, folder layout (cmd/api, internal/*, pkg)
- [ ] config pkg (env → typed Config), `.env.example`
- [ ] GORM Postgres connect + pool settings
- [ ] Gin router + middleware: CORS → RequestID → Logger → Recover
- [ ] Response envelope `{data,error}` + error-code constants
- [ ] `GET /healthz`, `GET /readyz` (DB ping)
- [ ] Makefile, Dockerfile, docker-compose.yml
- [ ] **C0:** `make up` + `curl /healthz`=200, `/readyz` pings DB → commit

## P1 — Auth → C1
- [ ] `User` model + AutoMigrate
- [ ] auth pkg: bcrypt password, recovery-code gen+hash, JWT issue/verify (access+refresh)
- [ ] DTOs + validation (username, password min 8)
- [ ] `POST /auth/register` (returns recovery_code once)
- [ ] `POST /auth/login`
- [ ] `POST /auth/refresh` (rotate)
- [ ] `POST /auth/recover`
- [ ] `RequireAuth` middleware
- [ ] `GET /me`, `PATCH /me`
- [ ] Integration tests: full auth flow
- [ ] **C1** → commit

## P2 — Wedding → C2
- [ ] `Wedding` + `Membership` models + indexes
- [ ] `POST /weddings` (creator → owner membership, same txn)
- [ ] `GET /weddings` (my weddings via membership)
- [ ] `GET /weddings/:id` (+ my role)
- [ ] `PATCH /weddings/:id` (owner)
- [ ] `DELETE /weddings/:id` (owner, cascade)
- [ ] `POST /weddings/:id/share` (regenerate code)
- [ ] Tests
- [ ] **C2** → commit

## P3 — Membership & roles (authz core) → C3
- [ ] `RequireRole(min)` middleware (load membership → 404/403)
- [ ] Wire all `/weddings/:id/*` under one guarded router group
- [ ] `POST /weddings/join` (idempotent, default viewer)
- [ ] `GET /weddings/:id/members`
- [ ] `PATCH /weddings/:id/members/:userId` (owner)
- [ ] `DELETE /weddings/:id/members/:userId` (owner or self-leave; owner must transfer first)
- [ ] **Critical tests:** non-member→404, viewer-write→403, editor-does-owner-action→403
- [ ] **C3** → commit

## P4 — Guests → C4  (needs G0 cleared)
- [ ] `Guest` model + indexes (wedding_id; wedding_id,status; wedding_id,full_name)
- [ ] `GET /weddings/:id/guests` (filter status/side/category + `q` + pagination)
- [ ] `POST /weddings/:id/guests` (editor+)
- [ ] `GET /weddings/:id/guests/:guestId`
- [ ] `PATCH /weddings/:id/guests/:guestId` (editor+, incl status)
- [ ] `DELETE /weddings/:id/guests/:guestId` (editor+)
- [ ] Service: wedding_id from membership; valid status transitions
- [ ] Tests (CRUD + viewer-blocked + filter/search correctness)
- [ ] **C4** → commit

## SPINE / SWARM SPLIT
P0–P3 above = **spine** (sequential, one hand). Below = **swarm** (parallel worktree agents,
merge after). Spine must compile + commit before swarm launches. Seams: `router.Module`, `Notifier`.

## P4 — Guests → C4  [SWARM] (needs G0 cleared)
(as listed above — worktree agent A)

## P5 — Stats → C5  [SWARM]  (worktree agent B; reads Guest model — depends on P4 model contract)
- [ ] `GET /weddings/:id/stats` (single aggregate query)
- [ ] Test vs seeded fixture
- [ ] **C5** → commit

## P5b — Notifications → C5b  [SWARM]  (worktree agent C)
- [ ] `Notification` model + index(user_id, read_at)
- [ ] `Notifier` interface impl: fan event → recipients (other members)
- [ ] `GET /notifications` (feed, ?unread, pagination)
- [ ] `GET /notifications/unread_count`
- [ ] `PATCH /notifications/:id/read`, `POST /notifications/read_all`
- [ ] Wire emit points in guest/member services (via interface — no direct coupling)
- [ ] Tests: event → correct recipients; read/unread transitions
- [ ] **C5b** → commit
- [ ] (later) FCM push + device_tokens; WebSocket realtime

## P5c — Member management polish  [SWARM]  (worktree agent D — if split from P3)

## P6 — Hardening → C6
- [ ] Rate-limit + lockout on login/recover (423 on lock, 429 on throttle)
- [ ] golang-migrate versioned migrations (retire AutoMigrate)
- [ ] Per-request context timeout
- [ ] Graceful shutdown on SIGTERM (drain + close pool)
- [ ] GORM error mapping → 409/404/500, no SQL leak
- [ ] **C6** → commit

## After build
- [ ] Phase: test — expand coverage (`agent-skills:test`)
- [ ] Phase: secure — `security-review` + `VibeSec-Skill` (auth, authz, brute-force)
- [ ] Phase: review — `agent-skills:review`
- [ ] Phase: ship — deploy checklist, backups, migrations on target
