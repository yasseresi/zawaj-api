# Zawaj — Design-Derived Backend Spec

Authoritative mapping of the **12 UI screens** (`زواج/*.jsx`) to backend schema + endpoints,
with v1/later phasing. Supersedes the schema/endpoint sketch in [SPEC.md](../SPEC.md) §4–5
where they differ. Arabic UI, RTL, Arabic + English.

## Screen → capability map
| # | Screen | Backend needs |
|---|---|---|
| 1 | Onboarding (3 slides) | none (client) |
| 2 | Signup / Login / Forgot password | auth (✅ built) + password reset |
| 3 | Home — "my lists" | list weddings w/ role + guest count + date; delete |
| 4 | Create list | create wedding (name, date, description) |
| 5 | Guest list | guests: search, filter tabs (all/confirmed/pending/declined + counts), delete |
| 6 | Add/Edit guest sheet | guest create/update (name, contact, status, companions, note) |
| 7 | Guest detail | guest + notes thread + change history + edit status/add note |
| 8 | Analytics (dark) | stats: totals, distribution, RSVP-over-time; send reminders |
| 9 | Activity log | wedding activity feed (grouped by day) |
| 10 | Share & Export | collaborators (invite/list/remove/role), share link + QR, export CSV/PDF |
| 11 | Pending invitations | incoming collab invites: accept / decline |
| 12 | Settings | profile edit, change password, notif prefs, linked OAuth, premium, logout |

---

## Data model (design-accurate)

Enums:
- **Role:** `owner` \| `editor` \| `viewer`
- **RSVP status:** `pending` \| `confirmed` \| `declined` (default `pending`; UI "مدعو/invited" = pending)
- **Invitation status:** `pending` \| `accepted` \| `declined`
- **Activity action:** `guest_added` \| `guest_updated` \| `guest_status_changed` \| `note_added` \| `invite_sent` \| `reminder_sent` \| `collaborator_joined` \| `list_exported`

```
User
  id, username(unique), display_name, password_hash, recovery_hash
  email            nullable   -- optional; used for OAuth + email invites + notifications
  is_premium       bool=false            [later]
  dark_mode        bool=false
  notif_push       bool=true
  notif_email      bool=true
  notif_rsvp       bool=true
  failed_attempts, locked_until, timestamps

Wedding  (a "list")
  id, name, event_date(nullable date), description(nullable)
  owner_id -> User
  share_code(unique)        -- powers zawaj.app/i/{code} link + QR
  timestamps

Membership
  id, wedding_id, user_id, role, joined_at
  UNIQUE(wedding_id, user_id)

Invitation  (collaborator invite — screen 11)
  id, wedding_id, inviter_id -> User
  invitee_email nullable  |  invitee_username nullable   -- one identifies target
  role (editor|viewer), status (pending|accepted|declined)
  created_at, responded_at

Guest  (invitee / مدعو)
  id, wedding_id, added_by -> User
  full_name (required)
  contact       nullable    -- phone OR email, single field (UI: "الهاتف أو البريد")
  relationship  nullable    -- "صديقة العروس"
  status        default 'pending'
  companions    int=0       -- "عدد المرافقين" (accompanying people, excludes the guest)
  table_label   nullable    -- "طاولة رقم 4"
  meal          nullable    -- "نباتي"
  invited_at    nullable    -- set when invite/reminder sent   [later delivery]
  timestamps

GuestNote  (screen 7 — multiple authored notes per guest)
  id, guest_id, author_id -> User, body, created_at

ActivityLog  (screen 9 feed + screen 7 per-guest history)
  id, wedding_id, actor_id -> User, guest_id nullable
  action (enum above), meta jsonb, created_at
  INDEX(wedding_id, created_at), INDEX(guest_id, created_at)

Notification  (personal push/updates)   [P5b]
  id, user_id, wedding_id nullable, type, title, body, data jsonb, read_at, created_at
  INDEX(user_id, read_at)

LinkedAccount  (OAuth Google/Facebook/Apple — screen 12)   [later]
  id, user_id, provider, provider_uid, created_at
```

Seat math: a guest counts as `1 + companions` people. `confirmed_seats = Σ(1+companions)` over
confirmed guests. "Total guests" in analytics = count of guest rows (245 in mock).

---

## API surface (design-accurate, phased)

`[v1]` build now · `[later]` after v1. All under `/api/v1`, JSON envelope, Bearer auth.

**Auth** `[v1]` (✅ mostly built)
```
POST /auth/register {username, display_name, password}   ✅
POST /auth/login {username, password}                     ✅
POST /auth/refresh {refresh}                              ✅
POST /auth/recover {username, recovery_code, new_password} ✅  (= "forgot password")
GET/PATCH /me                                             ✅
PATCH /me/password {old_password, new_password}          [v1] change-password (screen 12)
PATCH /me/settings {dark_mode?, notif_push?, notif_email?, notif_rsvp?, email?}  [v1]
```

