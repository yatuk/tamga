# tamga-client

Python SDK for the [Tamga AI Security Proxy](https://tamga.io).

## Installation

```bash
pip install -e .
```

Or with development dependencies:

```bash
pip install -e ".[dev]"
```

## Quick start

```python
from tamga import TamgaClient

client = TamgaClient(base_url="http://localhost:8080", admin_key="sk-...")

# Dashboard stats
stats = client.get_stats(range="7d")
print(f"Total requests: {stats['total_requests']}")

# Paginated events
events = client.get_events(page=1, limit=50)
for ev in events["events"]:
    print(ev["request_id"], ev["action"])

# Findings breakdown
breakdown = client.get_findings_breakdown(range="30d")
print(breakdown["by_type"])

# Reload policy from disk
result = client.reload_policy()
print(result["ok"])  # True
```

The client can also be used as a context manager:

```python
with TamgaClient("http://localhost:8080", "sk-...") as client:
    stats = client.get_stats()
```

## API reference

| Method | HTTP | Description |
|---|---|---|
| `get_stats(range)` | `GET /api/v1/stats` | Dashboard overview statistics |
| `get_events(page, limit)` | `GET /api/v1/events` | Paginated security events |
| `get_findings_breakdown(range)` | `GET /api/v1/findings/breakdown` | Finding type/category/severity breakdown |
| `reload_policy()` | `POST /api/v1/policies/reload` | Reload policy from disk |

### Error handling

All HTTP errors (4xx/5xx) and non-JSON responses raise `TamgaError`:

```python
from tamga import TamgaError

try:
    client.get_stats()
except TamgaError as e:
    print(f"HTTP {e.status_code}: {e.body}")
```

Constructor raises `ValueError` when `base_url` or `admin_key` is empty.
