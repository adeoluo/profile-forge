# ADR 0002: Cache raw API responses keyed by URL

## Status

Accepted.

## Context

profile-forge needs three things from a cache: fewer API requests on repeated
runs, fully offline runs for tests and for working without a network, and
output that is identical whether it came from the network or the cache.

Two designs were considered:

1. **Cache the decoded model** (one file with `[]Repo`). Simple, but offline
   runs would skip pagination and response decoding entirely, so the offline
   path would not exercise the same code as the online path, and partial
   refreshes would need custom merge logic.
2. **Cache raw HTTP responses keyed by URL.** The client consults the cache
   inside its single `get` method; everything above it is unchanged.

## Decision

Option 2. Each entry stores the URL, status code, `Link` header, body, and
fetch time in one JSON file named by a truncated SHA-256 of the URL. Both 200
and 404 responses are stored, because a 404 from `/releases/latest` is the
answer "no release". Online runs honor a TTL (default 6 hours). `--offline`
ignores the TTL and fails on any miss with a message that says which URL is
missing.

## Consequences

- Offline runs follow the same pagination and decoding path as online runs,
  which the CLI tests verify by comparing outputs byte for byte.
- Entries are independent, so one expired or corrupt entry costs one request.
- Writes are atomic (temporary file and rename in the same directory).
- The cache grows with the number of URLs and is never pruned. For a profile
  with 37 repositories it is 616 KiB.
- ETag revalidation would save rate limit on warm refreshes; it is not
  implemented yet.
