# Contributing to Tamga

Thank you for considering contributing to Tamga! This document outlines the
process for contributing code, documentation, and bug reports.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Commit Convention](#commit-convention)
- [Code Style](#code-style)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Bug Reports](#bug-reports)
- [Feature Requests](#feature-requests)
- [Documentation](#documentation)
- [License](#license)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

| Component | Requirement |
|-----------|-------------|
| **Go proxy** | Go 1.25+ |
| **Python analyzer** | Python 3.11+ |
| **Dashboard** | Node.js 20+ |
| **Database (optional)** | PostgreSQL 16+ |
| **Cache (optional)** | Redis 7+ |

### Setup

```bash
git clone https://github.com/yatuk/tamga.git
cd tamga

# Copy environment template
cp .env.example .env
# Edit .env with your API keys

# Go proxy
cd proxy
go mod download
go run ./cmd/tamga

# Python analyzer (separate terminal)
cd analyzer
pip install -r requirements.txt
uvicorn app.main:app --port 8444

# Next.js dashboard (separate terminal)
cd dashboard
npm ci --legacy-peer-deps
npm run dev
```

### Running the full stack

```bash
cd deploy
docker compose up -d
```

## Development Workflow

1. **Fork** the repository on GitHub
2. **Clone** your fork locally
3. **Branch** off `dev` for all work:
   ```bash
   git checkout dev
   git pull origin dev
   git checkout -b feat/your-feature-name
   ```
4. **Write code** following our [code style](#code-style) guidelines
5. **Test** your changes thoroughly
6. **Commit** using [Conventional Commits](#commit-convention)
7. **Push** your branch and open a PR against `main`

### Branch naming

| Prefix | Purpose | Example |
|--------|---------|---------|
| `feat/` | New features | `feat/oidc-sso-support` |
| `fix/` | Bug fixes | `fix/race-condition-bus` |
| `docs/` | Documentation only | `docs/api-reference` |
| `chore/` | Maintenance, CI, deps | `chore/update-go-1.25` |
| `test/` | Adding missing tests | `test/scanner-fuzz-cases` |
| `refactor/` | Code restructuring | `refactor/extract-ratelimit` |
| `perf/` | Performance improvements | `perf/reduce-alloc-hotpath` |

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/) 1.0.0:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

- `feat` — A new feature
- `fix` — A bug fix
- `docs` — Documentation only changes
- `style` — Formatting, missing semicolons, etc. (no code change)
- `refactor` — Code change that neither fixes a bug nor adds a feature
- `perf` — Code change that improves performance
- `test` — Adding missing tests or correcting existing tests
- `chore` — Changes to build process, CI, dependencies, tooling
- `revert` — Reverts a previous commit

### Scope

Use the component name: `proxy`, `analyzer`, `dashboard`, `scanner`, `api`,
`policy`, `store`, `deploy`, `docs`, `ci`.

### Examples

```
feat(scanner): add IBAN detection with TR country-specific validation
fix(proxy): prevent goroutine leak in event bus shutdown
docs(api): document webhook payload schema
test(ratelimit): add concurrent access tests for token bucket
chore(ci): add dashboard E2E job to PR workflow
```

## Code Style

### Go (proxy/)

- Follow [Effective Go](https://go.dev/doc/effective_go) and standard Go idioms
- Run `go vet ./...` before committing — zero warnings
- Run `go fmt ./...` or use `gofumpt` for consistent formatting
- Avoid package-level mutable state; prefer dependency injection
- Use `zerolog` for structured logging — no `fmt.Println` in production paths
- Interface definitions live in the consuming package (Go best practice)
- Errors are values: wrap with context, handle explicitly
- Tests use table-driven patterns with `t.Run()` subtests
- Package `internal/` tree: no external imports of internal packages

```go
// Good: structured log with context
log.Info().Str("request_id", id).Int("findings", n).Msg("scan complete")

// Good: table-driven test
tests := []struct {
    name string
    input string
    want  bool
}{...}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

### Python (analyzer/)

- Follow [PEP 8](https://peps.python.org/pep-0008/) with 100-char line limit
- Type hints on all public functions (`mypy --strict` compatible)
- Use `structlog` for structured logging, not `print()`
- Tests use `pytest` with fixtures, not `unittest.TestCase`
- Async endpoints use `async def` — no blocking calls in the event loop
- Avoid mutable default arguments

```python
# Good: typed async endpoint
async def scan_text(request: ScanRequest) -> ScanResponse:
    logger.info("scanning", request_id=request.id)
    ...
```

### TypeScript / React (dashboard/)

- Follow the project's ESLint flat config (`eslint.config.mjs`)
- Run `npx tsc --noEmit` before committing — zero errors
- Components use functional style with hooks; no class components
- Use Tailwind CSS utility classes; avoid inline styles
- `use client` / `use server` directives at the top of each file
- Form handling: `react-hook-form` + `zod` validation
- TanStack Query for server state; React Context only for UI state
- i18n: all user-facing strings go through `useDictionary()` / `t()`

```tsx
// Good: typed hook with proper i18n
const { t } = useDictionary();
const { data, isLoading } = useQuery({ queryKey: ['keys'], queryFn: () => api.getApiKeys() });
```

## Testing

### Go

```bash
cd proxy
go test ./... -v -count=1            # Full suite
go test -race ./... -count=1         # Race detector
go test ./internal/scanner/ -bench=. # Benchmarks
```

Coverage thresholds: new packages should have >70% statement coverage.

### Python

```bash
cd analyzer
pytest tests/ -v
pytest tests/ --cov=app --cov-report=term
```

### Dashboard

```bash
cd dashboard
npm run test:run       # Unit tests (vitest)
npm run lint           # ESLint
npx tsc --noEmit       # Type check
npx playwright test    # E2E tests (requires running dev server)
```

All tests must pass. Add tests for new functionality.

## Pull Request Process

1. **Create** a PR from your feature branch to `dev`
2. **Fill out** the PR template completely
3. **Ensure** all CI checks pass (tests, lint, type check, build)
4. **Link** related issues using GitHub keywords (`Closes #123`)
5. **Request review** from a maintainer
6. **Address** review feedback
7. **Squash merge** once approved (maintainer merges)

### PR checklist

- [ ] Branch is up to date with `dev`
- [ ] Conventional commit messages used
- [ ] Tests added/updated for all changed code
- [ ] Go: `go vet ./...` clean, `go test ./...` passes
- [ ] Dashboard: `tsc --noEmit` clean, `npm run lint` passes, `npm run test:run` passes
- [ ] Python: `pytest tests/` passes
- [ ] Documentation updated if API surface changed
- [ ] No commented-out code or debug logs in production paths
- [ ] No new dependencies without justification

## Bug Reports

Open an issue using the **Bug Report** template. Include:

1. **Version**: commit hash or release tag
2. **Environment**: OS, Go/Python/Node version, Docker vs bare metal
3. **Steps to reproduce**: minimal reproducible example
4. **Expected vs actual behavior**
5. **Logs**: proxy logs, analyzer logs, dashboard console errors
6. **Policy YAML** (sanitized — remove real API keys)

Security vulnerabilities: **DO NOT open a public issue.** See [SECURITY.md](SECURITY.md)
for private disclosure instructions.

## Feature Requests

Open an issue using the **Feature Request** template. Describe:

- The problem you're solving
- Proposed solution (if you have one)
- Alternatives considered
- How you'd test it

## Documentation

- API changes must update `proxy/docs/openapi.yaml`
- New environment variables must be added to `.env.example`
- Architecture changes must update the Mermaid diagram in `README.md`
- Major features should include a demo script entry in `docs/DEMO_SCRIPT_W2.md`

## License

Tamga is **open-core** under the [GNU AGPL v3.0](LICENSE).

By contributing code, you agree that your contributions will be licensed under
the same terms. Enterprise features are available under a separate commercial
license — see [LICENSE](LICENSE) for details.

---

**Questions?** Open a [GitHub Discussion](https://github.com/tamga/tamga/discussions)
or reach out to the team.
