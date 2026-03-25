# Contributing

## Setup

```bash
git clone git@github.com:pinchtab/seaportal.git
cd seaportal
bash scripts/install-hooks.sh
./dev doctor
```

## Development Workflow

1. Create a branch from `main`
2. Make your changes
3. Run `./dev check` (format, vet, lint, tests)
4. Commit with a descriptive message
5. Open a PR against `main`

## Commit Messages

Follow conventional commits:

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `ci:` CI/CD changes
- `chore:` maintenance

## Testing

```bash
./dev test          # Unit tests
./dev test race     # With race detector
./dev coverage      # Coverage report
./dev e2e           # E2E tests (requires Docker)
```

## Code Style

- `gofmt` for formatting (enforced by pre-commit hook)
- `golangci-lint` for linting
- Keep functions small and focused
- Tests live next to the code they test

## Adding Test Fixtures

See [fixtures documentation](../fixtures.md) for how to add HTML test fixtures.
