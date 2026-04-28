# Contributing to DD Photos

DD Photos started out as a personal project for self-hosting photo albums. It's open 
source because others may find it useful, and contributions are welcome!  Find a bug,
submit a fix!  Have an idea for a new feature, submit a PR!  Remember to keep backwards
compatibility for any big changes.  I'm very interested in making changes to
enable you to successfully use DD Photos.

## Bug Reports

Open a [GitHub Issue](https://github.com/dougdonohoe/ddphotos/issues) with:

- A clear description of the problem
- Steps to reproduce
- Expected vs. actual behavior
- Relevant environment details (OS, browser, Go/Node version)

## Pull Requests

For small fixes or improvements, feel free to open a PR directly. For larger changes
or new features, please open an issue first to discuss — this avoids investing time in
work that may not fit the project's direction.

Keep PRs focused. One thing per PR is easier to review and merge.  When appropriate,
include docs changes.

## Development Setup

- [docs/INSTALL.md](docs/INSTALL.md) — prerequisites and sample app setup
- [docs/ENV.md](docs/ENV.md) — environment configuration
- [docs/MAKEFILE.md](docs/MAKEFILE.md) — all Makefile targets
- [docs/PHOTOGEN.md](docs/PHOTOGEN.md) — `photogen` CLI flags and configuration

## Testing

CI runs automatically on every PR and covers the full test suite: Go unit tests, a
static site build with sample data, Apache and nginx routing tests, and Playwright
end-to-end tests across all password variants. The CI gate will catch regressions,
so you don't need to run everything locally — use your judgment about what's relevant
to your change.

```bash
make build test vet          # Go build, unit tests, static analysis
make sample-build            # build static site with sample data
bin/test-all.sh --mode apache  # Playwright e2e tests (requires Docker)
```

**New functionality** should include tests — Go unit tests for backend logic, Playwright
tests in `web/tests/` for UI behavior. 

**Bug fixes** should include a test if feasible;
some bugs aren't practically testable, and that's fine.

## Code Style

**Go** — run `gofmt` and `go vet` before committing (`make build test vet` covers this).

**TypeScript / Svelte** — no enforced formatter; follow the style of the surrounding code.

## License

By contributing, you agree that your changes will be licensed under the
[AGPL v3](LICENSE.txt).
