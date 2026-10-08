# Zawaj Backend — Architecture Decision Records

Backend for a collaborative wedding guest-list app. See [SPEC.md](../SPEC.md) for scope.
Records below are the load-bearing decisions. Status = **Accepted** unless noted.

**Constraints in force**
- Single solo/small dev, ship v1 fast.
- Community does **not** use email/phone → auth must be username-based, near-zero friction.
- Flutter mobile client later; backend is a pure JSON API.
- Low initial scale (hundreds → low thousands of users, one small DB). Correctness of
  collaboration/authz matters more than throughput.

---

## System context

```
Flutter app ──HTTPS/JSON──> Gin API ──> GORM ──> PostgreSQL
   (later)                    │
                              └── JWT (access+refresh), bcrypt secrets
```

Component layout (modular monolith, one deployable):

```
handler/ ──> service/ ──> repository/ ──> models/ (GORM) ──> Postgres
   ▲            ▲
   │            └─ business rules, role checks, status transitions
   └─ gin binding + DTO validation; middleware: RequireAuth, RequireRole, CORS, logging
```

---

## ADR-001: Modular monolith, layered (handler → service → repository)

**Status:** Accepted · **Date:** 2026-08-26

### Context
One small team, one bounded domain (weddings + guests + membership). Need fast iteration,
easy local run, and a client (Flutter) that only needs a stable JSON contract.

### Decision
Single Gin deployable, strict layering: `handler → service → repository → models`. No
microservices, no message bus, no CQRS. Modules organized by aggregate (auth, wedding,
guest, member), not by technical layer only.

### Options
| Option | Complexity | Cost | Scalability | Familiarity |
|---|---|---|---|---|
| A. Modular monolith (chosen) | Low | Low | Enough for v1+ | High |
| B. Microservices | High | High | Overkill | Low |
| C. Flat handlers (no service layer) | Low now | High later | Poor | High |

**Pros (A):** trivial deploy, one DB txn boundary, easy tests, clear seams to split later.
**Cons (A):** discipline required to keep handlers thin; shared process = shared failure domain.
Rejected **C** because authz/status rules would smear across handlers and become untestable.

### Consequences
- Easier: local dev, transactions, refactors, testing services in isolation.
- Harder: nothing at this scale; if a module later needs independent scaling, the aggregate seams allow extraction.
- Revisit when: sustained >tens of rps or a second team forms.

---

## ADR-002: GORM for data access (over sqlc / raw pgx)

**Status:** Accepted (user-directed) · **Date:** 2026-08-26

### Context
CRUD-heavy domain, few complex queries, solo dev optimizing for speed. User chose GORM.

### Decision
GORM with `AutoMigrate` for v1. Repository layer wraps GORM so services never import it
directly. Move to **versioned migrations (golang-migrate)** before production.

### Options
| Option | Complexity | Control | Speed to build | Risk |
|---|---|---|---|---|
| A. GORM (chosen) | Low | Medium | Fast | Hidden N+1 / implicit SQL |
| B. sqlc | Medium | High | Medium | More boilerplate |
| C. raw pgx | High | Highest | Slow | Manual mapping |

**Pros:** minimal boilerplate, associations/hooks, fast CRUD.
**Cons:** implicit SQL can hide N+1 and full-table scans; AutoMigrate is not safe for
destructive prod changes.

### Consequences
- Easier: writing CRUD + associations quickly.
- Harder: query-plan visibility → enable GORM SQL logging in dev; index `wedding_id`,
  `username`, `share_code`.
- **Action:** AutoMigrate for dev only; introduce golang-migrate before first real users.
- **Update (2026-10-08):** done — production and the integration tests both build the
  schema from the versioned SQL migrations (`database.RunMigrations`; the test harness
  resets `zawaj_test` and migrates once per run). AutoMigrate is gone entirely, so tests
  can't drift from the shipped schema.

---

## ADR-003: Username + password + one-time recovery code auth

**Status:** Accepted · **Date:** 2026-08-26

### Context
Target community largely lacks email and often won't share phone numbers. Email/OTP would
kill adoption. Still need a per-user secret — username-only lets anyone impersonate anyone.

### Decision
Register with **username (unique) + password (min 8)**. Server bcrypt-hashes the password,
generates a random **recovery code** returned **once** in plaintext (bcrypt hash stored).
Recovery code is the *only* account-reset path. Rate-limit + lockout on login/recover.

**Amendment (2026-10-06):** users may add an optional, **unverified** Algerian phone number
(national format, e.g. `0672859965`) as a profile contact field. It is never an auth or
recovery factor, so this decision is unchanged. The legacy `email` column is no longer exposed.

### Options
| Option | Friction | Security | Reset path |
|---|---|---|---|
| A. Username + password (min 8) + recovery code (chosen) | Medium | High | recovery code |
| B. Username + PIN + recovery code | Very low | Medium | recovery code |
| C. Email/phone OTP | High | High | email/SMS |

