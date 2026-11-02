# Security Policy

## Reporting a vulnerability

Please open a private advisory through GitHub (Security, then Advisories,
then "Report a vulnerability") rather than a public issue. I will
acknowledge within a few days.

## Scope and handling of credentials

- profile-forge reads a GitHub token from `GITHUB_TOKEN`, `GH_TOKEN`, or
  `gh auth token`. The token is sent only to the configured API base URL in
  the `Authorization` header. It is never written to the cache, logs, or
  output. Offline runs do not resolve a token at all.
- Only public repositories are fetched and rendered. Private repositories
  returned to an authenticated client are dropped before rendering.
- The cache stores API response bodies for public repositories in the user
  cache directory with mode 0644 files.
- `--check-links` sends GET requests to every image URL in the generated
  Markdown. With the default template these are shields.io URLs.
- Custom templates are executed with `text/template` and are trusted input.

## Controls

- CodeQL analysis for Go on every push, pull request, and weekly.
- `go vet` and staticcheck in CI.
- One third-party module, pinned and checksummed in `go.sum`.