**Weddings / lists** `[v1]`
```
POST   /weddings {name, event_date?, description?}
GET    /weddings                       -> [{..., my_role, guest_count}]
GET    /weddings/:id
PATCH  /weddings/:id                    (owner)
DELETE /weddings/:id                    (owner)
POST   /weddings/:id/share              (owner) regenerate share_code
POST   /weddings/join {share_code}      -> membership (default viewer)
```

**Members & collaborator invitations** `[v1]`
```
GET    /weddings/:id/members
PATCH  /weddings/:id/members/:userId {role}     (owner)
DELETE /weddings/:id/members/:userId            (owner or self-leave)
POST   /weddings/:id/invitations {email|username, role}   (owner) -> Invitation
GET    /invitations                             -> my pending incoming invites (screen 11)
POST   /invitations/:id/accept                  -> creates membership
POST   /invitations/:id/decline
```

**Guests** `[v1]`
```
GET    /weddings/:id/guests ?status=&q=&page=   -> items + tab counts {all,confirmed,pending,declined}
POST   /weddings/:id/guests {full_name, contact?, status?, companions?, relationship?, table_label?, meal?, note?}   (editor+)
GET    /weddings/:id/guests/:gid                -> guest + notes + history
PATCH  /weddings/:id/guests/:gid {...}          (editor+)
DELETE /weddings/:id/guests/:gid                (editor+)
POST   /weddings/:id/guests/:gid/notes {body}   (editor+)  -> GuestNote
PATCH  /weddings/:id/guests/:gid/status {status} (editor+) -> logs activity
```

**Stats / analytics** `[v1]`
```
GET /weddings/:id/stats  -> {total_guests, by_status{confirmed,pending,declined}, total_people, confirmed_seats}
GET /weddings/:id/stats/timeline ?bucket=week  -> RSVP-over-time series   [v1-lite or later]
```

**Activity feed** `[v1]`
```
GET /weddings/:id/activity ?page=  -> activity grouped by day (screen 9)
```

**Export** 
```
GET /weddings/:id/export.csv   [v1]   -> CSV of guests
GET /weddings/:id/export.pdf   [later] -> printable PDF
```

**Notifications (personal)** `[P5b]`
```
GET /notifications ?unread= ; GET /notifications/unread_count ; PATCH /notifications/:id/read ; POST /notifications/read_all
```

**Later (need external services / big scope)**
```
POST /weddings/:id/reminders            send reminders to pending guests (SMS/WhatsApp/email)  [later]
POST /weddings/:id/guests/:gid/invite   send invite to a guest                                 [later]
OAuth link/unlink Google/Facebook/Apple                                                        [later]
Subscription / premium (منصة الاشتراك)                                                          [later]
Push delivery (FCM), email delivery                                                            [later]
```

---

## ✅ Resolved (2026-08-27)
- **v1 scope:** defer OAuth, premium/subscription, guest reminders/SMS-WhatsApp, PDF export,
  FCM push. **v1 = lists, guests(+notes), roles, collaborator invites (link/QR + accept/decline),
  activity feed, stats, CSV export, notifications table.**
- **Email:** nullable on User. Collaboration primarily via **share link/QR + invite-by-username**;
  email-invite only when the invitee has an email. No email required to sign up.
- Status canonical = `pending|confirmed|declined` (invited = pending). Companions = accompanying
  count (Ahmed +5 → companions 5, total people 6). CSV v1, PDF later.

## ⚠ Original conflicts (now resolved above)
1. **Email.** Earlier: "community has no email." Design uses email for: profile display,
   collaborator invite-by-email, email notifications, OAuth. → Resolution: **email is optional**
   on User; **collaboration primarily via share link/QR + invite-by-username**; email-invite is a
   secondary path only when the invitee has an email. Confirm.
2. **OAuth (Google/Facebook/Apple).** Big scope. Propose **later**, not v1. Confirm.
3. **Premium / subscription.** Monetization. Propose **later**. Confirm.
4. **Send invites/reminders to guests** (SMS/WhatsApp/email). Needs a provider. Propose **later**;
   v1 just tracks status manually. Confirm.
5. **Export:** CSV in v1 (trivial), **PDF later** (needs lib). OK?
6. **Status enum:** design shows both "invited/مدعو" and "pending/معلق". Using **pending** as
   canonical default; treating invited = pending. OK?
7. **Companions:** stored as accompanying count (excludes the guest). Earlier you said
   "Ahmed +5 = 6". Same idea: companions=5, total people=6. Confirm label semantics.
```
