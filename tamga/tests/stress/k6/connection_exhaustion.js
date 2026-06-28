import http from 'k6/http';
import { check } from 'k6';

// Connection pool exhaustion test: 5000 concurrent connections.
// Expect PostgreSQL max_connections (default 100) to be hit,
// and file descriptor / TIME_WAIT pileup.

export const options = {
  scenarios: {
    high_concurrency: {
      executor: 'per-vu-iterations',
      vus: 5000,
      iterations: 1,
      maxDuration: '30m',
    },
  },
  thresholds: {
    // Very permissive — we expect failures at this concurrency level
    'http_req_failed': ['rate<0.50'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

const BASE = __ENV.TAMGA_BASE_URL || 'http://localhost:8443';
const API_KEY = __ENV.TAMGA_API_KEY || 'test-key';

export default function () {
  const res = http.get(`${BASE}/api/v1/stats`, {
    headers: { 'x-api-key': API_KEY },
    timeout: '30s',
  });
  check(res, {
    'responded': (r) => r.status > 0,
  });
}
