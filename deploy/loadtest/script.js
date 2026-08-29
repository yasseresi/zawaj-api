// k6 load test for the Zawaj API. Models the core authenticated flow:
// register -> create wedding -> add guests -> list -> stats -> notifications.
//
// Run against a NON-production instance:
//   BASE_URL=http://localhost:8080/api/v1 k6 run script.js
// Smoke (quick sanity):
//   k6 run --vus 1 --iterations 1 script.js
//
// Ramp/thresholds mirror the deploy SLOs (p95 < 800ms, <2% errors). Adjust VUs
// to find the breaking point, then use the numbers to size the DB pool and the
// rate limiter (RATE_LIMIT_RPS/BURST) and the SLO saturation alert.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
const errors = new Rate('business_errors');

export const options = {
  stages: [
    { duration: '30s', target: 20 }, // ramp up
    { duration: '1m', target: 50 },  // sustain
    { duration: '30s', target: 0 },  // ramp down
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],          // <2% transport errors
    http_req_duration: ['p(95)<800'],        // p95 < 800ms
    business_errors: ['rate<0.02'],          // <2% non-2xx business responses
  },
};

function jsonHeaders(token) {
  const h = { 'Content-Type': 'application/json' };
  if (token) h['Authorization'] = `Bearer ${token}`;
  return { headers: h };
}

export default function () {
  // Unique user per iteration (username has no spaces; password >= 8).
  const username = `load_${__VU}_${__ITER}_${Date.now()}`;
  const reg = http.post(`${BASE}/auth/register`, JSON.stringify({
    username, display_name: 'Load', password: 'password123',
  }), jsonHeaders());
  const okReg = check(reg, { 'register 201': (r) => r.status === 201 });
  errors.add(!okReg);
  if (!okReg) { sleep(1); return; }

  const access = reg.json('data.access');

  const wed = http.post(`${BASE}/weddings`, JSON.stringify({ name: 'Load Wedding' }), jsonHeaders(access));
  const okWed = check(wed, { 'wedding 201': (r) => r.status === 201 });
  errors.add(!okWed);
  if (!okWed) { sleep(1); return; }

  const wid = wed.json('data.id');

  for (let i = 0; i < 5; i++) {
    const g = http.post(`${BASE}/weddings/${wid}/guests`, JSON.stringify({
      full_name: `Guest ${i}`, status: 'pending',
    }), jsonHeaders(access));
    errors.add(!check(g, { 'guest 201': (r) => r.status === 201 }));
  }

  const list = http.get(`${BASE}/weddings/${wid}/guests?page_size=50`, jsonHeaders(access));
  errors.add(!check(list, { 'list 200': (r) => r.status === 200 }));

  const stats = http.get(`${BASE}/weddings/${wid}/stats`, jsonHeaders(access));
  errors.add(!check(stats, { 'stats 200': (r) => r.status === 200 }));

  const notifs = http.get(`${BASE}/notifications`, jsonHeaders(access));
  errors.add(!check(notifs, { 'notifs 200': (r) => r.status === 200 }));

  sleep(1);
}
