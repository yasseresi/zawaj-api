# Zawaj — Backend Specification (v1)

> Collaborative wedding **guest-list** app. Families build one shared list of invitees
> (*visiteurs*) so nobody is forgotten, no paper, no double-calling. This spec covers the
> **Go + Gin backend only**. Flutter mobile app comes later and consumes this API.

**Status:** draft — awaiting design import to finalize guest fields.
**Last decisions locked:** collaboration roles (owner/editor/viewer); v1 scope = shared list + RSVP tracking; DB = Postgres + GORM; auth = username + PIN + JWT.

---

## 1. Objective

Let a wedding organizer create a wedding, invite family members to co-manage a single
shared guest list, add/track invitees, and follow each guest's RSVP status
(to-invite → called → confirmed → declined) with live counts of guests and seats.

**Target users:** wedding organizers and their families in a community that does **not**
use email or phone-number login. Auth must be near-zero friction: a chosen **username** +
short **PIN**.

**Out of scope for v1 (later phases):** sending invitations (SMS/WhatsApp), seating/tables,
phone-contacts import, push notifications, admin/moderation, analytics.

### Acceptance criteria (v1 "done")
- A user can register with username + PIN, receive a recovery code, and log in.
- A user can create a wedding and becomes its **owner**.
- An owner can generate a **share code**; another user can join via that code as **editor** or **viewer**.
- **Editors** can add/edit/delete guests and change RSVP status; **viewers** are read-only; **owner** additionally manages members and wedding settings.
- Guests can be filtered by status / side / category and searched by name.
- A stats endpoint returns counts by status and total confirmed seats.
- All wedding/guest endpoints enforce membership + role. Non-members get 403/404.
- Automated tests cover auth, role enforcement, and guest CRUD.

---

## 2. Tech stack & commands

- **Language:** Go 1.22+
- **Web:** Gin
- **DB:** PostgreSQL 15+ via **GORM** (auto-migrate for v1; switch to versioned migrations before prod)
- **Auth:** username + PIN (bcrypt), JWT access + refresh (golang-jwt), one-time recovery code
- **Config:** env vars via `.env` (godotenv) → typed `config.Config`
- **Validation:** `go-playground/validator` on DTOs
- **UUIDs:** google/uuid
- **Logging:** slog (structured)
- **Lint:** golangci-lint · **Format:** gofmt/goimports
- **Local infra:** docker-compose (Postgres + api)

**Commands (Makefile targets):**
```bash
make run          # go run ./cmd/api
make test         # go test ./... -race -cover
make lint         # golangci-lint run
make up           # docker compose up -d  (postgres + api)
make down         # docker compose down
make seed         # optional: seed dev data
```

---

## 3. Project structure

```
backend/
├── cmd/api/main.go            # entrypoint: load config, db, router, serve
├── internal/
│   ├── config/                # env → typed Config
│   ├── database/              # gorm connect + AutoMigrate
│   ├── models/                # gorm models (User, Wedding, Membership, Guest, ActivityLog)
│   ├── dto/                   # request/response structs + validation tags
│   ├── auth/                  # pin hashing, jwt issue/verify, recovery codes
│   ├── middleware/            # RequireAuth, RequireRole, CORS, request logging, recovery
│   ├── repository/            # gorm data access, one file per aggregate
│   ├── service/              # business logic (auth, wedding, guest, member, stats)
│   ├── handler/               # gin handlers, grouped by resource
│   └── router/                # route registration + group wiring
├── pkg/                       # shared helpers (response envelope, errors)
├── go.mod
├── .env.example
├── Makefile
├── Dockerfile
└── docker-compose.yml
```

**Layering (dependency direction):** handler → service → repository → models. Handlers never
touch GORM directly; services hold rules; repositories hold queries.

---

## 4. Data model

Enums:
- **Role:** `owner` | `editor` | `viewer`
- **RSVP status:** `to_invite` | `called` | `confirmed` | `declined`
- **Side:** `groom` | `bride` | `both` | `none`
- **Category:** free string (e.g. family/friends/work/neighbors) — not enforced in v1

