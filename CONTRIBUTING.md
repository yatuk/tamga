# Contributing to Tamga

Thank you for considering a contribution. This file says how to run each
part of Tamga and its tests, what a pull request should look like, and how
to sign your commits. [docs/development.md](docs/development.md) has the
repository layout and the adversarial test suite in more depth.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Where to start](#where-to-start)
- [Getting Started](#getting-started)
- [Testing](#testing)
- [Development Workflow](#development-workflow)
- [Commit Convention](#commit-convention)
- [Sign your commits (DCO)](#sign-your-commits-dco)
- [Code Style](#code-style)
- [Pull Request Process](#pull-request-process)
- [Bug Reports and Feature Requests](#bug-reports-and-feature-requests)
- [Documentation](#documentation)
- [License](#license)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold this code.

## Where to start

Issues labelled `good first issue` are small and say which files to touch
and how to test the change. A new detector is the easiest larger
contribution: a national identifier, a phone format or a secret format the
scanners do not know yet. Open a **New detector** issue first so the format,
its check digit and the words that name it can be agreed before you write
the pattern. [docs/scanner-development.md](docs/scanner-development.md)
describes the scanner interface; `proxy/internal/scanner/pii_tr_ids.go` is
the model for an identifier that is only taken where the text names it.

Questions go to GitHub Discussions, not to issues. Security problems go to
[SECURITY.md](SECURITY.md), never to a public issue.

## Getting Started

### Prerequisites

| Component | Requirement (what CI uses) |
|-----------|-------------|
| **Go proxy** (`proxy/`) | Go 1.26 or later (`proxy/go.mod`) |
| **Analyzer, classifier, Python SDK** | Python 3.11 or later |
| **Console** (`dashboard/`), **TypeScript SDK**, **browser extension** | Node.js 22 |
| **Full stack** | Docker with Compose |

### Run each part locally

```bash
git clone https://github.com/yatuk/tamga.git
cd tamga
cp .env.example .env    # TAMGA_MOCK_UPSTREAM=true runs without provider keys

# Go proxy, on :8443
cd proxy && go run ./cmd/tamga

# Python analyzer, on :8444 (from the repo root, separate terminal)
cd analyzer && pip install -r requirements.txt
uvicorn app.main:app --port 8444

# Console, on :3000 (from the repo root, separate terminal)
cd dashboard && npm ci --legacy-peer-deps
npm run dev
```

The browser extension has no build step: load `extension/` as an unpacked
extension ([docs/browser-extension.md](docs/browser-extension.md)).

The full stack runs under Compose; every service belongs to a profile
(`minimal`, `full`, `classifier`), described in
[docs/operations.md](docs/operations.md#docker-compose-profiles):

```bash
docker compose --env-file .env -f deploy/docker-compose.yml up -d
```

## Testing

Run the tests of every part you changed. The commands below are the ones
the CI workflows in `.github/workflows/` run.

| Part | Command | Workflow |
|------|---------|----------|
| Go proxy | `cd proxy && go vet ./... && TAMGA_STORE_NO_DOCKER=1 go test -count=1 ./...` | `proxy-go-fast.yml` |
| Console | `make dashboard-test` (lint, type check, vitest; after `npm ci --legacy-peer-deps` in `dashboard/`) | `dashboard-tests.yml` |
| Browser extension | `cd extension && node --test` | `extension-tests.yml` |
| Classifier | `make classifier-test` (needs `pip install -r classifier/requirements.txt pytest`) | `classifier-ci.yml` |
| Analyzer | `cd analyzer && pip install pytest && python -m pytest -q tests` | none yet |
| Python SDK | `cd sdk/python && pip install -e ".[dev]" && python -m pytest -q tests` | none yet |
| TypeScript SDK | `cd sdk/typescript/tamga-client && npm ci && npm test` | none yet |

Notes:

- **Go store tests.** With `docker` on the `PATH`, the `internal/store`
  tests start a throwaway PostgreSQL container. `TAMGA_STORE_NO_DOCKER=1`
  keeps them from doing so and the Postgres tests skip, as in CI's fast job;
  `TAMGA_TEST_DB_URL` points them at a PostgreSQL of your own instead.
- **Race detector.** `make test` runs `go test -race ./...`. CI runs it
  weekly and before a release (`proxy-ci.yml`), not on every pull request;
  run it when you change code that runs concurrently.
- **Detection floors.** The proxy suite includes the red-team floors in
  `proxy/cmd/redteam`: a change that lowers detection fails there.
- **API types.** After changing `proxy/docs/openapi.yaml`, run
  `npm run gen:api-types` in `dashboard/` and commit
  `lib/api-types.generated.ts`; CI fails when it is out of date.
- **Console end-to-end tests.** `npx playwright test` in `dashboard/`
  starts `npm run dev` itself unless `CI` is set.

Add tests for what you change. Tests never contact a real external service:
use `https://example.invalid/` or a local test server, and test values that
are obviously fake.

## Development Workflow

1. **Fork** the repository, clone your fork, and add this repository as
   `upstream`:
   ```bash
   git remote add upstream https://github.com/yatuk/tamga.git
   ```
2. **Branch** off `development`, which is where work lands; `main` takes
   releases:
   ```bash
   git fetch upstream
   git switch -c development upstream/development   # once
   git switch development
   git pull upstream development
   git switch -c feat/your-feature-name
   ```
3. **Write** the change and its tests.
4. **Commit** with [Conventional Commits](#commit-convention), signed off
   ([DCO](#sign-your-commits-dco)).
5. **Push** your branch and open a pull request against `development`.

Branch names start with the commit type: `feat/`, `fix/`, `docs/`, `test/`,
`refactor/`, `perf/`, `build/`, `ci/`, `chore/`.

### Line endings

The repository stores every text file with LF line endings, and there is
no `.gitattributes`. On Windows, set `git config core.autocrlf true` (or
`input`) so that files are committed with LF; a shell script committed with
CRLF fails on Linux. Elsewhere leave `core.autocrlf` unset. Check before you
push with `git ls-files --eol | grep i/crlf`, which should print nothing.

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/) 1.0.0,
as `git log` shows:

```
<type>(<scope>): <description>

<body: what changed and why, wrapped at 72 columns>

Signed-off-by: Your Name <you@example.com>
```

- **Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`
  (dependencies, images, packaging), `ci`, `chore`, `revert`.
- **Scope:** the part you changed: `proxy`, `scanner`, `policy`, `api`,
  `store`, `analyzer`, `classifier`, `dashboard`, `extension`, `sdk`,
  `deploy`, `ci`, `docs`. Leave it out when a change spans several.
- **Description:** lower case, imperative, no full stop, saying what the
  change does for someone using Tamga.

```
feat(scanner): find a Dutch BSN where the text names it
fix(proxy): prevent goroutine leak in event bus shutdown
docs(api): document webhook payload schema
test(scanner): cover the scan cache's expiry
```

One commit is one change: a fix and an unrelated refactor are two commits.

## Sign your commits (DCO)

Tamga uses the [Developer Certificate of Origin](https://developercertificate.org/)
(DCO) instead of a contributor licence agreement. By adding a
`Signed-off-by:` line to a commit you certify that you wrote the change, or
otherwise have the right to submit it under the project's licence, and that
you understand the contribution and your sign-off are public and kept.

Git adds the line for you with `-s`:

```bash
git commit -s -m "fix(scanner): ..."
```

The line must carry the same email as the commit's author
(`git config user.email`). The `DCO` check (`.github/workflows/dco.yml`)
fails a pull request in which any commit lacks a matching sign-off. Merge
commits and commits by `dependabot[bot]` are not checked. The check reads
only the commits of the pull request; the project's earlier history, which
has no sign-offs, is not affected.

To check your branch before you push:

```bash
bash scripts/check-dco.sh upstream/development
```

To fix a missing sign-off:

```bash
git commit --amend --no-edit -s           # the last commit
git rebase --signoff upstream/development # every commit on your branch
git push --force-with-lease
```

## Code Style

### Go (`proxy/`)

- `gofmt` and `go vet ./...` clean.
- Structured logging with `zerolog`; no `fmt.Println` in production paths.
- Interfaces live in the package that uses them; wrap errors with context.
- Table-driven tests with `t.Run()` subtests.

### Python (`analyzer/`, `classifier/`)

- [PEP 8](https://peps.python.org/pep-0008/) with a 100-character line
  limit, type hints on public functions.
- `structlog` for logging, not `print()`.
- `pytest` tests with fixtures, not `unittest.TestCase`.
- No blocking calls inside `async def` endpoints.

### TypeScript / React (`dashboard/`)

- The ESLint flat config (`eslint.config.mjs`) and `npm run type-check`
  clean.
- Functional components with hooks. A file that uses hooks or browser APIs
  starts with `"use client"`.
- Carbon components (`@carbon/react`); the project's own styles are in
  `app/tamga.css`. Use its colour tokens (`--tamga-*`, `--cds-*`): the
  `tamga/no-hardcoded-color` lint rule reports a literal colour. Visual
  changes follow [DESIGN.md](DESIGN.md).
- TanStack Query for server state. Calls to the proxy go through the `api`
  object in `lib/api.ts`, with types from `proxy/docs/openapi.yaml`.
- The console is in English only; there is no translation layer.

### Browser extension (`extension/`)

- Plain ES modules, no dependencies and no build step. Tests use
  `node --test`.

## Pull Request Process

A good pull request:

- does one thing, and says in its description what and why;
- links the issue it closes (`Closes #123`);
- has tests for the change, and the test commands you ran with their
  results;
- updates the docs and the `## Unreleased` section of `CHANGELOG.md` when a
  user or operator would notice the change;
- adds no dependency without saying why it is needed;
- has every commit signed off.

CI must pass. A maintainer reviews, may ask for changes, and merges.

## Bug Reports and Feature Requests

Use the issue forms: **Bug report**, **Feature request** or **New
detector**. Remove API keys, tokens and personal data from anything you
paste, logs and policy files included.

Security vulnerabilities: **do not open a public issue.** See
[SECURITY.md](SECURITY.md) for private reporting.

## Documentation

- API changes update `proxy/docs/openapi.yaml` (and the generated console
  types, see [Testing](#testing)).
- A new environment variable goes into `.env.example` and, if an operator
  sets it, into the table in `docs/operations.md`.
- Architecture changes update the Mermaid diagram in `README.md`.
- A change a user would notice gets an entry in `## Unreleased` at the top
  of `CHANGELOG.md`.

## License

Tamga is licensed under the [GNU AGPL v3.0](LICENSE), and under nothing else.

Contributions come in under the licence the project goes out under: by
contributing, you agree that your contribution is licensed under the AGPL-3.0.
You keep the copyright in what you wrote. There is no contributor licence
agreement and no copyright assignment; the DCO sign-off is the record that
you had the right to contribute the change.

---

**Questions?** Ask in [GitHub Discussions](https://github.com/yatuk/tamga/discussions).
