## Description

<!-- What this pull request changes and why. -->

## Type of Change

- [ ] 🚀 New feature (feat)
- [ ] 🐛 Bug fix (fix)
- [ ] 📚 Documentation (docs)
- [ ] 🧪 Tests (test)
- [ ] 🔧 Maintenance (build/ci/chore/refactor)
- [ ] ⚡ Performance (perf)

## Related Issues

<!-- Closes #123 -->

## Tests run

<!--
The commands you ran and what they printed, for example:
  cd proxy && TAMGA_STORE_NO_DOCKER=1 go test -count=1 ./...   ok, 44 packages
  cd extension && node --test                                  509 passed
CONTRIBUTING.md lists the command for each part.
-->

## Checklist

- [ ] Every commit is signed off (`git commit -s`; see [DCO](https://github.com/yatuk/tamga/blob/development/CONTRIBUTING.md#sign-your-commits-dco))
- [ ] The branch is based on `development` and up to date with it
- [ ] Conventional commit messages (e.g. `feat(scanner): find a Dutch BSN where the text names it`)
- [ ] Tests added or updated for the changed code, and the tests of each part I changed pass (listed above)
- [ ] Documentation updated if behaviour, configuration or the API changed (`proxy/docs/openapi.yaml`, `.env.example`, `docs/operations.md`)
- [ ] An entry under `## Unreleased` in `CHANGELOG.md` if a user or operator would notice the change
- [ ] No commented-out code or debug logs in production paths
- [ ] No new dependency, or the description says why it is needed

## Screenshots (if UI change)

<!-- Drag and drop screenshots here -->

## Additional Notes

<!-- Context for reviewers: design decisions, trade-offs, follow-up work -->
