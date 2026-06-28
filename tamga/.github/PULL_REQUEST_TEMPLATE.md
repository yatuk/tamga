## Description

<!-- Briefly describe what this PR does and why. -->

## Type of Change

- [ ] 🚀 New feature (feat)
- [ ] 🐛 Bug fix (fix)
- [ ] 📚 Documentation (docs)
- [ ] 🧪 Tests (test)
- [ ] 🔧 Maintenance (chore/ci/refactor)
- [ ] ⚡ Performance (perf)

## Related Issues

<!-- Link related issues: Closes #123, Fixes #456 -->

## Checklist

- [ ] Branch is up to date with `main`
- [ ] Conventional commits used (e.g., `feat(scanner): add IBAN detection`)
- [ ] **Go proxy**: `go vet ./...` clean, `go test ./... -race -count=1` passes
- [ ] **Dashboard**: `npx tsc --noEmit` clean, `npm run lint` passes, `npm run test:run` passes
- [ ] **Python analyzer**: `pytest tests/` passes
- [ ] Tests added/updated for all changed code paths
- [ ] Documentation updated if API surface changed
- [ ] `.env.example` updated if new environment variables added
- [ ] No commented-out code or debug logs in production paths
- [ ] No new dependencies without justification in PR description

## Screenshots (if UI change)

<!-- Drag and drop screenshots here -->

## Additional Notes

<!-- Any context for reviewers: design decisions, trade-offs, follow-up work -->
