# Release Process

SeaPortal ships pre-built binaries via GitHub Releases, built by goreleaser
from a tag push.

## Prerequisites

### Secrets (configure once in GitHub Settings → Secrets and variables → Actions)

- `HOMEBREW_TAP_GITHUB_TOKEN` — only if/when a Homebrew tap is added.

CodeQL, branch-naming, and the e2e workflow run without extra secrets.

### Local setup

```bash
git checkout main && git pull origin main
git describe --tags    # confirm the previous tag
./dev all              # gate: check + test + e2e (must pass)
./dev bench eval       # confirm no extraction-quality regression
```

## Pre-release checklist

1. **goreleaser config sane** — `cat tests/release/goreleaser_test.go` documents
   the invariants the test enforces. Run it:
   ```bash
   go test ./tests/release/
   ```
2. **Version consistency** — ensure no hard-coded version strings drifted.
   `grep -rn 'version *= *"' cmd/ internal/ | grep -v _test`
3. **CHANGELOG / commit log** — `git log --oneline $(git describe --tags --abbrev=0)..HEAD`
   should tell a clean story. Squash noise locally if needed.
4. **Open PR review state** — no pending PRs touching the same areas.

## Tagging + release

```bash
# 1. Tag and push
git tag -a v0.X.Y -m "Release v0.X.Y"
git push origin v0.X.Y

# 2. GitHub Actions (.github/workflows/release.yml) takes over:
#    - Validates the tag and secrets
#    - Runs goreleaser to build the 5-platform binary matrix
#    - Publishes the GitHub Release with checksums.txt
```

Watch the workflow run; the release is live when the GitHub Release page
shows all binaries + `checksums.txt`.

## Post-release verification

```bash
# Download a binary and smoke-test it locally
curl -L -o /tmp/seaportal \
  "https://github.com/pinchtab/seaportal/releases/download/v0.X.Y/seaportal-darwin-arm64"
chmod +x /tmp/seaportal
/tmp/seaportal --version          # prints v0.X.Y
/tmp/seaportal https://example.com  # smoke extraction
```

If verification fails: file a follow-up issue, do NOT delete the tag (immutability).
A patch release (v0.X.Y+1) is cheaper than re-creating a tag.

## Rollback

GitHub Releases can be marked as draft / pre-release to hide a bad build
without deleting the tag. To fully retract, file a `v0.X.(Y+1)` patch
release with the fix and mark the bad release as "deprecated" in the
release notes.

## Cadence

No fixed cadence. Release when:
- A user-facing bug fix lands
- A new subcommand is added
- The eval corpus quality numbers improve by ≥ 5% F1 across the bake-off

Don't release for pure internal-refactor changes — they're already in
`main` for any consumer building from source.
