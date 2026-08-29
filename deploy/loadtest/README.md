# Load testing (k6)

Baseline the API's throughput, latency, and breaking point. Run against a
**non-production** instance with a disposable database (the script creates users
and weddings).

## Install k6

```bash
brew install k6        # macOS
# or: https://k6.io/docs/get-started/installation/
```

## Run

Start the API + a throwaway DB (dev compose), then:

```bash
# Smoke: one user, one iteration — verify the flow works
k6 run --vus 1 --iterations 1 deploy/loadtest/script.js

# Full ramp (0 -> 50 VUs), against a custom host
BASE_URL=http://localhost:8080/api/v1 k6 run deploy/loadtest/script.js
```

> Disable the rate limiter for capacity testing (`RATE_LIMIT_RPS=0`), otherwise
> you measure the limiter, not the app.

## What it does

Per iteration: register → create wedding → add 5 guests → list guests → stats →
notifications. Thresholds mirror the SLOs: `p95 < 800ms`, `<2%` errors.

## Using the results

- **p95/p99** at your target concurrency → confirm/tune the SLO thresholds in
  `../observability/SLO.md`.
- **Breaking point** (raise `target` VUs until errors climb) → set the
  `HighInFlightRequests` alert threshold and size `DB_MAX_OPEN_CONNS`.
- **Throughput** (req/s) → informs `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST`.

Capture a run before launch and re-run after significant changes.
