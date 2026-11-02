# AGENTS.md

Working notes for coding agents and contributors in **Profile Forge**. The [README](README.md)
explains what the project does and why; this file explains how to change it safely.

## Project

A Go command-line tool that generates a GitHub profile README and a portfolio JSON document from live repository metadata and a small YAML config.

## Layout

- `cmd/`: entry points for the binaries
- `docs/`: documentation, architecture diagram source and render, design records
- `examples/`: runnable examples and sample configuration
- `internal/`: packages private to this module

Key modules:

- `cmd/profile-forge`: Command profile-forge generates a GitHub profile README from live repository metadata and a small YAML config.
- `internal/cli`: Package cli implements the profile-forge command line.
- `internal/config`: Package config loads and validates the profile-forge configuration file.
- `internal/fakegh`: Package fakegh serves a minimal fake of the GitHub REST endpoints that profile-forge uses.
- `internal/github`: Package github is a small GitHub REST API client with an on-disk cache.
- `internal/render`: Package render turns config plus repository metadata into Markdown and JSON.
- `internal/validate`: Package validate checks generated Markdown before it is written.

## Commands

| Command | Purpose |
|---|---|
| `make all` | Full local gate (mirror of continuous integration) |
| `make build` | Build the binaries |
| `make test` | Run the tests |
| `make race` | Tests with the race detector |
| `make cover` | Tests with a coverage profile |
| `make vet` | Static checks with go vet |
| `make fmt` | Format the code |
| `make fmt-check` | Fail if code is not formatted |
| `make lint` | Lint |
| `make bench` | Run the benchmarks |
| `make example` | Regenerate the example profile README from examples/ |
| `make check-example` | Fail if the committed example README is stale |
| `make release` | Cross-compile release binaries into dist/ |
| `make verify-release` | Verify release archive checksums |
| `make clean` | Remove build output |
| `make changelog` | Regenerate CHANGELOG.md from Conventional Commits (git-cliff) |
| `make cut-release` | Cut a release locally: version bump, changelog, tag, GitHub release (RELEASE_VERSION=vX.Y.Z optional) |

Run the full local gate (`make all`) before committing; CI runs the same checks.

## Conventions

- Commits follow Conventional Commits (`feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `test:`, `ci:`, `chore:`).
  The changelog and the next version number are derived from them.
- Every number quoted in the README is produced by a command in this repository. When behavior
  changes, rerun that command and update the number in the same commit.
- When a component is added, removed or renamed, update `docs/architecture/architecture.mmd`
  and re-render `architecture.svg` so the diagram matches the code.
- README sections keep their fixed order. Prose avoids em and en dashes and promotional wording.
- Tests accompany behavior changes; a bug fix comes with a test that fails without it.
- Releases are cut by the maintainer with `make cut-release`, never from CI.

## Go notes

- Code is formatted with `gofmt`; `go vet` and the race detector run in CI.
- Packages under `internal/` are not a public API; keep exported surface small.
- Errors are wrapped with context (`fmt.Errorf("...: %w", err)`), never discarded.