```
User
  id            uuid pk
  username      string unique (3–30, lowercase alnum + _)
  display_name  string
  pin_hash      string        (bcrypt of 4–6 digit PIN)
  recovery_hash string        (bcrypt of one-time recovery code)
  created_at, updated_at

Wedding
  id            uuid pk
  name          string
  event_date    date (nullable)
  location      string (nullable)
  owner_id      uuid fk -> User
  share_code    string unique (short, regeneratable)
  created_at, updated_at

Membership                    (join: which users manage which weddings)
  id            uuid pk
  wedding_id    uuid fk -> Wedding
  user_id       uuid fk -> User
  role          Role
  joined_at
  UNIQUE(wedding_id, user_id)

Guest  (invitee / visiteur)
  id            uuid pk
  wedding_id    uuid fk -> Wedding
  added_by      uuid fk -> User
  full_name     string (required)
  phone         string (nullable, optional)
  side          Side default 'none'
  category      string (nullable)
  party_size    int default 1     (# people in this invite = invitee + family; e.g. "Ahmed +5" = 6)
  status        RSVP default 'to_invite'
  notes         string (nullable)
  created_at, updated_at

ActivityLog  (optional in v1; audit "who added/changed whom")
  id, wedding_id, user_id, action, entity_type, entity_id, meta jsonb, created_at

Notification  (in-app; recipient's feed)
  id            uuid pk
  user_id       uuid fk -> User      (recipient)
  wedding_id    uuid fk -> Wedding (nullable)
  type          string   (member_joined | guest_added | guest_updated |
                          guest_status_changed | role_changed | member_removed)
  title         string
  body          string
  data          jsonb (nullable)     (ids for client deep-link)
  read_at       timestamp (nullable)
  created_at
  INDEX(user_id, read_at)             (unread feed + counts)
```

> ⚠ Guest fields (`side`, `category`, `party_size`, `notes`, phone) are provisional — confirm
> against the imported design (`screens-*.jsx`) before build.

---

## 5. API surface (`/api/v1`)

Standard JSON envelope: `{ "data": ..., "error": null }` or `{ "data": null, "error": {code,message} }`.

**Auth**
```
POST /auth/register     {username, display_name, pin}  -> {user, access, refresh, recovery_code}
POST /auth/login        {username, pin}                -> {access, refresh}
POST /auth/refresh      {refresh}                       -> {access, refresh}
POST /auth/recover      {username, recovery_code, new_pin} -> {access, refresh, recovery_code}
GET  /me                                                 -> current user
PATCH /me               {display_name?}                  -> user
```

**Weddings**
```
POST   /weddings                {name, event_date?, location?}  -> wedding (caller = owner)
GET    /weddings                                                -> weddings caller belongs to
GET    /weddings/:id                                            -> wedding + my role
PATCH  /weddings/:id            {name?, event_date?, location?}  (owner)
DELETE /weddings/:id                                             (owner)
POST   /weddings/:id/share                                       (owner) -> regenerate share_code
POST   /weddings/join           {share_code}                     -> membership (default role: viewer)
```

**Members**
```
GET    /weddings/:id/members                                    (any member)
PATCH  /weddings/:id/members/:userId   {role}                    (owner)
DELETE /weddings/:id/members/:userId                             (owner; or self = leave)
```

**Guests**
```
GET    /weddings/:id/guests    ?status=&side=&category=&q=&page= (any member)
POST   /weddings/:id/guests    {full_name, phone?, side?, category?, party_size?, notes?}  (editor+)
GET    /weddings/:id/guests/:guestId                            (any member)
PATCH  /weddings/:id/guests/:guestId  {...any field, status?}   (editor+)
DELETE /weddings/:id/guests/:guestId                            (editor+)
```

**Stats**
```
GET /weddings/:id/stats  -> { total_guests, by_status:{...}, total_party_size, confirmed_seats }
```

**Notifications** (recipient = current user)
```
GET   /notifications        ?unread=true&page=   -> paginated feed
GET   /notifications/unread_count                -> { count }
PATCH /notifications/:id/read                     -> marks one read
POST  /notifications/read_all                     -> marks all read
```

