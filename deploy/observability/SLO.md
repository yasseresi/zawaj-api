# Zawaj API — SLOs & Alerting

Service Level Objectives for the backend, measured from the RED metrics exposed
at `GET /metrics`. Alerts live in [`prometheus-rules.yml`](./prometheus-rules.yml);
scrape config in [`prometheus-scrape.yml`](./prometheus-scrape.yml).

## SLOs (starting targets — tune with real traffic)

| SLO | Target | Metric |
|-----|--------|--------|
| Availability (non-5xx) | 99.9% / 30d | `http_requests_total{status!~"5.."}` / total |
| Latency | p95 < 800ms, p99 < 1.5s | `http_request_duration_seconds_bucket` |
| Readiness | `/readyz` 200 | probe (blackbox / LB health) |

**Error budget** at 99.9% = 0.1% of requests (~43m/30d of full downtime-equivalent).

## Alerts (see rules file)

| Alert | Condition | Severity |
|-------|-----------|----------|
| `ZawajApiDown` | `up == 0` 2m | critical |
| `HighErrorRateWarning` | 5xx > 2% for 10m | warning (rollback trigger) |
| `HighErrorRateCritical` | 5xx > 5% for 5m | critical (roll back) |
| `HighLatencyP95` | p95 > 800ms for 10m | warning |
| `HighInFlightRequests` | in-flight > 100 for 5m | warning (saturation) |
| `ErrorBudgetFastBurn` | >14.4x burn on 1h+5m | critical |

## Wiring

1. Scrape `/metrics` (internal only).
2. Load the rules file into Prometheus.
3. Route alerts via Alertmanager to your on-call (PagerDuty/Slack/email).
4. For readiness, add a blackbox probe or use the load balancer's health check on
   `/readyz`.
5. Tune thresholds after a week of real traffic; the 2%/800ms values match the
   deploy-checklist rollback triggers.

## Notes

- In-flight saturation threshold (100) is a placeholder — set it from load-test
  capacity numbers (P2 item).
- Latency/error alerts are service-wide; add `route`-scoped variants once you know
  which endpoints matter most.