Chose **A** over the original PIN (B) after the user prioritized security: a min-8 password
resists online guessing far better than a 4–6 digit PIN, at the cost of more typing.

**Pros:** no email/SMS dependency, one-screen signup, strong secret, works for the community.
**Cons:** more typing than a PIN; lost recovery code = lost account.

### Consequences
- Rate-limit + lock out after N failed logins (defense-in-depth against stuffing; the main
  brute-force risk is much lower than a PIN would carry).
- UI must force the user to save the recovery code at signup (no other reset channel).
- Revisit if account-takeover appears: add optional phone as recovery, or device binding.

---

## ADR-004: Authorization = membership + role middleware, default-deny, wedding-scoped

**Status:** Accepted · **Date:** 2026-08-26

### Context
The whole product is a *shared* list. The core risk is one user reading/editing a wedding
they don't belong to, or a viewer mutating data. This is the highest-value correctness area.

### Decision
Every wedding-scoped route passes through `RequireAuth` then `RequireRole(min)`. The guard
loads the caller's `Membership` for the path `:id`; missing membership → 404 (don't leak
existence), insufficient role → 403. **`wedding_id` and `role` come only from the verified
membership**, never from the request body. All repository queries are scoped by that
`wedding_id`. Roles: viewer < editor < owner (see SPEC role matrix).

### Options
| Option | Safety | Complexity |
|---|---|---|
| A. Central role middleware + scoped queries (chosen) | High | Low |
| B. Per-handler ad-hoc checks | Low (easy to miss one) | Low |
| C. Full RBAC/policy engine (Casbin) | High | High (overkill) |

### Consequences
- Easier: uniform enforcement, one place to test/audit.
- Harder: must remember to attach the middleware to every wedding route — mitigated by
  grouping all wedding routes under one Gin router group that carries the guard.
- **Action:** integration tests assert 404 for non-member and 403 for viewer-writes on every route.
- **Addendum (2026-10-07/08) — joining by link needs the owner's approval.** Invite
  links are long-lived bearer tokens shared in group chats, and accounts are free, so no
  amount of link revocation can keep out someone who holds a live link (they re-register),
  and revoking a shared link to stop one person breaks it for everyone.
  - **Join requests** (migration 000012): accepting a link never creates a membership. It
    creates a pending `join_requests` row (one per wedding+user, partial unique index) and
    notifies the owner; the owner approves (optionally at a lower role) or declines.
    Approval locks the row, creates the membership (`via_link_id`, migration 000011, kept
    for history) and records the ceiling in one transaction. A declined user can't ask
    again for 24h (409 `join_request_declined`). Links are never revoked on a member's
    behalf. Username invites (owner picked the exact account) still join directly.
  - **Ceilings** (migration 000010) record the owner's last decision per (wedding, user),
    surviving the membership: role changes and approvals write the role, owner removals
    write `none`, and an accepted owner username invite newer than the decision writes
    its outcome. A join request asks for `min(link role, ceiling)`; `none` is refused up
    front (403 `removed_from_wedding`). Self-leave writes nothing. An owner removal also
    withdraws the user's pending username invites.
  - Tests: `test/join_request_test.go`, `test/ceiling_test.go`.

---

## ADR-005: Stateless JWT — short access + long refresh

**Status:** Accepted · **Date:** 2026-08-26

### Context
Mobile client, no server session store desired for v1. Need revocation-ish behavior without
infra overhead.

### Decision
Access JWT ~15 min, refresh JWT ~30 days. `/auth/refresh` rotates both. Secrets in env.
No server-side token store in v1 (accept that logout is client-side + short access TTL).

### Options
| Option | Complexity | Revocation | Infra |
|---|---|---|---|
| A. Stateless access+refresh (chosen) | Low | Weak (TTL-bounded) | None |
| B. Server session store (Redis) | Medium | Strong | Adds Redis |
| C. Long-lived single token | Lowest | None | None |

### Consequences
- Easier: no session infra, scales trivially.
- Harder: can't instantly revoke a stolen access token (bounded by 15-min TTL).
- Revisit if abuse: add a refresh-token denylist table (cheap, DB-backed) before Redis.

---

## Cross-cutting decisions (lightweight)
- **IDs:** UUID v4 primary keys (safe to expose in URLs, no enumeration).
- **Response envelope:** `{data, error}` uniform; typed error codes for the Flutter client.
- **Validation:** DTOs with `validator` tags at the handler edge; models never bound directly.
- **Config:** 12-factor env vars; `.env.example` committed, `.env` gitignored.
- **Observability (v1-min):** structured slog + request-id middleware. Metrics/tracing later.

---

## Open architectural questions (blocked)
1. **Design not yet imported** — `screens-*.jsx` may reveal features (e.g. seating, invite
   sending) that change module boundaries. Confirm before build freeze.
2. Refresh-token revocation: accept TTL-only for v1, or add denylist table now? (Leaning: TTL-only.)
3. Multi-wedding per user assumed — confirm no "one active wedding" constraint from the design.
```