### Notification generation (the `Notifier` seam)
Guest/member **services** never build notifications directly — they call a `Notifier`
interface (`Notify(ctx, event)`). The notifications module implements it and fans an event
out to the right recipients (all *other* members of the wedding). This decouples the
producers (guest/member) from the consumer (notifications) so they build in parallel.
v1 delivery = stored rows, client polls. FCM push + WebSocket = later phase.

Events → recipients:
| Event | Emitted by | Recipients |
|---|---|---|
| `member_joined` | join wedding | owner + editors |
| `role_changed` | member role update | the affected member |
| `member_removed` | member delete | the affected member |
| `guest_added` / `guest_updated` / `guest_status_changed` | guest write | all other members |

### Role matrix
| Action | viewer | editor | owner |
|---|---|---|---|
| Read wedding / guests / members / stats | ✅ | ✅ | ✅ |
| Guest create/edit/delete + status change | ❌ | ✅ | ✅ |
| Edit wedding settings | ❌ | ❌ | ✅ |
| Regenerate share code | ❌ | ❌ | ✅ |
| Manage member roles / remove members | ❌ | ❌ | ✅ |
| Delete wedding | ❌ | ❌ | ✅ |
| Leave wedding (self) | ✅ | ✅ | owner must transfer first |

---

## 6. Auth design (low-friction)

- **Register:** username (unique, validated) + PIN (4–6 digits). Server bcrypt-hashes PIN,
  generates a random **recovery code** (e.g. 10 chars), returns it **once** in plaintext,
  stores only its bcrypt hash. UI must tell user to save it (no email/phone to reset).
- **Login:** username + PIN → access JWT (short TTL ~15m) + refresh JWT (long TTL ~30d).
- **Recover:** username + recovery code + new PIN → resets PIN, issues fresh recovery code.
- **Middleware:** `RequireAuth` parses Bearer access token → sets `userID` in ctx.
  `RequireRole(min)` loads membership for `:id` and checks role ≥ min.
- **Rate-limit** login/recover to blunt PIN brute force (PIN space is small). Lockout after N fails.

---

## 7. Testing strategy

- **Unit:** services with mocked repositories (auth flow, role checks, status transitions, stats math). Table-driven.
- **Integration:** handlers against a real Postgres (docker or testcontainers); cover auth happy/sad paths, role enforcement (403 for viewer writes, 404 for non-member), guest CRUD, stats.
- **Target:** every endpoint has ≥1 happy + ≥1 authz-denied test. `go test -race -cover`.
- Approach: TDD where practical (auth + role guards first — highest risk).

---

## 8. Boundaries

**Always**
- Enforce membership + role on every wedding-scoped route (default deny).
- Hash secrets (PIN, recovery code) with bcrypt; never log or return them (except recovery code once, at issue).
- Validate + bind all input via DTOs; never bind GORM models straight from request bodies.
- Scope every query by `wedding_id` derived from the authenticated membership, not from the body.

**Ask first**
- Adding external services (SMS, push, storage).
- Schema changes that drop columns/data.
- Switching auth model or exposing any endpoint unauthenticated.

**Never**
- Store PIN/recovery code in plaintext.
- Trust `wedding_id`/`role` from the request body.
- Return another user's data without a shared-membership check.
- Hard-delete a wedding without owner confirmation semantics.

---

## 9. Decisions & open questions
**Resolved (2026-08-26):**
- Join role default = **viewer** (owner promotes to editor). ✅
- Guest row = one invite; `party_size` = **headcount incl. family** ("Ahmed +5" = one row, party_size 6). ✅
- Scale = up to ~**900 guests/wedding** — comfortably within single-Postgres design. ✅

**Still open:**
1. **Design import** — read `screens-*.jsx` / `styles.css` to confirm guest fields, statuses, any missed screen. (Blocked: needs claude.ai connector auth or local files.) Blocks **P4 (guests)** only.
2. Multiple weddings per user, or one? Spec assumes **many** — confirm.
3. Recovery-code UX acceptable, or prefer a no-secret "account code" login? Spec assumes recovery code.
```
