# Zawaj Backend — System Design

Companion to [SPEC.md](../SPEC.md) and [ARCHITECTURE.md](ARCHITECTURE.md). This doc covers
data flows, sequence diagrams, error handling, load, and reliability — the "how it runs"
layer, not the "what/why" (that's SPEC/ADR).

---

## 1. Requirements

### Functional
- Register/login with username + PIN; recover via one-time code.
- Create weddings; owner invites family via share code; roles owner/editor/viewer.
- CRUD guests (invitees) with RSVP status; filter/search; live stats.

### Non-functional
| Dimension | Target (v1) |
|---|---|
| Scale | ~1–5k users, ~hundreds of weddings, ≤ ~10k guests/wedding worst case |
| Read:write | Read-heavy (list/stats refreshed often; edits bursty around planning) |
| Latency | p95 < 200 ms for list/stats at v1 volumes |
| Availability | Single region, best-effort; brief downtime tolerable |
| Consistency | Strong within one Postgres; last-write-wins on guest edits (v1) |
| Cost | One small VM + one managed/containerized Postgres |

### Constraints
- Solo dev, ship fast. No email/SMS. Flutter client later. GORM chosen.

---

## 2. High-level design

```
                 ┌─────────────────────────────────────────┐
   Flutter  ──►  │  Gin API (single process)               │  ──►  PostgreSQL
   (HTTPS)       │                                          │       (single instance,
                 │  middleware: CORS→ReqID→Logger→Recover   │        daily backups)
                 │             →RequireAuth→RequireRole      │
                 │  handler → service → repository (GORM)    │
                 └─────────────────────────────────────────┘
```

- **Stateless API** → horizontally scalable later behind a load balancer (JWT, no sticky sessions).
- **One SQL database** is the single source of truth and the single transaction boundary.
- No cache, no queue, no external services in v1 (see §6 for when to add).

### Request lifecycle
```
HTTP → CORS → RequestID → Logger → PanicRecover
     → RequireAuth (parse access JWT → userID in ctx)
     → RequireRole(min) [wedding routes only: load Membership(:id,userID), check role]
     → handler: bind+validate DTO
     → service: rules (status transition, uniqueness, role edge-cases)
     → repository: GORM query scoped by wedding_id
     → response envelope {data|error}
```

---

## 3. Key data flows (sequence)

### 3.1 Register
```
Client → POST /auth/register {username, display_name, pin}
Service: validate username format + uniqueness
         bcrypt(pin) → pin_hash
         gen recovery_code (random) → recovery_hash = bcrypt(code)
         INSERT user
         issue access+refresh JWT
← 201 {user, access, refresh, recovery_code}   # recovery_code shown ONCE
```

### 3.2 Join a wedding via share code
```
Client → POST /weddings/join {share_code}   (auth required)
Service: find wedding by share_code (index) → 404 if none
         if membership exists → return it (idempotent)
         else INSERT membership(role=viewer)
← 200 {wedding, role: viewer}
Owner later: PATCH /weddings/:id/members/:userId {role: editor}
```

### 3.3 Add guest (role-guarded write)
```
Client → POST /weddings/:id/guests {full_name, party_size, side, ...}
RequireRole(editor): load Membership(:id, userID)
         missing → 404 (don't leak existence)
         role < editor → 403
Service: default status=to_invite; wedding_id from membership (NOT body)
         INSERT guest(added_by=userID)
         (optional) INSERT activity_log
← 201 {guest}
```

### 3.4 Stats
```
GET /weddings/:id/stats  (any member)
Repository: SELECT
  count(*) as total,
  count(*) FILTER (WHERE status=...) per status,
  sum(party_size) as total_party,
  sum(party_size) FILTER (WHERE status='confirmed') as confirmed_seats
  WHERE wedding_id = :id
← {total_guests, by_status{}, total_party_size, confirmed_seats}
```
Single aggregate query — no N+1. Cheap at v1 volumes; add a cached counter only if it hurts.

---

## 4. API contract conventions

- Base `/api/v1`. JSON only. Bearer access token in `Authorization`.
- Envelope: `{ "data": <T>|null, "error": {code,message,details?}|null }`.
- Pagination: `?page=&page_size=` (default 50, max 200), response `data.items` + `data.meta{page,page_size,total}`.
- Filtering guests: `?status=&side=&category=&q=` (q = ILIKE on full_name).
- Idempotency: `join` is idempotent; guest create is not (client dedup by name is a later feature).

### Error taxonomy (stable `error.code` for the Flutter client)
| HTTP | code | Meaning |
|---|---|---|
| 400 | `validation_error` | DTO failed validation (details lists fields) |
| 401 | `unauthenticated` | missing/expired/invalid access token |
| 403 | `forbidden` | authenticated but role too low |
| 404 | `not_found` | resource absent **or** caller not a member (deliberate ambiguity) |
| 409 | `conflict` | username/share_code collision, duplicate membership |
| 423 | `locked` | account locked after N failed PIN attempts |
| 429 | `rate_limited` | login/recover throttled |
| 500 | `internal` | unexpected; logged with request-id |

### Error handling & retries
- **Client-safe retries:** GETs and `/auth/refresh` are safe to retry; POSTs are not (no idempotency keys in v1).
- **DB errors:** unique-violation → map to 409; not-found → 404; else 500 + log. Never leak SQL.
- **Panics:** recover middleware → 500 with request-id; never crash the process.
- **Timeouts:** per-request context deadline (e.g. 5s) so a slow query can't pile up connections.

---

## 5. Data model notes (impl-level)

Indexes (correctness + perf):
```
users(username) UNIQUE
weddings(share_code) UNIQUE
weddings(owner_id)
memberships(wedding_id, user_id) UNIQUE      -- prevents double-join, powers authz lookup
memberships(user_id)                          -- "list my weddings"
guests(wedding_id)                            -- every guest query is wedding-scoped
guests(wedding_id, status)                    -- filter + stats
guests(wedding_id, full_name)                 -- search (ILIKE; consider trigram later)
```
- **Cascade:** delete wedding → cascade memberships + guests (owner-only, confirmed action).
- **Auth loads one row:** `memberships(wedding_id,user_id)` is the hot authz path — covered by the unique index.
- **Last-write-wins** on guest edits in v1 (no optimistic locking). If two family members edit the same guest concurrently, last save wins — acceptable at this scale; add `version`/`updated_at` check if it bites.

---

## 6. Scale & reliability

### Load estimate (v1)
- 5k users × light usage ≈ well under 1 rps sustained, small bursts.
- Heaviest query = guest list + stats per wedding; indexed, single-table, trivial.
- **Conclusion:** one small API instance + one small Postgres handles v1 with huge headroom.

### Scaling path (only when metrics say so)
1. **Vertical first** — bigger VM/DB is the cheapest next step.
2. **Horizontal API** — API is stateless (JWT); run N replicas behind a LB. No code change.
3. **Read load** — add `pgbouncer`; add read replica only if reads dominate and hurt.
4. **Stats hot?** — precompute per-wedding counters via triggers or a cached row.
5. **Token revocation at scale** — DB-backed refresh denylist before reaching for Redis (ADR-005).

### Reliability
- **State:** all durable state in Postgres → reliability = DB reliability. **Daily automated backups + tested restore** is the single most important ops task.
- **Failover:** v1 accepts single-instance downtime; managed Postgres w/ standby when it matters.
- **Deploy:** Docker image; `docker compose` for local, single container/host for v1. Health check `GET /healthz` (liveness) + `GET /readyz` (DB ping).
- **Graceful shutdown:** drain in-flight requests, close DB pool on SIGTERM.

### Monitoring (v1-minimum → grow)
- Structured slog with request-id on every line; log 4xx/5xx with code.
- `/metrics` (Prometheus) later: request rate, p95 latency, error rate, DB pool usage.
- Alert (later): 5xx rate spike, DB connection saturation, backup failure.

---

## 7. Trade-offs made explicit
| Decision | Gained | Gave up | Revisit when |
|---|---|---|---|
| No cache/queue in v1 | Simplicity | Some latency headroom | stats/list p95 > target |
| Stateless JWT | No session infra | Instant revocation | account-takeover reports |
| Last-write-wins guests | No locking code | Concurrent-edit safety | families report overwrites |
| GORM + AutoMigrate | Build speed | SQL control, safe prod migrations | before first real users |
| 404-hides-existence authz | No info leak | Slightly confusing debugging | never (keep it) |
| Single region/instance | Cost, simplicity | HA | paying users depend on uptime |

---

## 8. What I'd revisit as it grows
1. **Import the design** — could add seating/invite-send modules that introduce a queue + SMS provider, changing this diagram. (Still blocked.)
2. Versioned migrations before any production data exists.
3. Refresh-token denylist table once real users log in from multiple devices.
4. Trigram index on `guests.full_name` if search feels slow on big lists.
5. Precomputed stats counters if the stats query ever shows up in slow logs.
```
