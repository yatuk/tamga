import http from 'k6/http';
import { check } from 'k6';

// Weighted endpoint distribution for dashboard + proxy stress
const ENDPOINTS = [
  { weight: 60, method: 'POST', url: '/v1/messages', body: JSON.stringify({ model: 'claude-3-haiku-20240307', messages: [{ role: 'user', content: 'Hello' }] }) },
  { weight: 15, method: 'GET', url: '/api/v1/events?limit=20', body: null },
  { weight: 10, method: 'GET', url: '/api/v1/stats', body: null },
  { weight: 5, method: 'GET', url: '/api/v1/health/detailed', body: null },
  { weight: 5, method: 'GET', url: '/api/v1/billing/pricing', body: null },
  { weight: 5, method: 'GET', url: '/api/v1/billing/costs/breakdown', body: null },
];

// Build weighted distribution array
const DIST = [];
ENDPOINTS.forEach((e) => {
  for (let i = 0; i < e.weight; i++) DIST.push(e);
});

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: 300,
      timeUnit: '1s',
      duration: '90s',
      preAllocatedVUs: 150,
      maxVUs: 500,
    },
  },
  thresholds: {
    'http_req_duration{status:200}': ['p(95)<500', 'p(99)<1000'],
    'http_req_failed': ['rate<0.10'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

const BASE = __ENV.TAMGA_BASE_URL || 'http://localhost:8443';
const API_KEY = __ENV.TAMGA_API_KEY || 'test-key';

export default function () {
  const e = DIST[Math.floor(Math.random() * DIST.length)];
  const headers = {
    'Content-Type': 'application/json',
    'x-api-key': API_KEY,
  };

  let res;
  if (e.method === 'POST') {
    res = http.post(`${BASE}${e.url}`, e.body, { headers });
  } else {
    res = http.get(`${BASE}${e.url}`, { headers });
  }

  check(res, {
    'status ok': (r) => r.status >= 200 && r.status < 500,
  });
}
