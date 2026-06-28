# @tamga/client

TypeScript SDK for the [Tamga AI Security Proxy](https://tamga.dev) REST API.

## Install

```bash
npm install @tamga/client
```

## Quick start

```ts
import { TamgaClient } from '@tamga/client';

const client = new TamgaClient('http://localhost:8080', 'sk-your-admin-key');

// Aggregate proxy statistics
const stats = await client.getStats('7d');
console.log(stats.total_requests, stats.blocked_requests);

// Security event feed
const { events, total } = await client.getEvents(1, 50);

// Findings breakdown by type & category
const breakdown = await client.getFindingsBreakdown('24h');

// Trigger an on-disk policy reload
const result = await client.reloadPolicy();
```

## API

### `new TamgaClient(baseUrl, adminKey)`

Creates a client instance.

- `baseUrl` — root URL of the proxy (e.g. `http://localhost:8080`). Trailing slashes are trimmed automatically.
- `adminKey` — value sent as the `X-Tamga-Admin-Key` header on every request.

Both arguments are required. Empty strings throw a `TamgaError`.

### `client.getStats(range?)`

`GET /api/v1/stats?range=RANGE`

Returns aggregate proxy statistics (total requests, blocked, uptime, top providers, etc.).

- `range` — optional time window: `"24h"`, `"7d"` (default), `"30d"`.

### `client.getEvents(page?, limit?)`

`GET /api/v1/events?page=PAGE&limit=LIMIT`

Returns a paginated list of security events.

- `page` — 1-based page number (default 1).
- `limit` — items per page (default 50).

### `client.getFindingsBreakdown(range?)`

`GET /api/v1/findings/breakdown?range=RANGE`

Returns finding type and category counts for the given time window.

- `range` — optional time window (default `"7d"`).

### `client.reloadPolicy()`

`POST /api/v1/policies/reload`

Triggers an on-disk policy reload. Returns `{ ok: true, name: "policy-name" }`.

## Error handling

All methods throw a `TamgaError` when:

- The proxy returns a non-2xx status code.
- The response body is not valid JSON.
- The request times out (default 30 s).
- A network error occurs.

`TamgaError` extends `Error` and includes:

- `.statusCode` — HTTP status (0 for client-side errors like timeout).
- `.body` — raw response body as a string.

```ts
import { TamgaClient, TamgaError } from '@tamga/client';

try {
  await client.getStats();
} catch (err) {
  if (err instanceof TamgaError) {
    console.error(`Tamga ${err.statusCode}: ${err.body}`);
  }
}
```

## License

MIT
