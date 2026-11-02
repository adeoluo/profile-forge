# ADR 0001: YAML config with strict decoding

## Status

Accepted.

## Context

The config is edited by hand and holds prose (bio, flagship results,
category descriptions) next to structured data (colors, edges, repository
lists). The options were JSON with `encoding/json` (no dependency), TOML, or
YAML.

- JSON has no comments and needs escaped newlines for multi-line prose.
- TOML handles prose well but nested lists of tables (categories with repo
  lists, edges with optional fields) are verbose.
- YAML handles both, supports comments, and inline maps keep edge lists short.

## Decision

Use YAML through `gopkg.in/yaml.v3`, the most widely used YAML library for Go,
and decode with `KnownFields(true)` so an unknown key is an error.

## Consequences

- One third-party dependency, pinned in `go.mod` and verified by `go.sum`.
- A misspelled key such as `colour` fails loudly instead of being ignored,
  which matters because an ignored color would silently fall back to a
  default and the legend would still look plausible.
- YAML's implicit typing can surprise (for example an unquoted `#dbeafe` is a
  comment). The example configs quote every color, and validation rejects
  any color that is not `#rrggbb`, so the surprise becomes an error message.
