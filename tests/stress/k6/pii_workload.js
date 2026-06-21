import http from 'k6/http';
import { check } from 'k6';

// PII-heavy workload: used during chaos tests to verify scanning continues
// when backend services (analyzer, postgres, redis) are disrupted.
const PII_PROMPTS = [
  'My email is user@example.com',
  'TC: 10000000146',
  'Call me at +90 532 555 1234',
  'Card: 4532015112830366',
  'IBAN: TR330006100519786457841326',
];

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: 100,
      timeUnit: '1s',
      duration: '120s',
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.30'], // Allow higher error rate during chaos
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

const BASE = __ENV.TAMGA_BASE_URL || 'http://localhost:8443';
const API_KEY = __ENV.TAMGA_API_KEY || 'test-key';

export default function () {
  const content = PII_PROMPTS[Math.floor(Math.random() * PII_PROMPTS.length)];
  const res = http.post(`${BASE}/v1/messages`, JSON.stringify({
    model: 'claude-3-haiku-20240307',
    messages: [{ role: 'user', content }],
  }), {
    headers: {
      'Content-Type': 'application/json',
      'x-api-key': API_KEY,
    },
  });
  check(res, {
    'response received': (r) => r.status > 0,
    'scan header present': (r) => r.headers['X-Tamga-Findings-Count'] !== undefined || r.status >= 500,
  });
}
