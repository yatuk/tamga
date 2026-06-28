import http from 'k6/http';
import { check } from 'k6';

// Constant low-rate traffic for soak testing (memory leak detection).
// Run for 2 hours at 100 RPS with clean prompts.
// Monitored externally via pprof heap snapshots and docker stats.

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: 100,
      timeUnit: '1s',
      duration: '7200s', // 2 hours
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
  },
  thresholds: {
    'http_req_duration{status:200}': ['p(95)<500', 'p(99)<1000'],
    'http_req_failed': ['rate<0.05'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

const BASE = __ENV.TAMGA_BASE_URL || 'http://localhost:8443';
const API_KEY = __ENV.TAMGA_API_KEY || 'test-key';

const PAYLOAD = JSON.stringify({
  model: 'claude-3-haiku-20240307',
  messages: [{ role: 'user', content: 'Tell me a fun fact about space.' }],
});

export default function () {
  const res = http.post(`${BASE}/v1/messages`, PAYLOAD, {
    headers: {
      'Content-Type': 'application/json',
      'x-api-key': API_KEY,
    },
  });
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
