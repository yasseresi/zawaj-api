#!/usr/bin/env bash
#
# setup-db.sh — bootstrap the local Postgres role + database the API expects.
#
# The API auto-runs SQL migrations on boot; it does NOT create its own role or
# database. This script fills that gap idempotently so `make run` works from a
# clean machine without any manual psql steps.
#
# It parses DATABASE_URL from backend/.env (falling back to the compose default),
# connects to the local server as a Postgres SUPERUSER, and ensures:
#   - a LOGIN role with the URL's user/password (CREATEDB granted)
#   - a database owned by that role
#   - ownership of the public schema handed to that role
#
# Flags:
#   --reset    Drop and recreate the database first (empty state). Use this when
#              migrations fail with "table already exists" after a partial/older
#              init. Destroys all local data in that database.
#   --superuser NAME   Superuser role to connect as (default: current OS user,
#                      then "postgres"). Override for non-standard installs.
#
# Usage:
#   ./scripts/setup-db.sh
#   ./scripts/setup-db.sh --reset
#   ./scripts/setup-db.sh --superuser postgres
#
set -euo pipefail

# --- locate repo + .env -----------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${BACKEND_DIR}/.env"

# Compose/dev default if .env has no DATABASE_URL.
DEFAULT_URL="postgres://zawaj:zawaj@localhost:5432/zawaj?sslmode=disable"

# --- args -------------------------------------------------------------------
RESET=0
SUPERUSER=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --reset) RESET=1; shift ;;
    --superuser) SUPERUSER="${2:-}"; shift 2 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# --- read DATABASE_URL from .env (without sourcing arbitrary shell) ----------
DATABASE_URL=""
if [[ -f "${ENV_FILE}" ]]; then
  DATABASE_URL="$(grep -E '^\s*DATABASE_URL=' "${ENV_FILE}" | tail -n1 | cut -d= -f2- || true)"
fi
DATABASE_URL="${DATABASE_URL:-$DEFAULT_URL}"

# --- parse the URL ----------------------------------------------------------
# postgres://USER:PASS@HOST:PORT/DBNAME?params
proto_stripped="${DATABASE_URL#*://}"
creds="${proto_stripped%%@*}"
hostpart="${proto_stripped#*@}"

DB_USER="${creds%%:*}"
DB_PASS="${creds#*:}"
[[ "${DB_PASS}" == "${creds}" ]] && DB_PASS=""   # no password in URL

hostport="${hostpart%%/*}"
DB_HOST="${hostport%%:*}"
DB_PORT="${hostport#*:}"
[[ "${DB_PORT}" == "${hostport}" ]] && DB_PORT="5432"

dbpart="${hostpart#*/}"
DB_NAME="${dbpart%%\?*}"

# Docker/compose host aliases aren't reachable from the host shell — psql runs
# against a locally listening server.
if [[ "${DB_HOST}" == "db" ]]; then DB_HOST="localhost"; fi

echo "Target: role=${DB_USER} db=${DB_NAME} host=${DB_HOST}:${DB_PORT}"

# --- preflight --------------------------------------------------------------
if ! command -v psql >/dev/null 2>&1; then
  echo "error: psql not found on PATH. Install Postgres client tools." >&2
  echo "  brew install libpq && brew link --force libpq   # macOS" >&2
  exit 1
fi

# Pick a superuser to connect as: explicit flag, else current OS user, else postgres.
CANDIDATES=()
[[ -n "${SUPERUSER}" ]] && CANDIDATES+=("${SUPERUSER}")
CANDIDATES+=("${USER:-}" "postgres")

SU=""
for cand in "${CANDIDATES[@]}"; do
  [[ -z "${cand}" ]] && continue
  if psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${cand}" -d postgres -tAc 'SELECT 1' >/dev/null 2>&1; then
    SU="${cand}"; break
  fi
done

if [[ -z "${SU}" ]]; then
  echo "error: could not connect to Postgres at ${DB_HOST}:${DB_PORT} as a superuser." >&2
  echo "  Is the server running?  brew services start postgresql   (or use: make up)" >&2
  echo "  Try passing one explicitly:  ./scripts/setup-db.sh --superuser postgres" >&2
  exit 1
fi
echo "Connected as superuser: ${SU}"

psu() { psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${SU}" -d postgres -v ON_ERROR_STOP=1 "$@"; }

# --- role -------------------------------------------------------------------
if [[ -n "$(psu -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'")" ]]; then
  echo "role '${DB_USER}' exists — ensuring password + CREATEDB"
  psu -c "ALTER ROLE \"${DB_USER}\" WITH LOGIN CREATEDB PASSWORD '${DB_PASS}';" >/dev/null
else
  echo "creating role '${DB_USER}'"
  psu -c "CREATE ROLE \"${DB_USER}\" WITH LOGIN CREATEDB PASSWORD '${DB_PASS}';" >/dev/null
fi

# --- optional reset ---------------------------------------------------------
if [[ "${RESET}" -eq 1 ]]; then
  echo "--reset: dropping database '${DB_NAME}'"
  psu -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='${DB_NAME}' AND pid<>pg_backend_pid();" >/dev/null || true
  psu -c "DROP DATABASE IF EXISTS \"${DB_NAME}\";" >/dev/null
fi

# --- database ---------------------------------------------------------------
if [[ -n "$(psu -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'")" ]]; then
  echo "database '${DB_NAME}' exists — ensuring owner is '${DB_USER}'"
  psu -c "ALTER DATABASE \"${DB_NAME}\" OWNER TO \"${DB_USER}\";" >/dev/null
else
  echo "creating database '${DB_NAME}' owned by '${DB_USER}'"
  psu -c "CREATE DATABASE \"${DB_NAME}\" OWNER \"${DB_USER}\";" >/dev/null
fi

# --- schema ownership (so migrations can CREATE freely) ---------------------
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${SU}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 \
  -c "ALTER SCHEMA public OWNER TO \"${DB_USER}\";" \
  -c "GRANT ALL ON SCHEMA public TO \"${DB_USER}\";" >/dev/null

echo "✓ Database ready. Migrations run automatically on 'make run'."
