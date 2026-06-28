import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: 500,
      timeUnit: '1s',
      duration: '60s',
      preAllocatedVUs: 200,
      maxVUs: 600,
    },
  },
  thresholds: {
    'http_req_duration{status:200}': ['p(95)<200', 'p(99)<500'],
    'http_req_failed': ['rate<0.05'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

const BASE = __ENV.TAMGA_BASE_URL || 'http://localhost:8443';
const API_KEY = __ENV.TAMGA_API_KEY || 'test-key';

const PAYLOAD = JSON.stringify({
  model: 'claude-3-haiku-20240307',
  messages: [{ role: 'user', content: 'What is 2+2?' }],
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
